import type {
  AnnouncementClient,
  AnnouncementDraftPayload,
  AnnouncementInboxPage,
  AnnouncementManagementClient,
  AnnouncementRecipientPage,
  AnnouncementStats,
  CreateAnnouncementManagementClientOptions,
  ManagedAnnouncement,
  ManagedAnnouncementPage,
  CreateAnnouncementClientOptions,
  PortalAnnouncement
} from "./types";

export function createPortalAnnouncementClient(options: CreateAnnouncementClientOptions): AnnouncementClient {
  return {
    list(query = {}) {
      return options.request<AnnouncementInboxPage>({
        method: "GET",
        path: "/api/v1/announcements",
        query,
        headers: options.headers,
        baseUrl: options.baseUrl
      });
    },
    get(id) {
      return options.request<PortalAnnouncement>({
        method: "GET",
        path: `/api/v1/announcements/${encodeURIComponent(id)}`,
        headers: options.headers,
        baseUrl: options.baseUrl
      });
    },
    async markRead(id) {
      await options.request<{ message: string }>({
        method: "POST",
        path: `/api/v1/announcements/${encodeURIComponent(id)}/read`,
        headers: options.headers,
        baseUrl: options.baseUrl
      });
    }
  };
}

export function createAnnouncementManagementClient(
  options: CreateAnnouncementManagementClientOptions
): AnnouncementManagementClient {
  const path = (suffix = "") => `${options.basePath}${suffix}`;
  const request = options.request;
  const common = { headers: options.headers, baseUrl: options.baseUrl };

  return {
    list(query = {}) {
      return request<ManagedAnnouncementPage>({ method: "GET", path: path(), query, ...common });
    },
    get(id) {
      return request<ManagedAnnouncement>({ method: "GET", path: path(`/${encodeURIComponent(id)}`), ...common });
    },
    create(payload: AnnouncementDraftPayload) {
      return request<ManagedAnnouncement>({ method: "POST", path: path(), body: payload, ...common });
    },
    update(id, payload) {
      return request<ManagedAnnouncement>({ method: "PUT", path: path(`/${encodeURIComponent(id)}`), body: payload, ...common });
    },
    async deleteDraft(id) {
      await request<{ message: string }>({ method: "DELETE", path: path(`/${encodeURIComponent(id)}`), ...common });
    },
    publish(id) {
      return request<ManagedAnnouncement>({ method: "POST", path: path(`/${encodeURIComponent(id)}/publish`), ...common });
    },
    archive(id) {
      return request<ManagedAnnouncement>({ method: "POST", path: path(`/${encodeURIComponent(id)}/archive`), ...common });
    },
    stats(id) {
      return request<AnnouncementStats>({ method: "GET", path: path(`/${encodeURIComponent(id)}/stats`), ...common });
    },
    recipients(id, query = {}) {
      return request<AnnouncementRecipientPage>({
        method: "GET",
        path: path(`/${encodeURIComponent(id)}/recipients`),
        query,
        ...common
      });
    }
  };
}
