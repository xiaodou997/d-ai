import { formatMultiplier as formatMultiplierValue } from "@/platform/ai/utils";

import type {
  TenantAiPriceBookEntry,
  TenantAiTokenPriceTierUSD,
  TenantAiUpstreamResource
} from "@/api/types/aiTenant";

export type PriceTone = "input" | "output" | "cache" | "default" | "resolution" | "audio";
export type CapabilityTheme = "token" | "image" | "video" | "audio";
export type ProtocolTagTone = "neutral" | "positive" | "warning" | "info";

export interface PricingLine {
  label: string;
  usd: string;
  tone: PriceTone;
}

export interface PricingSection {
  key: string;
  title: string;
  lines: PricingLine[];
}

export interface UpstreamPricingCard {
  key: string;
  modelCode: string;
  capabilityLabel: string;
  theme: CapabilityTheme;
  sections: PricingSection[];
}

const capabilityLabels: Record<string, string> = {
  chat: "对话",
  embedding: "向量",
  rerank: "重排序",
  image: "图像",
  video: "视频",
  audio_tts: "语音合成",
  audio_stt: "语音识别"
};

export function formatMultiplier(value: number) {
  return `×${formatMultiplierValue(value)}`;
}

const protocolLabels: Record<string, string> = {
  openai_chat: "OpenAI Chat",
  openai_responses: "OpenAI Responses",
  openai_embeddings: "OpenAI Embeddings",
  openai_images: "OpenAI Images",
  openai_compatible: "OpenAI",
  anthropic: "Anthropic",
  anthropic_messages: "Anthropic Messages",
  gemini: "Gemini",
  google: "Gemini",
  gemini_text: "Gemini",
  gemini_generate: "Gemini Generate",
  gemini_embeddings: "Gemini Embeddings",
  gemini_images: "Gemini"
};

export function protocolLabel(format: string) {
  return protocolLabels[format] || format || "未知协议";
}

export function protocolTagTone(value: string): ProtocolTagTone {
  const label = protocolLabel(value);
  if (label.startsWith("OpenAI")) return "positive";
  if (label.startsWith("Anthropic")) return "warning";
  if (label.startsWith("Gemini")) return "info";
  return "neutral";
}

export function resourceProtocolLabels(resource: TenantAiUpstreamResource) {
  return [...new Set((resource.api_formats || []).map(protocolLabel))];
}

export function buildPricingCards(resource: TenantAiUpstreamResource | null): UpstreamPricingCard[] {
  if (!resource) return [];
  const multiplier = normalizedMultiplier(resource.tenant_multiplier);

  return resource.models.flatMap((model) => {
    if (model.availability !== "available" || !model.price) return [];
    return [{
      key: `${model.model_code}-${model.capability_type}`,
      modelCode: model.model_code,
      capabilityLabel: capabilityLabels[model.capability_type] ?? model.capability_type,
      theme: capabilityTheme(model.capability_type),
      sections: priceSections(model.price, multiplier)
    }];
  });
}

function normalizedMultiplier(value: number) {
  const multiplier = Number(value);
  return Number.isFinite(multiplier) && multiplier >= 0 ? multiplier : 0;
}

function capabilityTheme(capability: string): CapabilityTheme {
  if (capability === "image") return "image";
  if (capability === "video") return "video";
  if (capability === "audio_tts" || capability === "audio_stt") return "audio";
  return "token";
}

function priceSections(price: TenantAiPriceBookEntry, multiplier: number): PricingSection[] {
  if (["chat", "embedding", "rerank"].includes(price.capability_type)) {
    return tokenSections(price.token_price_tiers ?? [], multiplier);
  }

  if (price.capability_type === "image") {
    return mediaSections(price.image_default_price_usd, price.image_prices ?? [], multiplier, "/张");
  }

  if (price.capability_type === "video") {
    return mediaSections(price.video_default_price_usd, price.video_prices ?? [], multiplier, "/秒");
  }

  if (price.capability_type === "audio_tts") {
    return [singlePriceSection("语音合成", "合成", price.audio_tts_per_1m_chars_usd, multiplier, "/1M字符")];
  }

  return [singlePriceSection("语音识别", "识别", price.audio_stt_per_minute_usd, multiplier, "/分钟")];
}

function tokenSections(tiers: TenantAiTokenPriceTierUSD[], multiplier: number): PricingSection[] {
  return tiers.map((tier, index) => ({
    key: `token-${index}`,
    title: contextTierLabel(tier, tiers.length),
    lines: [
      priceLine("输入", tier.input_per_1m_usd, multiplier, "/1M", "input"),
      priceLine("输出", tier.output_per_1m_usd, multiplier, "/1M", "output"),
      priceLine("缓存写入", tier.cache_write_per_1m_usd, multiplier, "/1M", "cache"),
      priceLine("缓存读取", tier.cache_read_per_1m_usd, multiplier, "/1M", "cache")
    ]
  }));
}

function contextTierLabel(tier: TenantAiTokenPriceTierUSD, tierCount: number) {
  if (tier.up_to_input_tokens == null) return tierCount === 1 ? "输入上下文 无上限" : "更长输入上下文";
  return `输入上下文 ≤ ${compactNumber(tier.up_to_input_tokens)}`;
}

function mediaSections(
  defaultPrice: number,
  overrides: Array<{ resolution: string; price: number }>,
  multiplier: number,
  unit: string
): PricingSection[] {
  const sections: PricingSection[] = [singlePriceSection("默认价格", "默认", defaultPrice, multiplier, unit)];
  if (overrides.length) {
    sections.push({
      key: "overrides",
      title: "规格价格",
      lines: overrides.map((item) => priceLine(item.resolution, item.price, multiplier, unit, "resolution"))
    });
  }
  return sections;
}

function singlePriceSection(
  title: string,
  label: string,
  price: number,
  multiplier: number,
  unit: string
): PricingSection {
  return {
    key: "default",
    title,
    lines: [priceLine(label, price, multiplier, unit, title.startsWith("语音") ? "audio" : "default")]
  };
}

function priceLine(
  label: string,
  price: number,
  multiplier: number,
  unit: string,
  tone: PriceTone
): PricingLine {
  const effectiveUSD = Number(price) * multiplier;
  return {
    label,
    usd: `${formatUSD(effectiveUSD)}${unit}`,
    tone
  };
}

function formatUSD(value: number) {
  const amount = Number.isFinite(value) ? value : 0;
  return `$${formatNumber(amount, 2)}`;
}

function formatNumber(value: number, minimumFractionDigits = 0) {
  const amount = Number.isFinite(value) ? value : 0;
  return amount.toLocaleString("en-US", { minimumFractionDigits, maximumFractionDigits: 6 });
}

function compactNumber(value: number) {
  if (value >= 1_000_000) return `${value / 1_000_000}M`;
  if (value >= 1_000) return `${value / 1_000}K`;
  return value.toLocaleString("en-US");
}
