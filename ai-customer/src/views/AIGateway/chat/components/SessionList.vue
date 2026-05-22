<script setup>
import { Delete, Plus } from '@element-plus/icons-vue'

defineProps({
  sessions: {
    type: Array,
    default: () => []
  },
  loading: {
    type: Boolean,
    default: false
  },
  selectedSessionId: {
    type: String,
    default: ''
  }
})

const emit = defineEmits(['new-session', 'select-session', 'remove-session'])
</script>

<template>
  <aside class="session-rail">
    <div class="rail-head">
      <span>历史对话</span>
      <el-button link type="primary" :icon="Plus" @click="emit('new-session')">新建</el-button>
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
        <span>{{ session.title || '新对话' }}</span>
        <small>{{ session.model_code || '未选择模型' }}</small>
        <el-button link type="danger" :icon="Delete" @click.stop="emit('remove-session', session)" />
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
  background: #fff;
  border: 1px solid #e5e7eb;
  border-radius: 12px;
}

.rail-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 12px;
  color: #334155;
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
  border: 1px solid #e5e7eb;
  border-radius: 8px;
  background: #f8fafc;
  cursor: pointer;
}

.session-item.active {
  border-color: #0891b2;
  background: #ecfeff;
}

.session-item span {
  overflow: hidden;
  color: #0f172a;
  font-size: 13px;
  font-weight: 800;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.session-item small {
  overflow: hidden;
  color: #64748b;
  font-size: 11px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.session-item .el-button {
  grid-row: 1 / 3;
  grid-column: 2;
}

.empty-session {
  display: grid;
  place-items: center;
  min-height: 140px;
  color: #94a3b8;
  font-size: 13px;
}

@media (max-width: 1200px) {
  .session-list {
    max-height: 220px;
  }
}
</style>
