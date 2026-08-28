import {
  createTypedOperationRequest,
  type OperationBody,
  type OperationQuery,
  type OperationResponse
} from "@/api";
import type { components } from "@/api/generated/dai";
import type {
  AnnouncementAudienceKind,
  AnnouncementAudienceRule,
  AnnouncementAudienceScope,
  AnnouncementCategory,
  AnnouncementClient,
  AnnouncementDisplayMode,
  AnnouncementDraftPayload,
  AnnouncementInboxPage,
  AnnouncementManagementClient,
  AnnouncementRecipient,
  AnnouncementRecipientPage,
  AnnouncementSeverity,
  AnnouncementStats,
  AnnouncementStatus,
  CreateAnnouncementManagementClientOptions,
  CreateAnnouncementClientOptions,
  ManagedAnnouncement,
  ManagedAnnouncementPage,
  PortalAnnouncement
} from "./types";

type AnnouncementTransport = components["schemas"]["AnnouncementOutputItem"];
type AudienceRuleTransport = components["schemas"]["AudienceRuleOutput"];
type RecipientTransport = components["schemas"]["AnnouncementRecipientOutput"];

const publisherTypes = ["platform", "tenant"] as const;
const categories = ["general", "maintenance", "upgrade", "pricing", "security"] as const;
const severities = ["info", "important", "critical"] as const;
const displayModes = ["inbox", "popup"] as const;
const statuses = ["draft", "published", "archived"] as const;
const audienceKinds = ["admin", "tenant_user", "end_user"] as const;
const audienceScopes = ["all", "tenant"] as const;

function oneOf<T extends string>(value: string, allowed: readonly T[], label: string): T {
  if ((allowed as readonly string[]).includes(value)) return value as T;
  throw new Error(`Unexpected ${label}: ${value}`);
}

function toAudienceRule(value: AudienceRuleTransport): AnnouncementAudienceRule {
  return {
    kind: oneOf<AnnouncementAudienceKind>(value.kind, audienceKinds, "announcement audience kind"),
    scope: oneOf<AnnouncementAudienceScope>(value.scope, audienceScopes, "announcement audience scope"),
    tenantId: value.tenantId
  };
}

function toAnnouncement(value: AnnouncementTransport): PortalAnnouncement {
  return {
    announcementId: value.announcementId,
    publisherType: oneOf(value.publisherType, publisherTypes, "announcement publisher type"),
    publisherTenantId: value.publisherTenantId,
    title: value.title,
    contentMarkdown: value.contentMarkdown,
    category: oneOf<AnnouncementCategory>(value.category, categories, "announcement category"),
    severity: oneOf<AnnouncementSeverity>(value.severity, severities, "announcement severity"),
    displayMode: oneOf<AnnouncementDisplayMode>(value.displayMode, displayModes, "announcement display mode"),
    status: oneOf<AnnouncementStatus>(value.status, statuses, "announcement status"),
    startsAt: value.startsAt,
    endsAt: value.endsAt,
    publishedAt: value.publishedAt,
    archivedAt: value.archivedAt,
    audienceSizeAtPublish: value.audienceSizeAtPublish,
    createdBy: value.createdBy,
    updatedBy: value.updatedBy,
    createdAt: value.createdAt,
    updatedAt: value.updatedAt,
    readAt: value.readAt,
    audiences: value.audiences?.map(toAudienceRule) ?? undefined
  };
}

function toAnnouncementPage(value: OperationResponse<"admin-list-announcements">): ManagedAnnouncementPage {
  return {
    items: value.items?.map(toAnnouncement) ?? [],
    total: value.total,
    page: value.page,
    size: value.size
  };
}

function toInboxPage(value: OperationResponse<"list-my-announcements">): AnnouncementInboxPage {
  return {
    items: value.items?.map(toAnnouncement) ?? [],
    total: value.total,
    unreadCount: value.unreadCount,
    page: value.page,
    size: value.size
  };
}

function toRecipient(value: RecipientTransport): AnnouncementRecipient {
  return {
    userType: value.userType,
    userId: value.userId,
    tenantId: value.tenantId,
    username: value.username,
    email: value.email,
    readAt: value.readAt
  };
}

function toRecipientPage(
  value: OperationResponse<"admin-list-announcement-recipients">
): AnnouncementRecipientPage {
  return {
    items: value.items?.map(toRecipient) ?? [],
    total: value.total,
    page: value.page,
    size: value.size
  };
}

function toStats(value: OperationResponse<"admin-get-announcement-stats">): AnnouncementStats {
  return {
    audienceSizeAtPublish: value.audienceSizeAtPublish,
    currentAudienceSize: value.currentAudienceSize,
    readCount: value.readCount
  };
}

function toDraft(value: AnnouncementDraftPayload): OperationBody<"admin-create-announcement"> {
  return {
    title: value.title,
    contentMarkdown: value.contentMarkdown,
    category: value.category,
    severity: value.severity,
    displayMode: value.displayMode,
    startsAt: value.startsAt,
    endsAt: value.endsAt,
    audiences: value.audiences?.map((audience) => ({
      kind: audience.kind,
      scope: audience.scope,
      tenantIds: audience.tenantIds
    }))
  };
}

export function createPortalAnnouncementClient(options: CreateAnnouncementClientOptions): AnnouncementClient {
  const request = createTypedOperationRequest(options.request);
  return {
    list(query: OperationQuery<"list-my-announcements"> = {}) {
      return request<"list-my-announcements">({
        method: "GET",
        path: "/api/v1/announcements",
        query,
        headers: options.headers,
        baseUrl: options.baseUrl
      }).then(toInboxPage);
    },
    get(id) {
      return request<"get-my-announcement">({
        method: "GET",
        path: `/api/v1/announcements/${encodeURIComponent(id)}`,
        pathParams: { id },
        headers: options.headers,
        baseUrl: options.baseUrl
      }).then(toAnnouncement);
    },
    async markRead(id) {
      await request<"mark-announcement-read">({
        method: "POST",
        path: `/api/v1/announcements/${encodeURIComponent(id)}/read`,
        pathParams: { id },
        headers: options.headers,
        baseUrl: options.baseUrl
      });
    }
  };
}

export function createAnnouncementManagementClient(
  options: CreateAnnouncementManagementClientOptions
): AnnouncementManagementClient {
  const request = createTypedOperationRequest(options.request);
  const isAdmin = options.basePath === "/api/v1/admin/announcements";
  const path = (suffix = "") => `${options.basePath}${suffix}`;
  const common = { headers: options.headers, baseUrl: options.baseUrl };

  return {
    list(query: OperationQuery<"admin-list-announcements"> = {}) {
      if (isAdmin) {
        return request<"admin-list-announcements">({
          method: "GET",
          path: path(),
          query,
          ...common
        }).then(toAnnouncementPage);
      }
      return request<"tenant-list-announcements">({
        method: "GET",
        path: path(),
        query,
        ...common
      }).then(toAnnouncementPage);
    },
    get(id) {
      if (isAdmin) {
        return request<"admin-get-announcement">({
          method: "GET",
          path: path(`/${encodeURIComponent(id)}`),
          pathParams: { id },
          ...common
        }).then(toAnnouncement);
      }
      return request<"tenant-get-announcement">({
        method: "GET",
        path: path(`/${encodeURIComponent(id)}`),
        pathParams: { id },
        ...common
      }).then(toAnnouncement);
    },
    create(payload) {
      const body = toDraft(payload);
      if (isAdmin) {
        return request<"admin-create-announcement">({ method: "POST", path: path(), body, ...common }).then(toAnnouncement);
      }
      return request<"tenant-create-announcement">({ method: "POST", path: path(), body, ...common }).then(toAnnouncement);
    },
    update(id, payload) {
      const body = toDraft(payload);
      if (isAdmin) {
        return request<"admin-update-announcement">({
          method: "PUT",
          path: path(`/${encodeURIComponent(id)}`),
          pathParams: { id },
          body,
          ...common
        }).then(toAnnouncement);
      }
      return request<"tenant-update-announcement">({
        method: "PUT",
        path: path(`/${encodeURIComponent(id)}`),
        pathParams: { id },
        body,
        ...common
      }).then(toAnnouncement);
    },
    async deleteDraft(id) {
      if (isAdmin) {
        await request<"admin-delete-announcement">({
          method: "DELETE",
          path: path(`/${encodeURIComponent(id)}`),
          pathParams: { id },
          ...common
        });
        return;
      }
      await request<"tenant-delete-announcement">({
        method: "DELETE",
        path: path(`/${encodeURIComponent(id)}`),
        pathParams: { id },
        ...common
      });
    },
    publish(id) {
      if (isAdmin) {
        return request<"admin-publish-announcement">({
          method: "POST",
          path: path(`/${encodeURIComponent(id)}/publish`),
          pathParams: { id },
          ...common
        }).then(toAnnouncement);
      }
      return request<"tenant-publish-announcement">({
        method: "POST",
        path: path(`/${encodeURIComponent(id)}/publish`),
        pathParams: { id },
        ...common
      }).then(toAnnouncement);
    },
    archive(id) {
      if (isAdmin) {
        return request<"admin-archive-announcement">({
          method: "POST",
          path: path(`/${encodeURIComponent(id)}/archive`),
          pathParams: { id },
          ...common
        }).then(toAnnouncement);
      }
      return request<"tenant-archive-announcement">({
        method: "POST",
        path: path(`/${encodeURIComponent(id)}/archive`),
        pathParams: { id },
        ...common
      }).then(toAnnouncement);
    },
    stats(id) {
      if (isAdmin) {
        return request<"admin-get-announcement-stats">({
          method: "GET",
          path: path(`/${encodeURIComponent(id)}/stats`),
          pathParams: { id },
          ...common
        }).then(toStats);
      }
      return request<"tenant-get-announcement-stats">({
        method: "GET",
        path: path(`/${encodeURIComponent(id)}/stats`),
        pathParams: { id },
        ...common
      }).then(toStats);
    },
    recipients(id, query: OperationQuery<"admin-list-announcement-recipients"> = {}) {
      if (isAdmin) {
        return request<"admin-list-announcement-recipients">({
          method: "GET",
          path: path(`/${encodeURIComponent(id)}/recipients`),
          pathParams: { id },
          query,
          ...common
        }).then(toRecipientPage);
      }
      return request<"tenant-list-announcement-recipients">({
        method: "GET",
        path: path(`/${encodeURIComponent(id)}/recipients`),
        pathParams: { id },
        query,
        ...common
      }).then(toRecipientPage);
    }
  };
}
