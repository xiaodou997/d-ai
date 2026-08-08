export interface PortalVisibleGroupRecord {
  id: string;
  name: string;
  description?: string | null;
  effective_user_multiplier?: number | null;
  status?: string;
}

export interface PortalResolutionUSDPriceRecord {
  resolution: string;
  price: number;
}

export interface PortalGroupEffectivePriceRecord {
  model_code: string;
  capability_type: string;
  token_price_tiers: PortalEffectiveTokenPriceTierRecord[];
  image_default_price_usd: number;
  video_default_price_usd: number;
  image_prices?: PortalResolutionUSDPriceRecord[];
  video_prices?: PortalResolutionUSDPriceRecord[];
  audio_tts_per_1m_chars_usd: number;
  audio_stt_per_minute_usd: number;
}

export interface PortalEffectiveTokenPriceTierRecord {
  up_to_input_tokens: number | null;
  input_per_1m_usd: number;
  output_per_1m_usd: number;
  cache_write_per_1m_usd: number;
  cache_read_per_1m_usd: number;
}

export interface PortalVisibleGroupsApi<
  TGroup extends PortalVisibleGroupRecord = PortalVisibleGroupRecord
> {
  listGroups: () => Promise<{ items: TGroup[]; total?: number }>;
}

export interface PortalGroupPricingApi<
  TGroup extends PortalVisibleGroupRecord = PortalVisibleGroupRecord,
  TPrice extends PortalGroupEffectivePriceRecord = PortalGroupEffectivePriceRecord
> {
  listGroups: () => Promise<{ items: TGroup[]; total?: number }>;
  getGroupEffectivePrices: (groupId: string) => Promise<{
    group_id: string;
    effective_user_multiplier: number;
    items: TPrice[];
    total?: number;
  }>;
}
