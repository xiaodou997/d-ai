<script setup>
import { computed, nextTick, onMounted, shallowRef } from 'vue'
import { ChatDotRound, Delete, Plus, Promotion, Refresh, Setting } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import { listUserModelGrants } from '@/api/aiGateway'
import { streamConsoleChat } from '@/api/consoleChat'

const loadingModels = shallowRef(false)
const sending = shallowRef(false)
const models = shallowRef([])
const selectedModel = shallowRef('')
const input = shallowRef('')
const messages = shallowRef([])
const conversationId = shallowRef('')
const temperature = shallowRef(0.7)
const maxTokens = shallowRef(2048)
const showAdvanced = shallowRef(false)
const messageListRef = shallowRef(null)
let abortController = null

const chatModels = computed(() =>
  models.value
    .filter((model) => model.capability_type === 'chat' && model.status !== 'disabled')
    .map((model) => ({
      label: model.model_code,
      value: model.model_code
    }))
)

const requestMessages = computed(() =>
  messages.value
    .filter((message) => message.role === 'user' || message.role === 'assistant')
    .filter((message) => message.content.trim())
    .map((message) => ({ role: message.role, content: message.content }))
)

const canSend = computed(() =>
  Boolean(selectedModel.value && input.value.trim() && !sending.value)
)

const newConversationId = () => {
  if (crypto.randomUUID) return crypto.randomUUID()
  return `web-chat-${Date.now()}-${Math.random().toString(16).slice(2)}`
}

const newMessageId = () => {
  if (crypto.randomUUID) return crypto.randomUUID()
  return `message-${Date.now()}-${Math.random().toString(16).slice(2)}`
}

const scrollToBottom = async () => {
  await nextTick()
  const el = messageListRef.value
  if (el) el.scrollTop = el.scrollHeight
}

const fetchModels = async () => {
  loadingModels.value = true
  try {
    const res = await listUserModelGrants()
    models.value = Array.isArray(res) ? res : []
    if (!selectedModel.value && chatModels.value.length > 0) {
      selectedModel.value = chatModels.value[0].value
    }
  } finally {
    loadingModels.value = false
  }
}

const resetConversation = () => {
  abortController?.abort()
  abortController = null
  sending.value = false
  messages.value = []
  conversationId.value = newConversationId()
  input.value = ''
}

const appendMessage = (message) => {
  messages.value = [...messages.value, message]
}

const updateLastAssistant = (delta) => {
  const next = [...messages.value]
  const last = next[next.length - 1]
  if (!last || last.role !== 'assistant') return
  next[next.length - 1] = { ...last, content: last.content + delta }
  messages.value = next
}

const stopGeneration = () => {
  abortController?.abort()
}

const sendMessage = async () => {
  if (!canSend.value) return
  const content = input.value.trim()
  input.value = ''
  appendMessage({ id: newMessageId(), role: 'user', content })
  appendMessage({ id: newMessageId(), role: 'assistant', content: '' })
  await scrollToBottom()

  abortController = new AbortController()
  sending.value = true
  try {
    await streamConsoleChat({
      model: selectedModel.value,
      messages: requestMessages.value,
      conversationId: conversationId.value,
      temperature: temperature.value,
      maxTokens: maxTokens.value,
      signal: abortController.signal,
      onDelta: (delta) => {
        updateLastAssistant(delta)
        scrollToBottom()
      }
    })
  } catch (error) {
    if (error.name !== 'AbortError') {
      updateLastAssistant(`\n\n请求失败：${error.message}`)
      ElMessage.error(error.message || 'AI 对话请求失败')
    }
  } finally {
    sending.value = false
    abortController = null
    await scrollToBottom()
  }
}

const handleInputKeydown = (event) => {
  if (event.key === 'Enter' && !event.shiftKey) {
    event.preventDefault()
    sendMessage()
  }
}

onMounted(() => {
  conversationId.value = newConversationId()
  fetchModels()
})
</script>

<template>
  <div class="chat-page">
    <header class="chat-header">
      <div class="title-block">
        <p class="eyebrow">Web Chat</p>
        <h1>AI 对话</h1>
        <p>通过网页对话入口调用模型，消耗记为网页对话并计入个人用量。</p>
      </div>
      <div class="header-actions">
        <el-button :icon="Refresh" :loading="loadingModels" @click="fetchModels">刷新模型</el-button>
        <el-button :icon="Plus" type="primary" @click="resetConversation">新对话</el-button>
      </div>
    </header>

    <section class="chat-shell">
      <aside class="chat-sidebar">
        <label class="field-label">模型</label>
        <el-select
          v-model="selectedModel"
          class="w-full"
          filterable
          :loading="loadingModels"
          placeholder="选择模型"
        >
          <el-option
            v-for="model in chatModels"
            :key="model.value"
            :label="model.label"
            :value="model.value"
          />
        </el-select>

        <button class="advanced-toggle" type="button" @click="showAdvanced = !showAdvanced">
          <el-icon><Setting /></el-icon>
          <span>高级设置</span>
        </button>

        <div v-show="showAdvanced" class="advanced-panel">
          <label class="field-label">Temperature</label>
          <el-slider v-model="temperature" :min="0" :max="2" :step="0.1" />
          <label class="field-label">Max tokens</label>
          <el-input-number v-model="maxTokens" :min="256" :max="32768" :step="256" class="w-full" />
        </div>

        <div class="source-note">
          <el-icon><ChatDotRound /></el-icon>
          <span>当前调用来源：网页对话</span>
        </div>
      </aside>

      <main class="conversation">
        <div ref="messageListRef" class="message-list">
          <div v-if="messages.length === 0" class="empty-state">
            <el-icon :size="42"><ChatDotRound /></el-icon>
            <h2>开始一次文本对话</h2>
            <p>选择可用的文本模型后发送消息。</p>
          </div>

          <article
            v-for="message in messages"
            :key="message.id"
            class="message-row"
            :class="message.role"
          >
            <div class="message-avatar">{{ message.role === 'user' ? '我' : 'AI' }}</div>
            <div class="message-bubble">
              <p v-if="message.content">{{ message.content }}</p>
              <span v-else class="typing-dot">生成中...</span>
            </div>
          </article>
        </div>

        <footer class="composer">
          <el-input
            v-model="input"
            type="textarea"
            :rows="3"
            resize="none"
            placeholder="输入消息，Enter 发送，Shift + Enter 换行"
            @keydown="handleInputKeydown"
          />
          <div class="composer-actions">
            <el-button :icon="Delete" @click="resetConversation">清空</el-button>
            <el-button v-if="sending" type="warning" @click="stopGeneration">停止生成</el-button>
            <el-button v-else type="primary" :icon="Promotion" :disabled="!canSend" @click="sendMessage">
              发送
            </el-button>
          </div>
        </footer>
      </main>
    </section>
  </div>
</template>

<style scoped>
.chat-page {
  height: 100%;
  display: flex;
  flex-direction: column;
  gap: 20px;
  padding: 24px;
  color: #0f172a;
}

.chat-header {
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 22px 24px;
  background: #fff;
  border: 1px solid #e5e7eb;
  border-radius: 14px;
}

.title-block {
  min-width: 0;
}

.eyebrow {
  margin: 0 0 4px;
  color: #64748b;
  font-size: 11px;
  font-weight: 900;
  letter-spacing: 0.05em;
  text-transform: uppercase;
}

.title-block h1 {
  margin: 0;
  font-size: 22px;
  font-weight: 900;
}

.title-block p {
  margin: 5px 0 0;
  color: #64748b;
  font-size: 13px;
}

.header-actions {
  display: flex;
  gap: 10px;
  flex-shrink: 0;
}

.chat-shell {
  flex: 1;
  min-height: 0;
  display: grid;
  grid-template-columns: 280px minmax(0, 1fr);
  gap: 20px;
}

.chat-sidebar,
.conversation {
  min-height: 0;
  background: #fff;
  border: 1px solid #e5e7eb;
  border-radius: 14px;
}

.chat-sidebar {
  padding: 18px;
}

.field-label {
  display: block;
  margin: 0 0 8px;
  color: #475569;
  font-size: 12px;
  font-weight: 800;
}

.advanced-toggle {
  width: 100%;
  display: flex;
  align-items: center;
  gap: 8px;
  margin: 18px 0 10px;
  padding: 10px 0;
  color: #334155;
  font-size: 13px;
  font-weight: 800;
  background: transparent;
  border: 0;
  cursor: pointer;
}

.advanced-panel {
  padding: 12px;
  border: 1px solid #eef2f7;
  border-radius: 10px;
  background: #f8fafc;
}

.source-note {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 18px;
  padding: 10px 12px;
  color: #0369a1;
  font-size: 12px;
  font-weight: 700;
  border-radius: 10px;
  background: #e0f2fe;
}

.conversation {
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

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
  background: #06b6d4;
}

.message-row.assistant .message-avatar {
  background: #0f766e;
}

.message-bubble {
  max-width: min(720px, 76%);
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
  overflow-wrap: anywhere;
  line-height: 1.7;
  font-size: 14px;
}

.typing-dot {
  color: #64748b;
  font-size: 13px;
}

.composer {
  flex-shrink: 0;
  padding: 16px;
  border-top: 1px solid #e5e7eb;
  background: #fff;
}

.composer-actions {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
  margin-top: 10px;
}

@media (max-width: 900px) {
  .chat-shell {
    grid-template-columns: 1fr;
  }

  .chat-header {
    align-items: flex-start;
    flex-direction: column;
  }

  .header-actions {
    width: 100%;
  }
}
</style>
