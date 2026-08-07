export type PortalImageResolution = "auto" | "1k" | "2k";

export const PORTAL_IMAGE_RESOLUTIONS: ReadonlyArray<{ value: PortalImageResolution; label: string }> = [
  { value: "auto", label: "自动（按 4K 计费）" },
  { value: "1k", label: "1K" },
  { value: "2k", label: "2K" }
];

export const PORTAL_DEFAULT_IMAGE_ASPECT_RATIO = "1:1";
export const PORTAL_IMAGE_ASPECT_RATIOS = ["1:1", "2:3", "3:2", "3:4", "4:3", "16:9", "9:16"] as const;

export function normalizeImageAspectRatio(value: unknown, fallback = PORTAL_DEFAULT_IMAGE_ASPECT_RATIO): string {
  if (typeof value !== "string") return fallback;
  const parts = value.trim().split(":");
  if (parts.length !== 2) return fallback;
  const width = Number(parts[0]);
  const height = Number(parts[1]);
  if (!Number.isInteger(width) || !Number.isInteger(height) || width < 1 || height < 1 || width > 10_000 || height > 10_000) {
    return fallback;
  }
  const ratio = width / height;
  if (ratio < 1 / 3 || ratio > 3) return fallback;
  const divisor = greatestCommonDivisor(width, height);
  return `${width / divisor}:${height / divisor}`;
}

function greatestCommonDivisor(a: number, b: number): number {
  while (b) [a, b] = [b, a % b];
  return a;
}
