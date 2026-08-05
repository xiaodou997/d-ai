// 用户门户小图标前端预处理：把用户上传的任意常见图片裁成正方形、缩放并转成
// PNG dataURL，压到后端限制（≤512KB、64~512 像素正方形）之内再上传。
// 后端只接受 PNG，故这里统一输出 PNG。

const MAX_FAVICON_BYTES = 512 * 1024;
// 逐级尝试的输出边长（像素），从清晰到最小，直到压缩后不超过大小限制。
// 最小 64 与后端下限一致。
const CANDIDATE_SIZES = [256, 192, 128, 96, 64] as const;

export const FAVICON_ACCEPT = "image/png, image/jpeg, image/webp";

/**
 * 将上传的图片文件归一化为正方形 PNG 的 dataURL。
 * - 非图片文件直接报错；
 * - 按短边居中裁剪为正方形，避免拉伸变形；
 * - 从大到小尝试输出尺寸，返回首个不超过 512KB 的结果。
 */
export async function normalizeFaviconToPngDataUrl(file: File): Promise<string> {
  if (!file.type.startsWith("image/")) {
    throw new Error("请选择图片文件（JPG / PNG / WebP）");
  }

  const bitmap = await loadBitmap(file);
  try {
    const side = Math.min(bitmap.width, bitmap.height);
    if (side <= 0) {
      throw new Error("无法读取图片尺寸，请更换图片");
    }
    const sx = (bitmap.width - side) / 2;
    const sy = (bitmap.height - side) / 2;

    for (const size of CANDIDATE_SIZES) {
      const dataUrl = drawSquarePng(bitmap, sx, sy, side, size);
      if (dataUrlByteLength(dataUrl) <= MAX_FAVICON_BYTES) {
        return dataUrl;
      }
    }
    throw new Error("图片压缩后仍超过 512KB，请换一张更简单的图片");
  } finally {
    bitmap.close();
  }
}

async function loadBitmap(file: File): Promise<ImageBitmap> {
  try {
    return await createImageBitmap(file);
  } catch {
    throw new Error("无法解析该图片，请更换 JPG / PNG / WebP 图片");
  }
}

function drawSquarePng(source: ImageBitmap, sx: number, sy: number, side: number, size: number): string {
  const canvas = document.createElement("canvas");
  canvas.width = size;
  canvas.height = size;
  const ctx = canvas.getContext("2d");
  if (!ctx) {
    throw new Error("当前浏览器不支持图片处理，请更换浏览器");
  }
  ctx.imageSmoothingEnabled = true;
  ctx.imageSmoothingQuality = "high";
  ctx.drawImage(source, sx, sy, side, side, 0, 0, size, size);
  return canvas.toDataURL("image/png");
}

function dataUrlByteLength(dataUrl: string): number {
  const commaIndex = dataUrl.indexOf(",");
  const base64 = commaIndex >= 0 ? dataUrl.slice(commaIndex + 1) : dataUrl;
  const padding = base64.endsWith("==") ? 2 : base64.endsWith("=") ? 1 : 0;
  return Math.floor((base64.length * 3) / 4) - padding;
}
