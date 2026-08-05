import { formatMultiplier as formatMultiplierValue } from "@/platform/ai/utils";

export function formatMultiplier(multiplier: number): string {
  return `×${formatMultiplierValue(multiplier)}`;
}
