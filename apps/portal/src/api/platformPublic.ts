import { createFetchAdapter } from "@/platform";

import { portalEnv } from "@/env";
import type {
  PublicInvitation,
  PublicRegistrationPayload,
  PublicRegistrationResult
} from "./types/platformPublic";

const request = createFetchAdapter();
const baseUrl = portalEnv.apiBaseUrl;

export const platformPublicApi = {
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
