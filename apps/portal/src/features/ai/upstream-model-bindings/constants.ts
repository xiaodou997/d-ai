// 显式模型绑定只维护模型能力；协议选项仅用于展示模型发现来源端点。

export interface CapabilityOption {
  label: string;
  value: string;
}

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
