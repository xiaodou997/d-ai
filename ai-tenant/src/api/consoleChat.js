import { useAuthStore } from '@/stores/auth'
import request from '@/utils/request'

// listConsoleModels returns the models usable in the web console for a given
// capability (default 'chat'): granted to the caller and backed by a route the
// console can actually reach. Pass 'image' etc. for future web features.
export const listConsoleModels = (capability = 'chat') =>
  request.get('/console/v1/models', { params: { capability } })

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

export const streamConsoleChat = async ({
  model,
  messages,
  conversationId,
  temperature,
  maxTokens,
  signal,
  onDelta
}) => {
  const authStore = useAuthStore()
  const response = await fetch('/console/v1/chat/completions', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      Accept: 'text/event-stream',
      Authorization: `Bearer ${authStore.accessToken}`,
      'X-Conversation-Id': conversationId
    },
    body: JSON.stringify({
      model,
      messages,
      stream: true,
      temperature,
      max_tokens: maxTokens || undefined
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

  while (true) {
    const { value, done } = await reader.read()
    if (done) break
    buffer += decoder.decode(value, { stream: true })
    const events = buffer.split('\n\n')
    buffer = events.pop() || ''

    for (const event of events) {
      const lines = event.split('\n').filter((line) => line.startsWith('data:'))
      for (const line of lines) {
        const data = line.slice(5).trim()
        if (!data || data === '[DONE]') continue
        const parsed = JSON.parse(data)
        const delta = parsed.choices?.[0]?.delta?.content || parsed.choices?.[0]?.text || ''
        if (delta) onDelta(delta)
      }
    }
  }
}
