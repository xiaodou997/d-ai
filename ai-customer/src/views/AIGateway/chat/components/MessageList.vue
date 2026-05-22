<script setup>
import { nextTick, shallowRef } from 'vue'
import { ChatDotRound } from '@element-plus/icons-vue'

defineProps({
  messages: {
    type: Array,
    default: () => []
  }
})

const listRef = shallowRef(null)

const scrollToBottom = async () => {
  await nextTick()
  const el = listRef.value
  if (el) el.scrollTop = el.scrollHeight
}

defineExpose({ scrollToBottom })
</script>

<template>
  <div ref="listRef" class="message-list">
    <div v-if="messages.length === 0" class="empty-state">
      <el-icon :size="42"><ChatDotRound /></el-icon>
      <h2>开始一次自动协议对话</h2>
      <p>选择已授权模型后发送消息，后台会自动匹配最佳协议。</p>
    </div>

    <article v-for="message in messages" :key="message.id" class="message-row" :class="message.role">
      <div class="message-avatar">{{ message.role === 'user' ? '我' : 'AI' }}</div>
      <div class="message-bubble">
        <div v-if="message.role === 'assistant' && message.protocol" class="message-meta">{{ message.protocol }}</div>
        <p v-if="message.content">{{ message.content }}</p>
        <span v-else class="typing-dot">生成中...</span>
      </div>
    </article>
  </div>
</template>

<style scoped>
.message-list {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: 24px;
  background: #f8fafc;
}

.empty-state {
  height: 100%;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  color: #94a3b8;
  text-align: center;
}

.empty-state h2 {
  margin: 14px 0 4px;
  color: #334155;
  font-size: 18px;
  font-weight: 900;
}

.empty-state p {
  margin: 0;
  font-size: 13px;
}

.message-row {
  display: flex;
  gap: 12px;
  margin-bottom: 18px;
}

.message-row.user {
  flex-direction: row-reverse;
}

.message-avatar {
  width: 34px;
  height: 34px;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  color: #fff;
  font-size: 12px;
  font-weight: 900;
  border-radius: 10px;
  background: #0891b2;
}

.message-row.assistant .message-avatar {
  background: #0f766e;
}

.message-bubble {
  max-width: min(760px, 76%);
  padding: 12px 14px;
  color: #0f172a;
  background: #fff;
  border: 1px solid #e2e8f0;
  border-radius: 12px;
  box-shadow: 0 8px 24px rgba(15, 23, 42, 0.04);
}

.message-row.user .message-bubble {
  color: #fff;
  background: #0891b2;
  border-color: #0891b2;
}

.message-bubble p {
  margin: 0;
  white-space: pre-wrap;
  word-break: break-word;
  line-height: 1.7;
  font-size: 14px;
}

.message-meta {
  margin-bottom: 6px;
  color: #64748b;
  font-size: 11px;
  font-weight: 800;
}

.typing-dot {
  color: #64748b;
  font-size: 13px;
  font-weight: 700;
}
</style>
