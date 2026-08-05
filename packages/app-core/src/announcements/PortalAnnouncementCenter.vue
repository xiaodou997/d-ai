<script setup lang="ts">
import { defineAsyncComponent, ref, shallowRef } from "vue";
import { Bell } from "lucide-vue-next";

import PortalAnnouncementDrawer from "./PortalAnnouncementDrawer.vue";
import type { AnnouncementClient, PortalAnnouncement } from "./types";
import { usePortalAnnouncements } from "./usePortalAnnouncements";

const props = defineProps<{ client: AnnouncementClient }>();
const PortalAnnouncementDetail = defineAsyncComponent(() => import("./PortalAnnouncementDetail.vue"));
const PortalAnnouncementPopup = defineAsyncComponent(() => import("./PortalAnnouncementPopup.vue"));

const drawerVisible = shallowRef(false);
const selected = ref<PortalAnnouncement | null>(null);
const announcements = usePortalAnnouncements({ client: props.client });

async function selectAnnouncement(item: PortalAnnouncement) {
  selected.value = item;
  if (item.readAt === undefined) {
    await announcements.markAsRead(item.announcementId);
    selected.value = announcements.items.value.find((candidate) => candidate.announcementId === item.announcementId) ?? item;
  }
}
</script>

<template>
  <div class="announcement-center">
    <button type="button" class="announcement-center__bell" title="公告中心" @click="drawerVisible = true">
      <Bell :size="18" aria-hidden="true" />
      <span v-if="announcements.unreadCount.value > 0" class="announcement-center__badge">
        {{ announcements.unreadCount.value > 99 ? "99+" : announcements.unreadCount.value }}
      </span>
    </button>

    <PortalAnnouncementDrawer
      :visible="drawerVisible"
      :items="announcements.items.value"
      :loading="announcements.loading.value"
      :unread-count="announcements.unreadCount.value"
      @close="drawerVisible = false"
      @refresh="announcements.refresh"
      @select="selectAnnouncement"
      @mark-read="announcements.markAsRead"
    />
    <PortalAnnouncementDetail v-if="selected" :item="selected" @close="selected = null" />
    <PortalAnnouncementPopup
      v-if="announcements.currentPopup.value"
      :item="announcements.currentPopup.value"
      @acknowledge="announcements.acknowledgeCurrentPopup"
    />
  </div>
</template>

<style scoped>
.announcement-center {
  display: flex;
  align-items: center;
}

.announcement-center__bell {
  position: relative;
  display: grid;
  width: 34px;
  height: 34px;
  place-items: center;
  border: 0;
  border-radius: var(--ds-radius-control);
  background: transparent;
  color: var(--ds-muted);
  cursor: pointer;
}

.announcement-center__bell:hover {
  background: var(--ds-panel-muted);
  color: var(--ds-ink);
}

.announcement-center__badge {
  position: absolute;
  top: -3px;
  right: -5px;
  min-width: 17px;
  height: 17px;
  border: 2px solid var(--ds-panel);
  border-radius: 9px;
  background: #dc2626;
  color: #fff;
  font-size: 9px;
  font-weight: 700;
  line-height: 13px;
  text-align: center;
}
</style>
