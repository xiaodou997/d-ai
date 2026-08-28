import { createFetchAdapter } from "@/platform";
import { createTypedOperationRequest } from ".";

import { portalEnv } from "@/env";
import type {
  ActivateAccountPayload,
  ActivateAccountResult,
  PasswordPolicy,
  PublicInvitation,
  PublicRegistrationPayload,
  PublicRegistrationResult
} from "./types/platformPublic";
import type { PublicInvitationStatus } from "./types/platformPublic";

const request = createFetchAdapter();
const typedRequest = createTypedOperationRequest(request);
const baseUrl = portalEnv.apiBaseUrl;

function toPublicInvitation(value: Awaited<ReturnType<typeof typedRequest<"public-get-invitation">>>): PublicInvitation {
  const statuses: PublicInvitationStatus[] = ["active", "expired", "disabled", "used_up", "not_found"];
  const status = statuses.includes(value.status as PublicInvitationStatus)
    ? (value.status as PublicInvitationStatus)
    : "not_found";
  return {
    code: value.code,
    tenantName: value.tenantName,
    customerSiteName: value.customerSiteName,
    faviconPath: value.faviconPath,
    description: value.description,
    expiresAt: value.expiresAt ?? null,
    status,
    canRegister: value.canRegister,
    message: value.message,
    legal: value.legal
  };
}

export const platformPublicApi = {
  getPasswordPolicy() {
    return typedRequest<"auth-password-policy">({
      method: "GET",
      path: "/api/auth/password-policy",
      baseUrl
    });
  },
  activateAccount(body: ActivateAccountPayload) {
    return typedRequest<"auth-activate-account">({
      method: "POST",
      path: "/api/auth/activate",
      body,
      baseUrl
    });
  },
  getInvitation(code: string) {
    return typedRequest<"public-get-invitation">({
      method: "GET",
      path: `/api/v1/public/invitations/${encodeURIComponent(code)}`,
      baseUrl
    }).then(toPublicInvitation);
  },
  registerInvitation(code: string, body: PublicRegistrationPayload) {
    return typedRequest<"public-register-invitation-user">({
      method: "POST",
      path: `/api/v1/public/invitations/${encodeURIComponent(code)}/registrations`,
      body,
      baseUrl
    });
  }
};
