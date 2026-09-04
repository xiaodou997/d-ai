import type { UpstreamAPIFormat, UpstreamEndpointAuthScheme } from '@/api/types/admin'

export const upstreamAPIFormatOptions: Array<{ label: string; value: UpstreamAPIFormat }> = [
  { label: 'OpenAI · Chat Completions', value: 'openai_chat' },
  { label: 'OpenAI · Responses', value: 'openai_responses' },
  { label: 'OpenAI · Embeddings', value: 'openai_embeddings' },
  { label: 'OpenAI · Images', value: 'openai_images' },
  { label: 'Anthropic · Messages', value: 'anthropic_messages' },
  { label: 'Gemini · Generate Content', value: 'gemini_generate' },
  { label: 'Gemini · Embeddings', value: 'gemini_embeddings' }
]

export const endpointAuthSchemeOptions: Array<{ label: string; value: UpstreamEndpointAuthScheme }> = [
  { label: '跟随 API 格式', value: 'format_default' },
  { label: 'Authorization: Bearer', value: 'bearer' },
  { label: 'Anthropic x-api-key', value: 'anthropic_api_key' },
  { label: 'Gemini x-goog-api-key', value: 'gemini_api_key' },
  { label: '自定义请求头', value: 'custom_header' }
]

export function upstreamAPIFormatLabel(value?: string) {
  return upstreamAPIFormatOptions.find((option) => option.value === value)?.label || value || '-'
}
