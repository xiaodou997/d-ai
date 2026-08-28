import { createTypedOperationRequest, type RequestAdapter } from "@/api";
import type { components } from "@/api/generated/dai";

export type AuthTokenResponse = components["schemas"]["AuthTokenResponse"];
export type UserInfoResponse = components["schemas"]["UserInfoOutputBody"];
export type AuthLogoutResponse = components["schemas"]["AuthLogoutOutputBody"];

export class MFARequiredError extends Error {
  readonly challengeToken: string;

  constructor(challengeToken: string) {
    super("请输入管理员 MFA 验证码");
    this.name = "MFARequiredError";
    this.challengeToken = challengeToken;
  }
}

export interface CreateAuthApiOptions {
  request: RequestAdapter;
  baseUrl: string;
}

export function createPortalAuthApi(options: CreateAuthApiOptions) {
  const request = createTypedOperationRequest(options.request);
  return {
    async login(username: string, password: string): Promise<AuthTokenResponse> {
      return request<"auth-login">({
        method: "POST",
        path: "/api/auth/login",
        body: { username: username.trim(), password },
        baseUrl: options.baseUrl
      });
    },
    async refreshToken(): Promise<AuthTokenResponse> {
      return request<"auth-refresh">({
        method: "POST",
        path: "/api/auth/refresh",
        baseUrl: options.baseUrl
      });
    },
    async verifyMFA(challengeToken: string, code: string): Promise<AuthTokenResponse> {
      return request<"auth-mfa-verify">({
        method: "POST",
        path: "/api/auth/mfa/verify",
        body: { challengeToken, code },
        baseUrl: options.baseUrl
      });
    },
    async logout(): Promise<AuthLogoutResponse> {
      return request<"auth-logout">({
        method: "POST",
        path: "/api/auth/logout",
        baseUrl: options.baseUrl
      });
    },
    async getCurrentUser(): Promise<UserInfoResponse> {
      return request<"auth-current-user">({
        method: "GET",
        path: "/api/auth/me",
        baseUrl: options.baseUrl
      });
    }
  };
}
