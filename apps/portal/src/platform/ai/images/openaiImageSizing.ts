import { normalizeImageAspectRatio } from "../apps/contract";

const imageTierPixels: Record<string, number> = {
  "1k": 1024 * 1024,
  "2k": 2048 * 2048,
  "4k": 3840 * 2160
};
const imageGrid = 16;
const imageMaxEdge = 3840;

export function resolveOpenAIImageSize(resolution: string, aspectRatio: string): string {
  if (resolution === "auto") return "auto";
  const pixels = imageTierPixels[resolution] || imageTierPixels["1k"];
  const [widthRatio, heightRatio] = normalizeImageAspectRatio(aspectRatio).split(":").map(Number);
  const ratio = widthRatio / heightRatio;
  let width = floorToGrid(Math.sqrt(pixels * ratio));
  let height = floorToGrid(Math.sqrt(pixels / ratio));

  if (width > imageMaxEdge || height > imageMaxEdge) {
    const scale = Math.min(imageMaxEdge / width, imageMaxEdge / height);
    width = floorToGrid(width * scale);
    height = floorToGrid(height * scale);
  }
  while (width * height > pixels) {
    if (width >= height) width -= imageGrid;
    else height -= imageGrid;
  }
  return `${width}x${height}`;
}

function floorToGrid(value: number): number {
  return Math.floor(value / imageGrid) * imageGrid;
}
