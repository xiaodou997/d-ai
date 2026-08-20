import type { ProblemDetail, RequestAdapter } from "@/api";
import { joinUrl, withQuery } from "@/api";

import type { PortalEnv } from "./env";

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

export type RequestRecoveryResult = boolean | "retry" | "handled" | void;

export interface HttpClientOptions {
  getAccessToken?: () => string | undefined;
  onUnauthorized?: () => Promise<RequestRecoveryResult> | RequestRecoveryResult;
  defaultHeaders?: Record<string, string>;
}

export interface PortalAuthLike {
  accessToken: string;
  refreshAccessToken: () => Promise<unknown>;
  ensureSession: (options?: { force?: boolean }) => Promise<unknown>;
  clear: () => void;
}

export interface PortalRequestContext {
  apiHeaders: Record<string, string>;
  apiBaseUrl: string;
  authenticatedRequest: () => RequestAdapter;
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
        credentials: "include",
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
  const apiHeaders: Record<string, string> = {};

  function authenticatedRequest(): RequestAdapter {
    return createFetchAdapter({
      getAccessToken: () => options.useAuthStore().accessToken,
      async onUnauthorized() {
        const authStore = options.useAuthStore();
        try {
          await authStore.refreshAccessToken();
          return true;
        } catch {
          authStore.clear();
          return (await redirectPortalToLogin(options.env)) ? "handled" : false;
        }
      }
    });
  }

  return {
    apiHeaders,
    apiBaseUrl: options.env.apiBaseUrl,
    authenticatedRequest
  };
}

export function createPortalRequestBindings(options: CreatePortalRequestContextOptions): PortalRequestContext {
  return createPortalRequestContext(options);
}

let portalLoginRedirectStarted = false;

export async function redirectPortalToLogin(
  env: PortalEnv,
  redirectPath = `${window.location.pathname}${window.location.search}${window.location.hash}`
): Promise<boolean> {
  if (portalLoginRedirectStarted) {
    return true;
  }
  const loginUrl = new URL("/login", window.location.origin);
  loginUrl.searchParams.set("redirect", redirectPath);
  portalLoginRedirectStarted = true;
  window.location.assign(`${loginUrl.pathname}${loginUrl.search}${loginUrl.hash}`);
  return true;
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
