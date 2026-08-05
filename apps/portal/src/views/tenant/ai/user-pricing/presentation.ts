import { formatMultiplier as formatMultiplierValue } from "@dai/app-core/ai/utils";

export function formatMultiplier(multiplier: number): string {
  return `×${formatMultiplierValue(multiplier)}`;
}
