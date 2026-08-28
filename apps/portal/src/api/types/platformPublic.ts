import type { components } from "../generated/dai";

export type PublicInvitationStatus = "active" | "expired" | "disabled" | "used_up" | "not_found";

export type PublicLegalDocuments = components["schemas"]["LegalStruct"];

export interface PublicInvitation {
  code: string;
  tenantName: string;
  customerSiteName: string;
  faviconPath?: string;
  description: string;
  expiresAt?: number | null;
  status: PublicInvitationStatus;
  canRegister: boolean;
  message: string;
  legal: PublicLegalDocuments;
}

export type PublicRegistrationPayload = components["schemas"]["PublicRegistrationInputBody"];
export type PublicRegistrationResult = components["schemas"]["PublicRegistrationOutputBody"];
export type PasswordPolicy = components["schemas"]["PasswordPolicy"];
export type ActivateAccountPayload = components["schemas"]["ActivateAccountInputBody"];
export type ActivateAccountResult = components["schemas"]["MessageOutputBody"];
