import type { RequestAdapter } from "@/api";

export interface AuthTokenResponse {
  accessToken: string;
  expiresIn: number;
  refreshExpiresIn: number;
  mfaRequired?: boolean;
  mfaChallengeToken?: string;
}

export class MFARequiredError extends Error {
  readonly challengeToken: string;

  constructor(challengeToken: string) {
    super("请输入管理员 MFA 验证码");
    this.name = "MFARequiredError";
    this.challengeToken = challengeToken;
  }
}

export interface UserInfoResponse {
  sub: string;
  username: string;
  userType: number;
  tenantId: string;
  tenantName: string;
  mfaEnabled?: boolean;
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
    async refreshToken(): Promise<AuthTokenResponse> {
      return requestAuth(options, "/api/auth/refresh");
    },
    async verifyMFA(challengeToken: string, code: string): Promise<AuthTokenResponse> {
      return requestAuth(options, "/api/auth/mfa/verify", {
        challengeToken,
        code
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
  body?: Record<string, string>
): Promise<AuthTokenResponse> {
  const response = await fetch(authEndpoint(options.baseUrl, path), {
    method: "POST",
    headers: {
      "Content-Type": "application/json"
    },
    credentials: "include",
    ...(body === undefined ? {} : { body: JSON.stringify(body) })
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
