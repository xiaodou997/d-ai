import type { RequestAdapter } from "@dai/api-client";

export interface OAuthTokenResponse {
  access_token: string;
  refresh_token?: string;
  token_type: string;
  expires_in: number;
  scope?: string;
}

export interface UserInfoResponse {
  sub: string;
  username: string;
  userType: number;
  clientType: string;
  tenantId: string;
  tenantName: string;
  clientId: string;
}

export interface CurrentCapabilitiesResponse {
  enabledClientIds: string[];
}

export interface CreateAuthApiOptions {
  request: RequestAdapter;
  baseUrl: string;
  clientType: "admin" | "tenant" | "customer";
  xClientId: string;
}

export function createUrmAuthApi(options: CreateAuthApiOptions) {
  const authHeaders = {
    "X-Client-Type": options.clientType,
    "X-Client-Id": options.xClientId
  };

  return {
    async refreshToken(refreshToken: string): Promise<OAuthTokenResponse> {
      return requestToken(options, {
        grant_type: "refresh_token",
        refresh_token: refreshToken
      });
    },
    async exchangeCode(
      code: string,
      redirectUri: string,
      codeVerifier: string
    ): Promise<OAuthTokenResponse> {
      return requestToken(options, {
        grant_type: "authorization_code",
        code,
        redirect_uri: redirectUri,
        code_verifier: codeVerifier
      });
    },
    async logout(): Promise<{ success?: boolean; message?: string }> {
      return options.request({
        method: "POST",
        path: "/api/oauth2/revoke",
        headers: authHeaders,
        baseUrl: options.baseUrl
      });
    },
    async getCurrentUser(): Promise<UserInfoResponse> {
      return options.request({
        method: "GET",
        path: "/api/oauth2/userinfo",
        headers: authHeaders,
        baseUrl: options.baseUrl
      });
    },
    async getCurrentCapabilities(): Promise<CurrentCapabilitiesResponse> {
      return options.request({
        method: "GET",
        path: "/api/v2/me/capabilities",
        headers: authHeaders,
        baseUrl: options.baseUrl
      });
    },
    ssoLogoutUrl(postLogoutRedirectUri: string, clientType: string): string | null {
      if (!options.baseUrl) return null;
      const params = new URLSearchParams({
        client_type: clientType,
        post_logout_redirect_uri: postLogoutRedirectUri
      });
      return `${authEndpoint(options.baseUrl, "/api/oauth2/logout")}?${params.toString()}`;
    }
  };
}

// authEndpoint 拼接认证端点 URL，兼容相对前缀（dev 的 "/urm"，由 vite/edge 反代）
// 与绝对基址（prod 的 "http://host/urm"）。fetch/导航对相对 URL 会按页面 origin 解析，
// 故不能用 new URL(path, base)——base 为相对路径时会抛 TypeError。
function authEndpoint(baseUrl: string, path: string): string {
  return `${(baseUrl || "").replace(/\/$/, "")}${path}`;
}

async function requestToken(
  options: CreateAuthApiOptions,
  formBody: Record<string, string>
): Promise<OAuthTokenResponse> {
  const body = new URLSearchParams(formBody);
  const response = await fetch(authEndpoint(options.baseUrl, "/api/oauth2/token"), {
    method: "POST",
    headers: {
      "Content-Type": "application/x-www-form-urlencoded",
      "X-Client-Type": options.clientType,
      "X-Client-Id": options.xClientId
    },
    body
  });

  if (!response.ok) {
    const contentType = response.headers.get("content-type") || "";
    if (contentType.includes("application/problem+json")) {
      throw new Error((await response.json()).detail || `HTTP ${response.status}`);
    }
    throw new Error(`HTTP ${response.status}`);
  }

  return (await response.json()) as OAuthTokenResponse;
}
