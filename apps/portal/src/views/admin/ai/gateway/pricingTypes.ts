export interface ResolutionUSDPrice {
  resolution: string
  price: number
}

export interface TokenPriceTier {
  up_to_input_tokens: number | null
  input_per_1m_usd: number
  output_per_1m_usd: number
  cache_write_per_1m_usd: number
  cache_read_per_1m_usd: number
}

const tokenPricedCapabilities = new Set(['chat', 'embedding', 'rerank'])

export function isTokenPricedCapability(capability: string) {
  return tokenPricedCapabilities.has(capability)
}

export function validateTokenPriceTiers(tiers: TokenPriceTier[]) {
  if (!tiers.length) return '至少需要一个价格档位'
  if (tiers.at(-1)?.up_to_input_tokens !== null) return '最后一档必须无上限'
  let previous = 0
  for (const [index, tier] of tiers.slice(0, -1).entries()) {
    const limit = tier.up_to_input_tokens
    if (typeof limit !== 'number' || !Number.isInteger(limit) || limit <= previous) {
      return `档位 ${index + 1} 的上限必须严格递增`
    }
    previous = limit
  }
  return ''
}

export interface PriceBookRecord {
  id: string
  name: string
  description: string
  status: 'active' | 'disabled'
}

export interface PriceBookEntryRecord {
  model_code: string
  capability_type: string
  token_price_tiers: TokenPriceTier[]
  image_default_price_usd: number
  video_default_price_usd: number
  image_prices?: ResolutionUSDPrice[]
  video_prices?: ResolutionUSDPrice[]
  audio_tts_per_1m_chars_usd: number
  audio_stt_per_minute_usd: number
  source: string
  manually_edited: boolean
}

export interface PriceBookEntryForm {
  model_code: string
  capability_type: string
  token_price_tiers: TokenPriceTier[]
  audio_tts_per_1m_chars_usd: number
  audio_stt_per_minute_usd: number
  image_default_price_usd: number
  video_default_price_usd: number
  image_prices: ResolutionUSDPrice[]
  video_prices: ResolutionUSDPrice[]
}

export interface LiteLLMPriceModel {
  model_code: string
  capability_type: string
  token_price_tiers: TokenPriceTier[]
}
