import { describe, expect, it, vi } from "vitest";

import type { AnnouncementClient, AnnouncementInboxPage } from "./types";
import { usePortalAnnouncements } from "./usePortalAnnouncements";

const popupPage: AnnouncementInboxPage = {
  items: [
    {
      announcementId: "ANN_1",
      publisherType: "platform",
      title: "升级通知",
      contentMarkdown: "今晚升级",
      category: "upgrade",
      severity: "important",
      displayMode: "popup",
      status: "published",
      publishedAt: Date.now(),
      createdBy: "SA_ROOT",
      updatedBy: "SA_ROOT",
      createdAt: Date.now(),
      updatedAt: Date.now()
    }
  ],
  total: 1,
  unreadCount: 1,
  page: 1,
  size: 20
};

describe("usePortalAnnouncements", () => {
  it("keeps popup unread until the user explicitly acknowledges it", async () => {
    const markRead = vi.fn(async () => undefined);
    const client: AnnouncementClient = {
      list: vi.fn(async () => structuredClone(popupPage)),
      get: vi.fn(),
      markRead
    };
    const announcements = usePortalAnnouncements({ client, autoStart: false });

    await announcements.refresh();

    expect(announcements.currentPopup.value?.announcementId).toBe("ANN_1");
    expect(announcements.unreadCount.value).toBe(1);
    expect(markRead).not.toHaveBeenCalled();

    await announcements.acknowledgeCurrentPopup();

    expect(markRead).toHaveBeenCalledWith("ANN_1");
    expect(announcements.unreadCount.value).toBe(0);
    expect(announcements.currentPopup.value).toBeNull();
  });
});
