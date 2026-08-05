import type { TenantAiClientSurface } from "../../../types/aiTenant";

export interface ClientSurfaceOption {
  id: TenantAiClientSurface;
  name: string;
  endpoint: string;
  capability: "chat" | "embedding" | "image";
}

export const clientSurfaceOptions: ClientSurfaceOption[] = [
  { id: "openai_chat", name: "OpenAI Chat", endpoint: "/v1/chat/completions", capability: "chat" },
  { id: "openai_responses", name: "OpenAI Responses", endpoint: "/v1/responses", capability: "chat" },
  { id: "anthropic_messages", name: "Anthropic Messages", endpoint: "/v1/messages", capability: "chat" },
  { id: "gemini_text", name: "Gemini Text", endpoint: "generateContent", capability: "chat" },
  { id: "openai_embeddings", name: "OpenAI Embeddings", endpoint: "/v1/embeddings", capability: "embedding" },
  { id: "gemini_embeddings", name: "Gemini Embeddings", endpoint: "embedContent", capability: "embedding" },
  { id: "openai_images", name: "OpenAI Images", endpoint: "/v1/images/*", capability: "image" },
  { id: "gemini_images", name: "Gemini Images", endpoint: "image generateContent", capability: "image" }
];

export const allClientSurfaces = clientSurfaceOptions.map((item) => item.id);

export const clientSurfacePresets = {
  conversation: clientSurfaceOptions.filter((item) => item.capability === "chat").map((item) => item.id),
  image: clientSurfaceOptions.filter((item) => item.capability === "image").map((item) => item.id),
  embedding: clientSurfaceOptions.filter((item) => item.capability === "embedding").map((item) => item.id)
} satisfies Record<string, TenantAiClientSurface[]>;

export function surfaceOption(value: string) {
  return clientSurfaceOptions.find((item) => item.id === value);
}

export function surfaceLabel(value: string) {
  const item = surfaceOption(value);
  return item ? `${item.name} · ${item.endpoint}` : value || "-";
}

export function capabilityForSurface(value: string) {
  return surfaceOption(value)?.capability || "chat";
}

export const capabilityLabels: Record<string, string> = {
  chat: "对话",
  embedding: "向量",
  image: "图片",
  video: "视频",
  audio_tts: "语音合成",
  audio_stt: "语音识别",
  rerank: "重排序"
};
