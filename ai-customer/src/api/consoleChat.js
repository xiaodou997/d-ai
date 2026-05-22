import { useAuthStore } from '@/stores/auth'
import request from '@/utils/request'

export const listConsoleChatModels = () => request.get('/console/v2/chat/models')

export const listConsoleChatSessions = () => request.get('/console/v2/chat/sessions')

export const createConsoleChatSession = (data) => request.post('/console/v2/chat/sessions', data)

export const getConsoleChatSession = (sessionId) => request.get(`/console/v2/chat/sessions/${sessionId}`)

export const deleteConsoleChatSession = (sessionId) => request.delete(`/console/v2/chat/sessions/${sessionId}`)

const parseErrorMessage = async (response) => {
  const text = await response.text().catch(() => '')
  if (!text) return `请求失败 (${response.status})`
  try {
    const data = JSON.parse(text)
    return data?.error?.message || data?.message || text
  } catch {
    return text
  }
}

const extractDelta = (payload, eventType) => {
  if (!payload || typeof payload !== 'object') return ''
  const choice = payload.choices?.[0]
  if (choice?.delta?.content) return choice.delta.content
  if (choice?.text) return choice.text
  if (typeof payload.delta === 'string') return payload.delta
  if (typeof payload.text === 'string' && eventType?.includes('delta')) return payload.text
  if (payload.delta?.text) return payload.delta.text
  const parts = payload.candidates?.[0]?.content?.parts
  if (Array.isArray(parts)) return parts.map((part) => part.text || '').join('')
  return ''
}

export const streamConsoleChatMessage = async ({
  sessionId,
  model,
  protocolPolicy,
  protocol,
  messages,
  temperature,
  maxTokens,
  signal,
  onDelta,
  onEvent
}) => {
  const authStore = useAuthStore()
  const response = await fetch(`/console/v2/chat/sessions/${sessionId}/messages:stream`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      Accept: 'text/event-stream',
      Authorization: `Bearer ${authStore.accessToken}`
    },
    body: JSON.stringify({
      model,
      protocol_policy: protocolPolicy || 'auto',
      protocol: protocol || '',
      messages,
      options: {
        temperature,
        max_tokens: maxTokens || undefined
      }
    }),
    signal
  })

  if (!response.ok) {
    throw new Error(await parseErrorMessage(response))
  }
  if (!response.body) {
    throw new Error('当前浏览器不支持流式响应')
  }

  const reader = response.body.getReader()
  const decoder = new TextDecoder()
  let buffer = ''
  let eventType = ''

  while (true) {
    const { value, done } = await reader.read()
    if (done) break
    buffer += decoder.decode(value, { stream: true })
    const events = buffer.split('\n\n')
    buffer = events.pop() || ''

    for (const event of events) {
      const lines = event.split('\n')
      for (const line of lines) {
        if (line.startsWith('event:')) {
          eventType = line.slice(6).trim()
          onEvent?.(eventType)
          continue
        }
        if (!line.startsWith('data:')) continue
        const data = line.slice(5).trim()
        if (!data || data === '[DONE]') continue
        const parsed = JSON.parse(data)
        const delta = extractDelta(parsed, eventType)
        if (delta) onDelta(delta)
      }
      eventType = ''
    }
  }
}
