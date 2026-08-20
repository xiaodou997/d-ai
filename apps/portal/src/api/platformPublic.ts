import { createFetchAdapter } from "@/platform";

import { portalEnv } from "@/env";
import type {
  ActivateAccountPayload,
  ActivateAccountResult,
  PasswordPolicy,
  PublicInvitation,
  PublicRegistrationPayload,
  PublicRegistrationResult
} from "./types/platformPublic";

const request = createFetchAdapter();
const baseUrl = portalEnv.apiBaseUrl;

export const platformPublicApi = {
  getPasswordPolicy() {
    return request<PasswordPolicy>({
      method: "GET",
      path: "/api/auth/password-policy",
      baseUrl
    });
  },
  activateAccount(body: ActivateAccountPayload) {
    return request<ActivateAccountResult>({
      method: "POST",
      path: "/api/auth/activate",
      body,
      baseUrl
    });
  },
  getInvitation(code: string) {
    return request<PublicInvitation>({
      method: "GET",
      path: `/api/v1/public/invitations/${encodeURIComponent(code)}`,
      baseUrl
    });
  },
  registerInvitation(code: string, body: PublicRegistrationPayload) {
    return request<PublicRegistrationResult>({
      method: "POST",
      path: `/api/v1/public/invitations/${encodeURIComponent(code)}/registrations`,
      body,
      baseUrl
    });
  }
};
