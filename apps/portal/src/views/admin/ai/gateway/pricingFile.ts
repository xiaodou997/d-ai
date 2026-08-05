import type { PriceBookEntryRecord, PriceBookRecord } from './pricingTypes'

export interface PriceBookFileV2 {
  schema_version: 2
  exported_at: string
  price_book: Pick<PriceBookRecord, 'name' | 'description' | 'status'>
  entries: PriceBookEntryRecord[]
}

export function createPriceBookFile(book: PriceBookRecord, entries: PriceBookEntryRecord[], exportedAt = new Date()): PriceBookFileV2 {
  return {
    schema_version: 2,
    exported_at: exportedAt.toISOString(),
    price_book: { name: book.name, description: book.description || '', status: book.status },
    entries: entries.map((entry) => ({
      ...entry,
      token_price_tiers: (entry.token_price_tiers || []).map((tier) => ({ ...tier })),
      image_prices: (entry.image_prices || []).map((price) => ({ ...price })),
      video_prices: (entry.video_prices || []).map((price) => ({ ...price }))
    }))
  }
}

export function parsePriceBookFile(value: unknown): { data?: PriceBookFileV2; error?: string } {
  if (!value || typeof value !== 'object') return { error: '文件格式不正确：仅支持 schema_version 2' }
  const candidate = value as Record<string, unknown>
  if (candidate.schema_version === 1) return { error: '不支持 v1 价格表，请使用 v2 文件' }
  if (candidate.schema_version !== 2 || !candidate.price_book || !Array.isArray(candidate.entries)) {
    return { error: '文件格式不正确：仅支持 schema_version 2' }
  }
  return { data: candidate as unknown as PriceBookFileV2 }
}
