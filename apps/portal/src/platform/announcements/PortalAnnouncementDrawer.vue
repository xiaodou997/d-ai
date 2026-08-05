<script setup lang="ts">
import { Check, RefreshCw } from "lucide-vue-next";

import type { PortalAnnouncement } from "./types";

defineProps<{
  visible: boolean;
  items: readonly PortalAnnouncement[];
  loading: boolean;
  unreadCount: number;
}>();

const emit = defineEmits<{
  close: [];
  refresh: [];
  select: [item: PortalAnnouncement];
  markRead: [id: string];
}>();

function formatTime(value?: number) {
  if (!value) return "";
  return new Intl.DateTimeFormat("zh-CN", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit"
  }).format(value);
}

const categoryLabels: Record<PortalAnnouncement["category"], string> = {
  general: "系统公告",
  maintenance: "维护通知",
  upgrade: "升级通知",
  pricing: "费率变更",
  security: "安全通知"
};
</script>

<template>
  <el-drawer
    :model-value="visible"
    title="公告中心"
    size="min(440px, 94vw)"
    append-to-body
    @close="emit('close')"
  >
    <template #header>
      <div class="announcement-drawer__header">
        <div>
          <strong class="announcement-drawer__title">公告中心</strong>
          <span class="announcement-drawer__count">{{ unreadCount }} 条未读</span>
        </div>
        <el-button :icon="RefreshCw" circle text :loading="loading" title="刷新公告" @click="emit('refresh')" />
      </div>
    </template>

    <div v-loading="loading" class="announcement-drawer__body">
      <button
        v-for="item in items"
        :key="item.announcementId"
        type="button"
        class="announcement-row"
        :class="{ 'is-unread': item.readAt === undefined }"
        @click="emit('select', item)"
      >
        <span class="announcement-row__indicator" />
        <span class="announcement-row__content">
          <span class="announcement-row__meta">
            <span>{{ categoryLabels[item.category] }}</span>
            <span>{{ item.publisherType === 'platform' ? '平台' : '所属租户' }}</span>
            <time>{{ formatTime(item.publishedAt ?? item.createdAt) }}</time>
          </span>
          <strong class="announcement-row__title">{{ item.title }}</strong>
          <span class="announcement-row__summary">{{ item.contentMarkdown }}</span>
        </span>
        <el-button
          v-if="item.readAt === undefined"
          class="announcement-row__read"
          :icon="Check"
          circle
          text
          title="标记已读"
          @click.stop="emit('markRead', item.announcementId)"
        />
      </button>

      <el-empty v-if="!loading && items.length === 0" description="暂无公告" />
    </div>
  </el-drawer>
</template>

<style scoped>
.announcement-drawer__header {
  display: flex;
  width: 100%;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.announcement-drawer__title {
  color: var(--ds-ink);
  font-size: 16px;
}

.announcement-drawer__count {
  margin-left: 10px;
  color: var(--ds-muted);
  font-size: 12px;
}

.announcement-drawer__body {
  min-height: 180px;
}

.announcement-row {
  position: relative;
  display: grid;
  width: 100%;
  grid-template-columns: 3px minmax(0, 1fr) 32px;
  gap: 12px;
  border: 0;
  border-bottom: 1px solid var(--ds-line);
  background: transparent;
  padding: 16px 2px;
  color: var(--ds-ink);
  text-align: left;
  cursor: pointer;
}

.announcement-row:hover {
  background: var(--ds-panel-muted);
}

.announcement-row__indicator {
  width: 3px;
  border-radius: 2px;
  background: transparent;
}

.announcement-row.is-unread .announcement-row__indicator {
  background: var(--ds-accent);
}

.announcement-row__content {
  display: flex;
  min-width: 0;
  flex-direction: column;
  gap: 7px;
}

.announcement-row__meta {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  color: var(--ds-muted);
  font-size: 11px;
}

.announcement-row__title {
  overflow: hidden;
  font-size: 14px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.announcement-row__summary {
  display: -webkit-box;
  overflow: hidden;
  color: var(--ds-muted);
  font-size: 12px;
  line-height: 1.55;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
}

.announcement-row__read {
  align-self: center;
}
</style>
