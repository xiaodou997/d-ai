export type PublicInvitationStatus = "active" | "expired" | "disabled" | "used_up" | "not_found";

export interface PublicLegalDocuments {
  termsUrl: string;
  termsVersion: string;
  privacyUrl: string;
  privacyVersion: string;
}

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

export interface PublicRegistrationPayload {
  username: string;
  password: string;
  email?: string;
  phone?: string;
  termsVersion: string;
  privacyVersion: string;
}

export interface PublicRegistrationResult {
  success: boolean;
  userId: string;
  sessionEstablished: boolean;
  message: string;
}
