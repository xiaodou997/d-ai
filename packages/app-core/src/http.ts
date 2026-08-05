import type { ProblemDetail, RequestAdapter } from "@dai/api-client";
import { joinUrl, withQuery } from "@dai/api-client";

import { beginSSOAuthorize, currentRedirectUri, ssoLoopTripped } from "./sso";
import type { BackendService, PortalEnv } from "./env";

export class HttpProblem extends Error {
  status: number;
  code?: string;
  detail?: string;
  meta?: ProblemDetail["meta"];
  errors?: ProblemDetail["errors"];

  constructor(problem: ProblemDetail) {
    super(problem.detail || problem.title);
    this.name = "HttpProblem";
    this.status = problem.status;
    this.code = problem.code;
    this.detail = problem.detail;
    this.meta = problem.meta;
    this.errors = problem.errors;
  }
}

export class ServiceAccessUnavailableError extends Error {
  readonly service: BackendService;

  constructor(service: BackendService) {
    super(`当前账号未开通 ${service} 服务`);
    this.name = "ServiceAccessUnavailableError";
    this.service = service;
  }
}

export type RequestRecoveryResult = boolean | "retry" | "handled" | void;

export interface HttpClientOptions {
  getAccessToken?: () => string | undefined;
  onUnauthorized?: () => Promise<RequestRecoveryResult> | RequestRecoveryResult;
  onAccessDenied?: (status: number, code?: string) => Promise<RequestRecoveryResult> | RequestRecoveryResult;
  defaultHeaders?: Record<string, string>;
}

export interface PortalAuthLike {
  accessToken: string;
  serviceTokens: Record<string, { accessToken: string } | undefined>;
  refreshServiceAccessToken: (service: BackendService) => Promise<unknown>;
  ensureSession: (options?: { force?: boolean }) => Promise<unknown>;
  refreshCapabilities: () => Promise<string[]>;
  hasClientAccess: (clientID: string) => boolean;
  clearServiceToken: (service: BackendService) => void;
  clear: () => void;
}

export interface PortalRequestContext {
  portalHeaders: Record<string, string>;
  portalHeadersFor: (service: BackendService) => Record<string, string>;
  serviceBaseUrl: (service: BackendService) => string;
  authenticatedRequest: (service?: BackendService) => RequestAdapter;
}

export interface CreatePortalRequestContextOptions {
  env: PortalEnv;
  useAuthStore: () => PortalAuthLike;
}

export function createFetchAdapter(options: HttpClientOptions = {}): RequestAdapter {
  return async function request<T>({
    method,
    path,
    query,
    headers,
    body,
    baseUrl,
    signal
  }: {
    method: string;
    path: string;
    query?: Record<string, string | number | boolean | undefined | null>;
    headers?: Record<string, string | undefined>;
    body?: unknown;
    baseUrl?: string;
    signal?: AbortSignal;
  }) {
    if (!baseUrl) {
      throw new Error(`missing baseUrl for request ${method} ${path}`);
    }

    const url = joinUrl(baseUrl, withQuery(path, query));
    const payload = body === undefined ? undefined : JSON.stringify(body);
    const execute = () =>
      fetch(url, {
        method,
        headers: buildHeaders(options, headers, body),
        body: payload,
        signal
      });

    let response = await execute();

    if (response.status === 401 && options.onUnauthorized) {
      const result = await options.onUnauthorized();
      if (result === "handled") {
        return suspendForNavigation<T>();
      }
      if (result === true || result === "retry") {
        response = await execute();
      }
    }

    const accessProblemCode = await readProblemCode(response);
    if (accessProblemCode === "service_access_denied" && options.onAccessDenied) {
      const result = await options.onAccessDenied(response.status, accessProblemCode);
      if (result === "handled") {
        return suspendForNavigation<T>();
      }
      if (result === true || result === "retry") {
        response = await execute();
      }
    }

    if (!response.ok) {
      const contentType = response.headers.get("content-type") || "";
      if (contentType.includes("application/problem+json")) {
        throw new HttpProblem((await response.json()) as ProblemDetail);
      }
      throw new Error(`HTTP ${response.status}`);
    }

    if (response.status === 204) {
      return undefined as T;
    }

    return (await response.json()) as T;
  };
}

export function createPortalRequestContext(options: CreatePortalRequestContextOptions): PortalRequestContext {
  const portalHeaders = portalHeadersForEnv(options.env, "urm");

  function portalHeadersFor(service: BackendService): Record<string, string> {
    return portalHeadersForEnv(options.env, service);
  }

  function serviceBaseUrl(service: BackendService): string {
    return {
      urm: options.env.urmBaseUrl,
      ai: options.env.aiBaseUrl,
      proxy: options.env.proxyBaseUrl
    }[service];
  }

  function authenticatedRequest(service: BackendService = "urm"): RequestAdapter {
    return createFetchAdapter({
      getAccessToken: () => accessTokenForRequest(options.env, options.useAuthStore(), service),
      async onUnauthorized() {
        const authStore = options.useAuthStore();
        try {
          await authStore.refreshServiceAccessToken(service);
          return true;
        } catch {
          authStore.clear();
          return (await redirectPortalToLogin(options.env)) ? "handled" : false;
        }
      },
      async onAccessDenied(status, code) {
        return recoverPortalSession(options.env, options.useAuthStore, status, code, service);
      }
    });
  }

  return {
    portalHeaders,
    portalHeadersFor,
    serviceBaseUrl,
    authenticatedRequest
  };
}

export function createPortalRequestBindings(options: CreatePortalRequestContextOptions): PortalRequestContext {
  return createPortalRequestContext(options);
}

export function portalHeadersForEnv(env: PortalEnv, service: BackendService): Record<string, string> {
  return {
    "X-Client-Type": env.clientTypeHeader,
    "X-Client-Id": env.serviceClientIds?.[service] || env.xClientId
  };
}

function accessTokenForRequest(env: PortalEnv, authStore: PortalAuthLike, service: BackendService): string {
  if (service === "urm") {
    return authStore.accessToken;
  }

  const clientID = env.serviceClientIds?.[service];
  if (!clientID || !authStore.hasClientAccess(clientID)) {
    throw new ServiceAccessUnavailableError(service);
  }

  const token = authStore.serviceTokens[service]?.accessToken || "";
  if (token) {
    return token;
  }
  const redirectPath = `${window.location.pathname}?service=${encodeURIComponent(service)}`;
  // 整页跳转前异步生成 PKCE；这里不阻塞返回（本次请求无 token，跳转后会重来）。
  void beginSSOAuthorize(
    env,
    new URL(redirectPath, window.location.origin).toString(),
    window.location.pathname,
    service
  ).then((authorizeUrl) => {
    if (authorizeUrl) {
      window.location.assign(authorizeUrl);
    }
  });
  return "";
}

export async function recoverPortalSession(
  env: PortalEnv,
  useAuthStore: () => PortalAuthLike,
  status: number,
  code?: string,
  service: BackendService = "urm"
): Promise<RequestRecoveryResult> {
  if (status !== 403 || code !== "service_access_denied") {
    return false;
  }
  const authStore = useAuthStore();
  try {
    await authStore.refreshCapabilities();
    authStore.clearServiceToken(service);
    window.location.assign(defaultURMPath(env));
    return "handled";
  } catch {
    authStore.clear();
    return (await redirectPortalToLogin(env)) ? "handled" : false;
  }
}

let portalLoginRedirectStarted = false;

export async function redirectPortalToLogin(
  env: PortalEnv,
  redirectPath = `${window.location.pathname}${window.location.search}${window.location.hash}`
): Promise<boolean> {
  if (portalLoginRedirectStarted) {
    return true;
  }
  if (ssoLoopTripped()) {
    console.error(
      "[sso] 授权重定向风暴已熔断：登录态无法建立。请检查 token 交换 / userinfo / cookie 配置。"
    );
    return false;
  }
  const authorizeUrl = await beginSSOAuthorize(
    env,
    currentRedirectUri(redirectPath),
    "",
    "urm"
  );
  if (!authorizeUrl) {
    return false;
  }
  portalLoginRedirectStarted = true;
  window.location.assign(authorizeUrl);
  return true;
}

function defaultURMPath(env: PortalEnv): string {
  return {
    admin: "/overview",
    tenant: "/overview",
    customer: "/account"
  }[env.portal];
}

async function readProblemCode(response: Response): Promise<string | undefined> {
  if (response.ok || !response.headers.get("content-type")?.includes("application/problem+json")) return undefined;
  try {
    const problem = (await response.clone().json()) as { code?: unknown };
    return typeof problem.code === "string" ? problem.code : undefined;
  } catch {
    return undefined;
  }
}

function suspendForNavigation<T>(): Promise<T> {
  // 已经发起整页跳转时，挂起当前请求，避免页面在离开前再消费错误。
  return new Promise<T>(() => undefined);
}

function buildHeaders(
  options: HttpClientOptions,
  headers: Record<string, string | undefined> | undefined,
  body: unknown
): Headers {
  const requestHeaders = new Headers(options.defaultHeaders ?? undefined);
  for (const [key, value] of Object.entries(headers ?? {})) {
    if (value) requestHeaders.set(key, value);
  }

  const accessToken = options.getAccessToken?.();
  if (accessToken) {
    requestHeaders.set("Authorization", `Bearer ${accessToken}`);
  }

  if (body !== undefined && !requestHeaders.has("Content-Type")) {
    requestHeaders.set("Content-Type", "application/json");
  }

  return requestHeaders;
}
