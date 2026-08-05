import type { PriceBookRecord } from './pricingTypes'

export function firstActivePriceBookId(priceBooks: readonly PriceBookRecord[]): string {
  return priceBooks.find((priceBook) => priceBook.status === 'active')?.id || ''
}
