const MULTIPLIER_SCALE = 10_000n;

export interface SubscriptionPricingGroup {
  groupId: string;
  paygMultiplier: number;
  quotaDebitMultiplier: number;
}

interface Ratio {
  numerator: bigint;
  denominator: bigint;
  groupId: string;
}

function scaledDecimal(value: number): bigint {
  if (!Number.isFinite(value) || value < 0) return -1n;
  return BigInt(Math.round(value * Number(MULTIPLIER_SCALE)));
}

function worstValueRatio(groups: SubscriptionPricingGroup[]): Ratio | null {
  let worstValue: Ratio | null = null;
  for (const group of groups) {
    const payg = scaledDecimal(group.paygMultiplier);
    const debit = scaledDecimal(group.quotaDebitMultiplier);
    if (payg <= 0n || debit <= 0n) return null;

    const valueRatio = { numerator: payg, denominator: debit, groupId: group.groupId };
    if (!worstValue || valueRatio.numerator * worstValue.denominator < worstValue.numerator * valueRatio.denominator) {
      worstValue = valueRatio;
    }
  }
  return worstValue;
}

function toSafeNumber(value: bigint): number {
  return Number(value > BigInt(Number.MAX_SAFE_INTEGER) ? BigInt(Number.MAX_SAFE_INTEGER) : value);
}

function scaledFloor(value: number, ratio: Ratio): number {
  return toSafeNumber(BigInt(Math.max(0, Math.round(value))) * ratio.numerator / ratio.denominator);
}

export function estimateSubscriptionPaygValue(
  totalLimitMicro: number,
  groups: SubscriptionPricingGroup[]
): number | null {
  const ratio = worstValueRatio(groups);
  if (!ratio || totalLimitMicro <= 0) return null;
  return scaledFloor(totalLimitMicro, ratio);
}
