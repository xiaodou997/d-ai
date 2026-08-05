import type {
  TenantAiLiteLLMPriceModel,
  TenantAiPriceBookEntry,
  TenantAiTokenPriceTierUSD
} from "../../../types/aiTenant";

export type PriceBookEntryRecord = TenantAiPriceBookEntry;
export type TokenPriceTier = TenantAiTokenPriceTierUSD;
export type LiteLLMPriceModel = TenantAiLiteLLMPriceModel;

export interface PriceBookEntryForm {
  model_code: string;
  capability_type: string;
  token_price_tiers: TokenPriceTier[];
  audio_tts_per_1m_chars_usd: number;
  audio_stt_per_minute_usd: number;
  image_default_price_usd: number;
  video_default_price_usd: number;
  image_prices: Array<{ resolution: string; price: number }>;
  video_prices: Array<{ resolution: string; price: number }>;
}

const tokenPricedCapabilities = new Set(["chat", "embedding", "rerank"]);

export function isTokenPricedCapability(capability: string) {
  return tokenPricedCapabilities.has(capability);
}

export function validateTokenPriceTiers(tiers: TokenPriceTier[]) {
  if (!tiers.length) return "至少需要一个价格档位";
  if (tiers.at(-1)?.up_to_input_tokens !== null) return "最后一档必须无上限";
  let previous = 0;
  for (const [index, tier] of tiers.slice(0, -1).entries()) {
    const limit = tier.up_to_input_tokens;
    if (typeof limit !== "number" || !Number.isInteger(limit) || limit <= previous) {
      return `档位 ${index + 1} 的上限必须严格递增`;
    }
    previous = limit;
  }
  return "";
}
