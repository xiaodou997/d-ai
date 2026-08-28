import { beforeEach, describe, expect, it, vi } from "vitest";

import { createAnnouncementManagementClient, createPortalAnnouncementClient } from "./api";

const request = vi.fn();
const common = {
  request,
  baseUrl: "/api",
  headers: { Accept: "application/json" }
};

const announcement = {
  announcementId: "announcement-1",
  publisherType: "platform",
  publisherTenantId: "tenant-1",
  title: "Maintenance",
  contentMarkdown: "Scheduled maintenance",
  category: "maintenance",
  severity: "important",
  displayMode: "inbox",
  status: "published",
  startsAt: 100,
  endsAt: 200,
  publishedAt: 110,
  archivedAt: undefined,
  audienceSizeAtPublish: 10,
  createdBy: "admin-1",
  updatedBy: "admin-1",
  createdAt: 90,
  updatedAt: 110,
  readAt: undefined,
  audiences: [{ kind: "tenant_user", scope: "tenant", tenantId: "tenant-1" }],
  $schema: "ignored"
};

beforeEach(() => request.mockReset());

describe("announcement generated operation clients", () => {
  it("maps inbox responses and forwards encoded paths", async () => {
    request
      .mockResolvedValueOnce({ items: null, total: 0, unreadCount: 0, page: 1, size: 20, $schema: "ignored" })
      .mockResolvedValueOnce(announcement)
      .mockResolvedValueOnce({ message: "ok" });
    const client = createPortalAnnouncementClient(common);

    await expect(client.list({ page: 1, size: 20, unreadOnly: true, displayMode: "popup" })).resolves.toEqual({
      items: [],
      total: 0,
      unreadCount: 0,
      page: 1,
      size: 20
    });
    await expect(client.get("announcement/1")).resolves.toMatchObject({
      announcementId: "announcement-1",
      publisherType: "platform",
      audiences: [{ kind: "tenant_user", scope: "tenant", tenantId: "tenant-1" }]
    });
    await expect(client.markRead("announcement/1")).resolves.toBeUndefined();

    expect(request.mock.calls[0]?.[0]).toMatchObject({
      path: "/api/v1/announcements",
      query: { page: 1, size: 20, unreadOnly: true, displayMode: "popup" }
    });
    expect(request.mock.calls[1]?.[0]).toMatchObject({
      path: "/api/v1/announcements/announcement%2F1",
      pathParams: { id: "announcement/1" }
    });
    expect(request.mock.calls[2]?.[0]).toMatchObject({
      path: "/api/v1/announcements/announcement%2F1/read",
      pathParams: { id: "announcement/1" }
    });
  });

  it("binds admin management operations and maps draft bodies", async () => {
    request
      .mockResolvedValueOnce({ items: [announcement], total: 1, page: 1, size: 20 })
      .mockResolvedValueOnce(announcement)
      .mockResolvedValueOnce(announcement)
      .mockResolvedValueOnce(announcement)
      .mockResolvedValueOnce({ message: "deleted" })
      .mockResolvedValueOnce(announcement)
      .mockResolvedValueOnce(announcement)
      .mockResolvedValueOnce({ audienceSizeAtPublish: 10, currentAudienceSize: 8, readCount: 2 })
      .mockResolvedValueOnce({ items: null, total: 0, page: 1, size: 20 });
    const client = createAnnouncementManagementClient({
      ...common,
      basePath: "/api/v1/admin/announcements"
    });
    const payload = {
      title: "Maintenance",
      contentMarkdown: "Scheduled maintenance",
      category: "maintenance" as const,
      severity: "important" as const,
      displayMode: "inbox" as const,
      audiences: [{ kind: "tenant_user" as const, scope: "tenant" as const, tenantIds: ["tenant-1"] }]
    };

    await expect(client.list({ status: "published", page: 1, size: 20 })).resolves.toMatchObject({
      items: [{ announcementId: "announcement-1" }]
    });
    await expect(client.get("announcement/1")).resolves.toMatchObject({ status: "published" });
    await expect(client.create(payload)).resolves.toMatchObject({ title: "Maintenance" });
    await expect(client.update("announcement/1", payload)).resolves.toMatchObject({ title: "Maintenance" });
    await expect(client.deleteDraft("announcement/1")).resolves.toBeUndefined();
    await expect(client.publish("announcement/1")).resolves.toMatchObject({ status: "published" });
    await expect(client.archive("announcement/1")).resolves.toMatchObject({ status: "published" });
    await expect(client.stats("announcement/1")).resolves.toEqual({
      audienceSizeAtPublish: 10,
      currentAudienceSize: 8,
      readCount: 2
    });
    await expect(client.recipients("announcement/1", { page: 1, size: 20 })).resolves.toEqual({
      items: [],
      total: 0,
      page: 1,
      size: 20
    });

    expect(request.mock.calls[2]?.[0]).toMatchObject({
      path: "/api/v1/admin/announcements",
      body: {
        title: "Maintenance",
        category: "maintenance",
        audiences: [{ kind: "tenant_user", scope: "tenant", tenantIds: ["tenant-1"] }]
      }
    });
    expect(request.mock.calls[3]?.[0]).toMatchObject({
      path: "/api/v1/admin/announcements/announcement%2F1",
      pathParams: { id: "announcement/1" }
    });
    expect(request.mock.calls[8]?.[0]).toMatchObject({
      path: "/api/v1/admin/announcements/announcement%2F1/recipients",
      pathParams: { id: "announcement/1" },
      query: { page: 1, size: 20 }
    });
  });

  it("selects tenant operations and rejects invalid generated states", async () => {
    request.mockResolvedValueOnce({ items: null, total: 0, page: 1, size: 20 });
    const client = createAnnouncementManagementClient({
      ...common,
      basePath: "/api/v1/tenants/me/announcements"
    });
    await expect(client.list()).resolves.toMatchObject({ items: [] });
    expect(request.mock.calls[0]?.[0]).toMatchObject({
      path: "/api/v1/tenants/me/announcements"
    });

    request.mockReset();
    request.mockResolvedValueOnce({ ...announcement, status: "unknown" });
    await expect(client.get("announcement-1")).rejects.toThrow("Unexpected announcement status: unknown");
  });
});
