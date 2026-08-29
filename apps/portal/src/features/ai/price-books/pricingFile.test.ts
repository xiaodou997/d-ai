import { describe, expect, it } from 'vitest'

import { createPriceBookFile, parsePriceBookFile } from './pricingFile'
import type { PriceBookEntryRecord, PriceBookRecord } from './pricingTypes'

describe('price book file v2', () => {
  it('exports context tiers without legacy flat prices', () => {
    const book = { id: 'pb-1', name: 'Standard', description: '', status: 'active' } as PriceBookRecord
    const entry = {
      model_code: 'model-1', capability_type: 'chat', token_price_tiers: [{
        up_to_input_tokens: null,
        input_per_1m_usd: 1,
        output_per_1m_usd: 2,
        cache_write_per_1m_usd: 1,
        cache_read_per_1m_usd: 0.1
      }]
    } as PriceBookEntryRecord

    const file = createPriceBookFile(book, [entry], new Date('2026-07-12T00:00:00Z'))

    expect(file.schema_version).toBe(2)
    expect(file.entries[0]?.token_price_tiers).toEqual(entry.token_price_tiers)
    expect(file.entries[0]).not.toHaveProperty('reasoning_per_1m_usd')
  })

  it('rejects v1 files explicitly', () => {
    expect(parsePriceBookFile({ schema_version: 1, price_book: {}, entries: [] }).error).toContain('不支持 v1')
  })

  it('accepts a v2 file with context tiers', () => {
    const file = {
      schema_version: 2,
      exported_at: '2026-07-12T00:00:00.000Z',
      price_book: { name: 'Imported', description: '', status: 'active' },
      entries: [{
        model_code: 'model-1',
        capability_type: 'chat',
        token_price_tiers: [{
          up_to_input_tokens: 200_000,
          input_per_1m_usd: 1,
          output_per_1m_usd: 2,
          cache_write_per_1m_usd: 1,
          cache_read_per_1m_usd: 0
        }, {
          up_to_input_tokens: null,
          input_per_1m_usd: 2,
          output_per_1m_usd: 4,
          cache_write_per_1m_usd: 2,
          cache_read_per_1m_usd: 0
        }]
      }]
    }

    expect(parsePriceBookFile(file).data).toEqual(file)
  })
})
