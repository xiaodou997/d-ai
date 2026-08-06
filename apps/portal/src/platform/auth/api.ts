import type { RequestAdapter } from "@/api";

export interface AuthTokenResponse {
  accessToken: string;
  refreshToken?: string;
  expiresIn: number;
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
    async login(username: string, password: string): Promise<AuthTokenResponse> {
      return requestAuth(options, "/api/auth/login", {
        username: username.trim(),
        password
      });
    },
    async refreshToken(refreshToken: string): Promise<AuthTokenResponse> {
      return requestAuth(options, "/api/auth/refresh", {
        refreshToken
      });
    },
    async logout(): Promise<{ success?: boolean; message?: string }> {
      return options.request({
        method: "POST",
        path: "/api/auth/logout",
        baseUrl: options.baseUrl
      });
    },
    async getCurrentUser(): Promise<UserInfoResponse> {
      return options.request({
        method: "GET",
        path: "/api/auth/me",
        baseUrl: options.baseUrl
      });
    }
  };
}

// 支持相对或绝对 API 基址；相对基址不能直接交给 new URL。
function authEndpoint(baseUrl: string, path: string): string {
  return `${(baseUrl || "").replace(/\/$/, "")}${path}`;
}

async function requestAuth(
  options: CreateAuthApiOptions,
  path: string,
  body: Record<string, string>
): Promise<AuthTokenResponse> {
  const response = await fetch(authEndpoint(options.baseUrl, path), {
    method: "POST",
    headers: {
      "Content-Type": "application/json"
    },
    body: JSON.stringify(body)
  });

  if (!response.ok) {
    const contentType = response.headers.get("content-type") || "";
    if (contentType.includes("application/problem+json")) {
      throw new Error((await response.json()).detail || `HTTP ${response.status}`);
    }
    throw new Error(`HTTP ${response.status}`);
  }

  return (await response.json()) as AuthTokenResponse;
}
