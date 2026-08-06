import type { RequestAdapter } from "@/api";

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
  tenantId: string;
  tenantName: string;
}

export interface CreateAuthApiOptions {
  request: RequestAdapter;
  baseUrl: string;
}

export function createPortalAuthApi(options: CreateAuthApiOptions) {
  return {
    async login(username: string, password: string): Promise<OAuthTokenResponse> {
      return requestToken(options, {
        grant_type: "password",
        username: username.trim(),
        password
      });
    },
    async refreshToken(refreshToken: string): Promise<OAuthTokenResponse> {
      return requestToken(options, {
        grant_type: "refresh_token",
        refresh_token: refreshToken
      });
    },
    async logout(): Promise<{ success?: boolean; message?: string }> {
      return options.request({
        method: "POST",
        path: "/api/oauth2/revoke",
        baseUrl: options.baseUrl
      });
    },
    async getCurrentUser(): Promise<UserInfoResponse> {
      return options.request({
        method: "GET",
        path: "/api/oauth2/userinfo",
        baseUrl: options.baseUrl
      });
    }
  };
}

// 支持相对或绝对 API 基址；相对基址不能直接交给 new URL。
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
      "Content-Type": "application/x-www-form-urlencoded"
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
