// 显式模型绑定的能力与上游协议选择器。

export interface CapabilityOption {
  label: string;
  value: string;
}

export const DEFAULT_OPENAI_BINDING_PROTOCOL = "openai_responses";

export const statusOptions: CapabilityOption[] = [
  { label: "启用", value: "active" },
  { label: "停用", value: "disabled" }
];

export const capabilityOptions: CapabilityOption[] = [
  { label: "文本对话", value: "chat" },
  { label: "生图", value: "image" },
  { label: "视频", value: "video" },
  { label: "Embedding", value: "embedding" },
  { label: "语音合成 TTS", value: "audio_tts" },
  { label: "语音识别 STT", value: "audio_stt" },
  { label: "重排", value: "rerank" }
];

export const protocolOptions: CapabilityOption[] = [
  { label: "OpenAI Chat", value: "openai_chat" },
  { label: "OpenAI Responses", value: "openai_responses" },
  { label: "OpenAI Embeddings", value: "openai_embeddings" },
  { label: "OpenAI Images", value: "openai_images" },
  { label: "Anthropic Messages", value: "anthropic_messages" },
  { label: "Gemini Generate", value: "gemini_generate" },
  { label: "Gemini Embeddings", value: "gemini_embeddings" }
];

export interface BindingFormatOption {
  value: string;
  capability_type: string;
  api_format: string;
  label: string;
}

export interface BindingFormatGroup {
  label: string;
  capability_type: string;
  options: BindingFormatOption[];
}

export const OTHER_CAPABILITIES_VALUE = "__other__";

export const OTHER_CAPABILITY_TYPES = ["video", "audio_tts", "audio_stt", "rerank"];

export const bindingFormatGroups: BindingFormatGroup[] = [
  {
    label: "文本对话",
    capability_type: "chat",
    options: [
      { value: "chat:openai_chat", capability_type: "chat", api_format: "openai_chat", label: "OpenAI Chat" },
      { value: "chat:openai_responses", capability_type: "chat", api_format: "openai_responses", label: "OpenAI Responses" },
      { value: "chat:anthropic_messages", capability_type: "chat", api_format: "anthropic_messages", label: "Anthropic Messages" },
      { value: "chat:gemini_generate", capability_type: "chat", api_format: "gemini_generate", label: "Gemini Generate" }
    ]
  },
  {
    label: "生图",
    capability_type: "image",
    options: [
      { value: "image:openai_images", capability_type: "image", api_format: "openai_images", label: "OpenAI Images" },
      { value: "image:gemini_generate", capability_type: "image", api_format: "gemini_generate", label: "Gemini Generate（原生生图）" }
    ]
  },
  {
    label: "Embedding",
    capability_type: "embedding",
    options: [
      { value: "embedding:openai_embeddings", capability_type: "embedding", api_format: "openai_embeddings", label: "OpenAI Embeddings" },
      { value: "embedding:gemini_embeddings", capability_type: "embedding", api_format: "gemini_embeddings", label: "Gemini Embeddings" }
    ]
  }
];

export function bindingFormatValue(capabilityType: string, apiFormat: string): string {
  return `${capabilityType}:${apiFormat}`;
}
