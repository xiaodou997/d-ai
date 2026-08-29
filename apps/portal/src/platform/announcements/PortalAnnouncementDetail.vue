<script setup lang="ts">
import { computed } from "vue";

import { renderAnnouncementMarkdown } from "./markdown";
import type { PortalAnnouncement } from "./types";

const props = defineProps<{
  item: PortalAnnouncement | null;
}>();

const emit = defineEmits<{ close: [] }>();

const renderedContent = computed(() => renderAnnouncementMarkdown(props.item?.contentMarkdown ?? ""));
</script>

<template>
  <el-dialog
    :model-value="item !== null"
    width="min(720px, 94vw)"
    append-to-body
    destroy-on-close
    @close="emit('close')"
  >
    <template #header>
      <div v-if="item" class="announcement-detail__header">
        <span class="announcement-detail__source">{{ item.publisherType === "platform" ? "平台公告" : "租户公告" }}</span>
        <h2 class="announcement-detail__title">{{ item.title }}</h2>
      </div>
    </template>
    <article v-if="item" class="announcement-detail">
      <div class="announcement-detail__meta">
        <span>{{ item.category }}</span>
        <time>{{ new Date(item.publishedAt ?? item.createdAt).toLocaleString("zh-CN") }}</time>
      </div>
      <div class="announcement-markdown" v-html="renderedContent" />
    </article>
  </el-dialog>
</template>

<style scoped>
.announcement-detail__header {
  padding-right: 32px;
}

.announcement-detail__source {
  color: var(--ds-accent);
  font-size: 12px;
  font-weight: 600;
}

.announcement-detail__title {
  margin: 6px 0 0;
  color: var(--ds-ink);
  font-size: 20px;
  line-height: 1.35;
}

.announcement-detail__meta {
  display: flex;
  gap: 12px;
  margin-bottom: 20px;
  color: var(--ds-muted);
  font-size: 12px;
}

.announcement-markdown {
  color: var(--ds-ink);
  font-size: 14px;
  line-height: 1.75;
  overflow-wrap: anywhere;
}

.announcement-markdown :deep(a) {
  color: var(--ds-accent);
}

.announcement-markdown :deep(pre) {
  overflow-x: auto;
  border: 1px solid var(--ds-line);
  border-radius: var(--ds-radius-sm);
  background: var(--ds-panel-muted);
  padding: 12px;
}
</style>
