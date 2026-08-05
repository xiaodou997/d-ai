import type { RequestAdapter } from "@dai/api-client";

export type AnnouncementPublisherType = "platform" | "tenant";
export type AnnouncementCategory = "general" | "maintenance" | "upgrade" | "pricing" | "security";
export type AnnouncementSeverity = "info" | "important" | "critical";
export type AnnouncementDisplayMode = "inbox" | "popup";
export type AnnouncementStatus = "draft" | "published" | "archived";
export type AnnouncementAudienceKind = "admin" | "tenant_user" | "end_user";
export type AnnouncementAudienceScope = "all" | "tenant";

export interface AnnouncementAudienceRule {
  kind: AnnouncementAudienceKind;
  scope: AnnouncementAudienceScope;
  tenantId?: string;
}

export interface AnnouncementAudienceSelection {
  kind: AnnouncementAudienceKind;
  scope: AnnouncementAudienceScope;
  tenantIds?: string[];
}

export interface PortalAnnouncement {
  announcementId: string;
  publisherType: AnnouncementPublisherType;
  publisherTenantId?: string;
  title: string;
  contentMarkdown: string;
  category: AnnouncementCategory;
  severity: AnnouncementSeverity;
  displayMode: AnnouncementDisplayMode;
  status: AnnouncementStatus;
  startsAt?: number;
  endsAt?: number;
  publishedAt?: number;
  archivedAt?: number;
  audienceSizeAtPublish?: number;
  createdBy: string;
  updatedBy: string;
  createdAt: number;
  updatedAt: number;
  readAt?: number;
  audiences?: readonly AnnouncementAudienceRule[];
}

export type ManagedAnnouncement = PortalAnnouncement;

export interface AnnouncementDraftPayload {
  title: string;
  contentMarkdown: string;
  category: AnnouncementCategory;
  severity: AnnouncementSeverity;
  displayMode: AnnouncementDisplayMode;
  startsAt?: number;
  endsAt?: number;
  audiences?: AnnouncementAudienceSelection[];
}

export interface ManagedAnnouncementPage {
  items: ManagedAnnouncement[];
  total: number;
  page: number;
  size: number;
}

export interface AnnouncementStats {
  audienceSizeAtPublish: number;
  currentAudienceSize: number;
  readCount: number;
}

export interface AnnouncementRecipient {
  userType: number;
  userId: string;
  tenantId?: string;
  username: string;
  email?: string;
  readAt?: number;
}

export interface AnnouncementRecipientPage {
  items: AnnouncementRecipient[];
  total: number;
  page: number;
  size: number;
}

export interface AnnouncementManagementClient {
  list(query?: { status?: AnnouncementStatus; search?: string; page?: number; size?: number }): Promise<ManagedAnnouncementPage>;
  get(id: string): Promise<ManagedAnnouncement>;
  create(payload: AnnouncementDraftPayload): Promise<ManagedAnnouncement>;
  update(id: string, payload: AnnouncementDraftPayload): Promise<ManagedAnnouncement>;
  deleteDraft(id: string): Promise<void>;
  publish(id: string): Promise<ManagedAnnouncement>;
  archive(id: string): Promise<ManagedAnnouncement>;
  stats(id: string): Promise<AnnouncementStats>;
  recipients(id: string, query?: { search?: string; page?: number; size?: number }): Promise<AnnouncementRecipientPage>;
}

export interface CreateAnnouncementManagementClientOptions extends CreateAnnouncementClientOptions {
  basePath: "/api/v1/admin/announcements" | "/api/v1/tenants/me/announcements";
}

export interface AnnouncementTenantOption {
  tenantId: string;
  tenantName: string;
}

export type AnnouncementTenantLoader = (keyword: string) => Promise<AnnouncementTenantOption[]>;

export interface AnnouncementInboxPage {
  items: PortalAnnouncement[];
  total: number;
  unreadCount: number;
  page: number;
  size: number;
}

export interface AnnouncementClient {
  list(query?: { page?: number; size?: number; unreadOnly?: boolean; displayMode?: AnnouncementDisplayMode }): Promise<AnnouncementInboxPage>;
  get(id: string): Promise<PortalAnnouncement>;
  markRead(id: string): Promise<void>;
}

export interface CreateAnnouncementClientOptions {
  request: RequestAdapter;
  baseUrl: string;
  headers?: Record<string, string>;
}
