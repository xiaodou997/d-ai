<script setup lang="ts">
import { computed } from "vue";

import type { PortalChatSessionRecord } from "./types";

const emit = defineEmits<{
  (e: "new-session"): void;
  (e: "select-session", id: string): void;
  (e: "remove-session", session: PortalChatSessionRecord): void;
}>();

const props = withDefaults(
  defineProps<{
    sessions?: PortalChatSessionRecord[];
    loading?: boolean;
    selectedSessionId?: string;
    createIcon?: unknown;
    deleteIcon?: unknown;
  }>(),
  {
    sessions: () => [],
    loading: false,
    selectedSessionId: ""
  }
);

const sessions = computed(() => props.sessions || []);
const selectedSessionId = computed(() => props.selectedSessionId || "");
const loading = computed(() => props.loading || false);

function sessionSubtitle(session: PortalChatSessionRecord) {
  return session.model_code || "未选择模型";
}
</script>

<template>
  <aside class="session-rail">
    <div class="rail-head">
      <span>历史对话</span>
      <el-button link type="primary" :icon="createIcon" @click="emit('new-session')">新建</el-button>
    </div>
    <div v-loading="loading" class="session-list">
      <button
        v-for="session in sessions"
        :key="session.id"
        class="session-item"
        :class="{ active: session.id === selectedSessionId }"
        type="button"
        @click="emit('select-session', session.id)"
      >
        <span>{{ session.title || "新对话" }}</span>
        <small>{{ sessionSubtitle(session) }}</small>
        <el-tooltip content="删除对话" placement="top">
          <el-button
            link
            type="danger"
            :icon="deleteIcon"
            aria-label="删除对话"
            @click.stop="emit('remove-session', session)"
          />
        </el-tooltip>
      </button>
      <div v-if="sessions.length === 0" class="empty-session">暂无历史对话</div>
    </div>
  </aside>
</template>

<style scoped>
.session-rail {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  padding: 16px;
  background: var(--ds-white);
  border: 1px solid var(--ds-line);
  border-radius: var(--ds-radius-shell);
}

.rail-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 12px;
  color: var(--ds-ink-soft);
  font-size: 13px;
  font-weight: 900;
}

.session-list {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  gap: 8px;
  overflow-y: auto;
}

.session-item {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  gap: 2px 8px;
  width: 100%;
  padding: 10px;
  text-align: left;
  border: 1px solid var(--ds-line);
  border-radius: var(--ds-radius-control);
  background: var(--ds-panel-muted);
  cursor: pointer;
}

.session-item.active {
  border-color: var(--ds-accent);
  background: var(--ds-accent-soft);
}

.session-item span {
  overflow: hidden;
  color: var(--ds-ink);
  font-size: 13px;
  font-weight: 800;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.session-item small {
  overflow: hidden;
  color: var(--ds-muted);
  font-size: 11px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.session-item :deep(.el-button) {
  grid-row: 1 / 3;
  grid-column: 2;
}

.empty-session {
  display: grid;
  place-items: center;
  min-height: 140px;
  color: var(--ds-faint);
  font-size: 13px;
}

@media (max-width: 1200px) {
  .session-list {
    max-height: 220px;
  }
}
</style>
