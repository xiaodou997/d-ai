import { computed, onBeforeUnmount, onMounted, readonly, ref, shallowRef } from "vue";

import type { AnnouncementClient, PortalAnnouncement } from "./types";

export interface UsePortalAnnouncementsOptions {
  client: AnnouncementClient;
  autoStart?: boolean;
  pollIntervalMs?: number;
}
export function usePortalAnnouncements(options: UsePortalAnnouncementsOptions) {
  const items = ref<PortalAnnouncement[]>([]);
  const loading = shallowRef(false);
  const error = shallowRef<unknown>(null);
  const unreadCount = shallowRef(0);
  let pollTimer: number | undefined;

  const currentPopup = computed(
    () => items.value.find((item) => item.displayMode === "popup" && item.readAt === undefined) ?? null
  );

  async function refresh() {
    if (loading.value) return;
    loading.value = true;
    try {
      const page = await options.client.list({ page: 1, size: 100 });
      items.value = page.items ?? [];
      unreadCount.value = page.unreadCount;
      error.value = null;
    } catch (cause) {
      error.value = cause;
    } finally {
      loading.value = false;
    }
  }

  async function markAsRead(id: string) {
    const target = items.value.find((item) => item.announcementId === id);
    if (!target || target.readAt !== undefined) return;
    await options.client.markRead(id);
    const readAt = Date.now();
    items.value = items.value.map((item) =>
      item.announcementId === id ? { ...item, readAt } : item
    );
    unreadCount.value = Math.max(0, unreadCount.value - 1);
  }

  async function acknowledgeCurrentPopup() {
    const popup = currentPopup.value;
    if (!popup) return;
    await markAsRead(popup.announcementId);
  }

  function stopPolling() {
    if (pollTimer !== undefined) {
      window.clearInterval(pollTimer);
      pollTimer = undefined;
    }
  }

  function startPolling() {
    if (typeof window === "undefined" || document.hidden || pollTimer !== undefined) return;
    pollTimer = window.setInterval(() => void refresh(), options.pollIntervalMs ?? 60_000);
  }

  function handleVisibilityChange() {
    if (document.hidden) {
      stopPolling();
      return;
    }
    void refresh();
    startPolling();
  }

  if (options.autoStart ?? true) {
    onMounted(() => {
      void refresh();
      startPolling();
      document.addEventListener("visibilitychange", handleVisibilityChange);
    });
    onBeforeUnmount(() => {
      stopPolling();
      document.removeEventListener("visibilitychange", handleVisibilityChange);
    });
  }

  return {
    items: readonly(items),
    loading: readonly(loading),
    error: readonly(error),
    unreadCount: readonly(unreadCount),
    currentPopup,
    refresh,
    markAsRead,
    acknowledgeCurrentPopup
  };
}
