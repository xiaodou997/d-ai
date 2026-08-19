export const MICRO_USD_PER_USD = 1_000_000;

const MICRO_USD_PER_CENT = 10_000;

function finiteNumber(value: number | string | null | undefined): number {
  const numeric = Number(value ?? 0);
  return Number.isFinite(numeric) ? numeric : 0;
}

function normalizeZero(value: number): number {
  return Object.is(value, -0) ? 0 : value;
}

export function truncateUSDForDisplay(value: number | string | null | undefined): number {
  const microUSD = Math.round(finiteNumber(value) * MICRO_USD_PER_USD);
  return normalizeZero(Math.trunc(microUSD / MICRO_USD_PER_CENT) / 100);
}

export function formatDisplayUSD(value: number | string | null | undefined): string {
  return `$${truncateUSDForDisplay(value).toLocaleString("en-US", {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2
  })}`;
}

export function formatDisplayMicroUSD(value: number | string | null | undefined): string {
  const cents = Math.trunc(finiteNumber(value) / MICRO_USD_PER_CENT);
  const usd = normalizeZero(cents / 100);
  return `$${usd.toLocaleString("en-US", {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2
  })}`;
}
