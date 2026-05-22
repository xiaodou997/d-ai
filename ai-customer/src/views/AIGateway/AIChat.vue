<script setup>
import { computed, onMounted, shallowRef, watch } from 'vue'
import { Plus, Refresh } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import ChatComposer from './chat/components/ChatComposer.vue'
import MessageList from './chat/components/MessageList.vue'
import ModelProtocolPanel from './chat/components/ModelProtocolPanel.vue'
import SessionList from './chat/components/SessionList.vue'
import {
  createConsoleChatSession,
  deleteConsoleChatSession,
  getConsoleChatSession,
  listConsoleChatModels,
  listConsoleChatSessions,
  streamConsoleChatMessage
} from '@/api/consoleChat'

const protocolLabels = {
  openai_chat: 'OpenAI Chat',
  openai_responses: 'OpenAI Responses',
  anthropic_messages: 'Claude Messages',
  gemini_generate: 'Gemini Native'
}

const loadingModels = shallowRef(false)
const loadingSessions = shallowRef(false)
const sending = shallowRef(false)
const models = shallowRef([])
const sessions = shallowRef([])
const selectedSessionId = shallowRef('')
const selectedModel = shallowRef('')
const protocolPolicy = shallowRef('auto')
const selectedProtocol = shallowRef('')
const input = shallowRef('')
const messages = shallowRef([])
const temperature = shallowRef(0.7)
const maxTokens = shallowRef(2048)
const showAdvanced = shallowRef(false)
const messageListRef = shallowRef(null)
let abortController = null

const selectedModelInfo = computed(() =>
  models.value.find((model) => model.model_code === selectedModel.value)
)

const activeProtocolLabel = computed(() => {
  const protocol = protocolPolicy.value === 'manual'
    ? selectedProtocol.value
    : selectedModelInfo.value?.default_protocol
  return protocolLabels[protocol] || protocol || '自动'
})

const requestMessages = computed(() =>
  messages.value
    .filter((message) => message.role === 'user' || message.role === 'assistant')
    .filter((message) => message.content.trim())
    .map((message) => ({ role: message.role, content: message.content }))
)

const canSend = computed(() =>
  Boolean(selectedModel.value && input.value.trim() && !sending.value)
)

const newMessageId = () => {
  if (crypto.randomUUID) return crypto.randomUUID()
  return `message-${Date.now()}-${Math.random().toString(16).slice(2)}`
}

const scrollToBottom = async () => {
  await messageListRef.value?.scrollToBottom()
}

const normalizeMessage = (message) => ({
  id: message.id || newMessageId(),
  role: message.role,
  content: message.content || '',
  protocol: message.protocol || ''
})

const fetchModels = async () => {
  loadingModels.value = true
  try {
    models.value = await listConsoleChatModels()
    if (!selectedModel.value && models.value.length > 0) {
      selectedModel.value = models.value[0].model_code
    }
  } finally {
    loadingModels.value = false
  }
}

const fetchSessions = async () => {
  loadingSessions.value = true
  try {
    sessions.value = await listConsoleChatSessions()
  } finally {
    loadingSessions.value = false
  }
}

const createSession = async () => {
  if (!selectedModel.value) {
    ElMessage.warning('请先选择模型')
    return null
  }
  const session = await createConsoleChatSession({
    model_code: selectedModel.value,
    title: '新对话'
  })
  sessions.value = [session, ...sessions.value.filter((item) => item.id !== session.id)]
  selectedSessionId.value = session.id
  messages.value = []
  return session
}

const loadSession = async (sessionId) => {
  if (!sessionId || sending.value) return
  const detail = await getConsoleChatSession(sessionId)
  selectedSessionId.value = detail.session.id
  selectedModel.value = detail.session.model_code || selectedModel.value
  if (detail.session.selected_protocol) {
    selectedProtocol.value = detail.session.selected_protocol
  }
  messages.value = (detail.messages || []).map(normalizeMessage)
  await scrollToBottom()
}

const newConversation = async () => {
  abortController?.abort()
  abortController = null
  sending.value = false
  input.value = ''
  await createSession()
}

const removeSession = async (session) => {
  try {
    await ElMessageBox.confirm(`确定删除「${session.title || '新对话'}」吗？`, '删除对话', {
      type: 'warning',
      confirmButtonText: '删除',
      cancelButtonText: '取消'
    })
  } catch {
    return
  }
  await deleteConsoleChatSession(session.id)
  sessions.value = sessions.value.filter((item) => item.id !== session.id)
  if (selectedSessionId.value === session.id) {
    selectedSessionId.value = ''
    messages.value = []
  }
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

const clearConversation = () => {
  messages.value = []
}

const sendMessage = async () => {
  if (!canSend.value) return
  let sessionId = selectedSessionId.value
  if (!sessionId) {
    const session = await createSession()
    if (!session) return
    sessionId = session.id
  }

  const content = input.value.trim()
  input.value = ''
  appendMessage({ id: newMessageId(), role: 'user', content })
  appendMessage({ id: newMessageId(), role: 'assistant', content: '', protocol: activeProtocolLabel.value })
  await scrollToBottom()

  abortController = new AbortController()
  sending.value = true
  try {
    await streamConsoleChatMessage({
      sessionId,
      model: selectedModel.value,
      protocolPolicy: protocolPolicy.value,
      protocol: selectedProtocol.value,
      messages: requestMessages.value,
      temperature: temperature.value,
      maxTokens: maxTokens.value,
      signal: abortController.signal,
      onDelta: (delta) => {
        updateLastAssistant(delta)
        scrollToBottom()
      }
    })
    await fetchSessions()
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

watch(selectedModelInfo, (model) => {
  if (!model) return
  if (!selectedProtocol.value || !model.available_protocols?.includes(selectedProtocol.value)) {
    selectedProtocol.value = model.default_protocol || model.available_protocols?.[0] || ''
  }
})

onMounted(async () => {
  await Promise.all([fetchModels(), fetchSessions()])
  if (sessions.value.length > 0) {
    await loadSession(sessions.value[0].id)
  }
})
</script>

<template>
  <div class="chat-page">
    <header class="chat-header">
      <div class="title-block">
        <p class="eyebrow">Console Chat v2</p>
        <h1>AI 对话</h1>
        <p>选择模型即可对话，后台会自动选择可用协议与健康路由，消耗计入个人用量。</p>
      </div>
      <div class="header-actions">
        <el-button :icon="Refresh" :loading="loadingModels" @click="fetchModels">刷新模型</el-button>
        <el-button :icon="Plus" type="primary" @click="newConversation">新对话</el-button>
      </div>
    </header>

    <section class="chat-shell">
      <aside class="control-rail">
        <ModelProtocolPanel
          v-model:selected-model="selectedModel"
          v-model:protocol-policy="protocolPolicy"
          v-model:selected-protocol="selectedProtocol"
          v-model:temperature="temperature"
          v-model:max-tokens="maxTokens"
          v-model:show-advanced="showAdvanced"
          :models="models"
          :loading-models="loadingModels"
          :selected-model-info="selectedModelInfo"
          :active-protocol-label="activeProtocolLabel"
          :protocol-labels="protocolLabels"
        />

        <SessionList
          :sessions="sessions"
          :loading="loadingSessions"
          :selected-session-id="selectedSessionId"
          @new-session="newConversation"
          @select-session="loadSession"
          @remove-session="removeSession"
        />
      </aside>

      <main class="conversation">
        <MessageList ref="messageListRef" :messages="messages" />
        <ChatComposer
          v-model="input"
          :sending="sending"
          :can-send="canSend"
          @send="sendMessage"
          @stop="stopGeneration"
          @clear="clearConversation"
        />
      </main>
    </section>
  </div>
</template>

<style scoped>
.chat-page {
  height: 100%;
  display: flex;
  flex-direction: column;
  gap: 18px;
  padding: 24px;
  color: #0f172a;
}

.chat-header,
.conversation {
  min-height: 0;
  background: #fff;
  border: 1px solid #e5e7eb;
  border-radius: 12px;
}

.chat-header {
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 20px 22px;
}

.title-block {
  min-width: 0;
}

.eyebrow {
  margin: 0 0 4px;
  color: #64748b;
  font-size: 11px;
  font-weight: 900;
  letter-spacing: 0;
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
  grid-template-columns: minmax(280px, 340px) minmax(0, 1fr);
  gap: 18px;
}

.control-rail {
  min-height: 0;
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.conversation {
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

@media (max-width: 1200px) {
  .chat-shell {
    grid-template-columns: 1fr;
  }
}
</style>
