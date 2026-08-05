// AI 网关共享常量。

export interface CapabilityOption {
  label: string;
  value: string;
}

export const DEFAULT_OPENAI_BINDING_PROTOCOL = "openai_responses";

export function defaultBindingProtocolForProviderFamily(providerFamily?: string) {
  switch (providerFamily) {
    case "anthropic":
      return "anthropic_messages";
    case "gemini":
      return "gemini_generate";
    default:
      return DEFAULT_OPENAI_BINDING_PROTOCOL;
  }
}

export const capabilityOptions: CapabilityOption[] = [
  { label: "文本对话", value: "chat" },
  { label: "生图", value: "image" },
  { label: "视频", value: "video" },
  { label: "Embedding", value: "embedding" },
  { label: "语音合成 TTS", value: "audio_tts" },
  { label: "语音识别 STT", value: "audio_stt" },
  { label: "重排", value: "rerank" }
];

export const statusOptions: CapabilityOption[] = [
  { label: "启用", value: "active" },
  { label: "停用", value: "disabled" }
];

// 上游部署协议（账号池/部署用）
export const protocolOptions: CapabilityOption[] = [
  { label: "OpenAI Chat", value: "openai_chat" },
  { label: "OpenAI Responses", value: "openai_responses" },
  { label: "OpenAI Embeddings", value: "openai_embeddings" },
  { label: "OpenAI Images", value: "openai_images" },
  { label: "Anthropic Messages", value: "anthropic_messages" },
  { label: "Gemini Generate", value: "gemini_generate" },
  { label: "Gemini Embeddings", value: "gemini_embeddings" }
];

// 上游端点协议（Provider endpoint 用）
export const endpointProtocolOptions: CapabilityOption[] = [
  { label: "OpenAI", value: "openai_compatible" },
  { label: "Anthropic", value: "anthropic" },
  { label: "Gemini", value: "gemini" }
];

const STATUS_LABEL_MAP: Record<string, string> = {
  active: "启用",
  disabled: "停用"
};

export function statusLabel(value: string): string {
  if (!value) return "-";
  return STATUS_LABEL_MAP[value] || value;
}

// ── 上游模型绑定：能力+API 格式合并选择器 ──────────────────────────────────────
//
// chat/image/embedding 这三个能力，后端 bindingProtocolSupportsCapability 有明确的
// 合法组合白名单（共 8 种），这里把该白名单物化成一份可渲染的分组清单，
// 让前端只能"选一项即合法组合"，不再需要用户自己拼出能力+协议的笛卡尔积。
// video/audio_tts/audio_stt/rerank 目前后端不限制协议（长尾能力，未设计专属协议），
// 不纳入这份清单，仍走 capabilityOptions/protocolOptions 的原始双选兜底。

export interface BindingFormatOption {
  value: string; // `${capability_type}:${api_format}`
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

export function findBindingFormatOption(capabilityType: string, apiFormat: string): BindingFormatOption | undefined {
  for (const group of bindingFormatGroups) {
    const found = group.options.find((o) => o.capability_type === capabilityType && o.api_format === apiFormat);
    if (found) return found;
  }
  return undefined;
}
