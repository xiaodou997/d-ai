import { describe, expect, it } from 'vitest'

import { firstActivePriceBookId } from './priceBookSelection'
import type { PriceBookRecord } from './pricingTypes'

function priceBook(id: string, status: PriceBookRecord['status']): PriceBookRecord {
  return { id, name: id, description: '', status }
}

describe('firstActivePriceBookId', () => {
  it('returns the first active price book in the supplied order', () => {
    const books = [
      priceBook('disabled-oldest', 'disabled'),
      priceBook('active-oldest', 'active'),
      priceBook('active-newer', 'active')
    ]

    expect(firstActivePriceBookId(books)).toBe('active-oldest')
  })

  it('returns an empty value when no active price book exists', () => {
    expect(firstActivePriceBookId([])).toBe('')
    expect(firstActivePriceBookId([priceBook('disabled', 'disabled')])).toBe('')
  })
})
