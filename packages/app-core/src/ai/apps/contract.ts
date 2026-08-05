import type { PortalAppRecord, PortalAppRuntimeConfig, PortalAppType, PortalPromptStrategy } from "./types";

export type PortalAppCreativity = "precise" | "balanced" | "creative";
export type PortalImageResolution = "auto" | "1k" | "2k";

/** 对话应用「创造性」三档预设——存语义、不存 temperature 魔数。 */
export const PORTAL_CREATIVITY_OPTIONS: ReadonlyArray<{ value: PortalAppCreativity; label: string; hint: string }> = [
  { value: "precise", label: "严谨", hint: "更稳定、更可预期" },
  { value: "balanced", label: "平衡", hint: "默认，兼顾稳定与灵活" },
  { value: "creative", label: "发散", hint: "更有创意、更多样" }
];

/**
 * 平台统一的图片输出档位。服务端结合比例解析为 OpenAI Image 2 的 size。
 * 自动模式原样传递给上游，并按最高 4K 档计费。
 * 与后端 application.ImageResolutions 保持一致。
 */
export const PORTAL_IMAGE_RESOLUTIONS: ReadonlyArray<{ value: PortalImageResolution; label: string }> = [
  { value: "auto", label: "自动（按 4K 计费）" },
  { value: "1k", label: "1K" },
  { value: "2k", label: "2K" }
];

export const PORTAL_DEFAULT_RESOLUTION = "auto";
export const PORTAL_DEFAULT_IMAGE_ASPECT_RATIO = "1:1";
export const PORTAL_IMAGE_ASPECT_RATIOS = ["1:1", "2:3", "3:2", "3:4", "4:3", "16:9", "9:16"] as const;
export const PORTAL_DEFAULT_IMAGE_OUTPUT_COUNT = 1;
export const PORTAL_MAX_IMAGE_OUTPUT_COUNT = 10;

export function agentTypeLabel(value: PortalAppType | string) {
  switch (value) {
    case "image_generation":
      return "文生图";
    case "image_edit":
      return "图生图";
    default:
      return "对话";
  }
}

export function creativityLabel(value: PortalAppCreativity | string) {
  return PORTAL_CREATIVITY_OPTIONS.find((item) => item.value === value)?.label ?? "平衡";
}

export function resolutionLabel(value: string) {
  const match = PORTAL_IMAGE_RESOLUTIONS.find((item) => item.value === value);
  return match ? `${match.label}（${match.value}）` : value;
}

export function isImageAppType(agentType: PortalAppType) {
  return agentType === "image_generation" || agentType === "image_edit";
}

export function defaultRuntimeConfig(agentType: PortalAppType): PortalAppRuntimeConfig {
  if (agentType === "chat") {
    return { chat: { creativity: "balanced", allow_attachments: false } };
  }
  return {
    image: {
      resolution: PORTAL_DEFAULT_RESOLUTION,
      aspect_ratio: PORTAL_DEFAULT_IMAGE_ASPECT_RATIO,
      default_output_count: PORTAL_DEFAULT_IMAGE_OUTPUT_COUNT,
      max_output_count: PORTAL_DEFAULT_IMAGE_OUTPUT_COUNT,
      allow_output_count_override: false
    }
  };
}

export function normalizeRuntimeConfig(agentType: PortalAppType, value?: Record<string, unknown> | null): PortalAppRuntimeConfig {
  const fallback = defaultRuntimeConfig(agentType);
  const source = value && typeof value === "object" ? value : {};
  if (agentType === "chat") {
    const chat = objectValue(source.chat) || {};
    return {
      chat: {
        creativity: normalizeCreativity(chat.creativity, fallback.chat?.creativity ?? "balanced"),
        allow_attachments: booleanValue(chat.allow_attachments, fallback.chat?.allow_attachments ?? false)
      }
    };
  }
  const image = objectValue(source.image) || {};
  const defaultOutputCount = normalizeImageOutputCount(
    image.default_output_count,
    fallback.image?.default_output_count ?? PORTAL_DEFAULT_IMAGE_OUTPUT_COUNT
  );
  const maxOutputCount = Math.max(
    defaultOutputCount,
    normalizeImageOutputCount(image.max_output_count, defaultOutputCount)
  );

  const [resolution, aspectRatio] = normalizeImageRuntimeSettings(
    image.resolution,
    image.aspect_ratio,
    fallback.image?.resolution ?? PORTAL_DEFAULT_RESOLUTION,
    fallback.image?.aspect_ratio ?? PORTAL_DEFAULT_IMAGE_ASPECT_RATIO
  );
  return {
    image: {
      resolution,
      aspect_ratio: aspectRatio,
      default_output_count: defaultOutputCount,
      max_output_count: maxOutputCount,
      allow_output_count_override: booleanValue(image.allow_output_count_override, false)
    }
  };
}

export function appRuntimeConfig(app: Pick<PortalAppRecord, "capability" | "runtime_config">) {
	return normalizeRuntimeConfig(app.capability, app.runtime_config as Record<string, unknown> | null | undefined);
}

/** 生图应用是否允许附件——生图没有附件概念，仅对话应用有。 */
export function appAllowsAttachments(agentType: PortalAppType, config: PortalAppRuntimeConfig) {
  return agentType === "chat" && Boolean(config.chat?.allow_attachments);
}

export function appInputFields(agentType: PortalAppType, variables: string[], allowAttachments = false) {
  const variablesField = {
    name: "variables",
    required: variables.length ? "是" : "否",
    desc: variables.length ? `提示词变量：${variables.join(", ")}` : "当前提示词没有变量占位符，可省略"
  };
  if (agentType === "chat") {
    const fields = [
      { name: "input", required: "是", desc: "本次用户输入文本" },
      variablesField
    ];
    if (allowAttachments) {
      fields.push({
        name: "attachments",
        required: "否",
        desc: "对象数组（非字符串数组），每项形如 {type:\"image\"|\"file\", url, name?, mime_type?}；type=image 走多模态图片(image_url)，其余走文件(file)。缺省 type 时按 mime_type 或 URL 后缀识别图片。"
      });
    }
    fields.push({ name: "stream", required: "否", desc: "是否返回流式响应（SSE），默认 false" });
    return fields;
  }
	const fields = [
		{ name: "input", required: "是", desc: "本次图片生成或编辑要求；分辨率由应用固定" },
    variablesField
  ];
  fields.push({ name: "n", required: "否", desc: "输出图片张数；省略时使用应用默认值，是否可改及上限由应用配置决定" });
  if (agentType === "image_edit") {
    fields.push({ name: "images", required: "是", desc: "对象数组，每项为 { image_url }；image_url 支持 HTTP(S) URL 或 base64 data URL" });
    fields.push({ name: "mask", required: "否", desc: "可选对象 { image_url }，格式与 images 数组项相同" });
  }
  fields.push({ name: "response_format", required: "否", desc: "b64_json（默认）或 url；平台托管 URL 默认有效期为 24 小时，上游已有 URL 原样透传" });
  fields.push({ name: "stream", required: "否", desc: "是否返回流式响应（SSE），默认 false" });
  return fields;
}

export function appOutputFields(agentType: PortalAppType) {
  if (agentType === "chat") {
    return [
      { name: "type", desc: "固定为 chat" },
      { name: "text", desc: "模型输出文本" },
      { name: "usage", desc: "用量统计" },
      { name: "request_id", desc: "请求 ID" }
    ];
  }
  return [
    { name: "type", desc: "固定为 image" },
    { name: "images[]", desc: "输出图片数组，每项依 response_format 含 url 或 b64_json；平台 URL 另含 asset_ref、expires_at" },
    { name: "usage", desc: "用量统计" },
    { name: "request_id", desc: "请求 ID" }
  ];
}

// All app types now share a single unified entrypoint: the app key bound to
// the request determines the behaviour, so there is no per-capability path.
export function appRunPath(_agentType: PortalAppType) {
  return "/v1/run";
}

export function appCurlExample(
  agentType: PortalAppType,
  variables: string[],
	promptStrategy: PortalPromptStrategy,
  publicBaseUrl = "https://api.example.com",
  runKey = "rk_xxx",
	allowAttachments = false,
	promptNames: string[] = []
) {
	const variableObject = variables.length ? Object.fromEntries(variables.map((item) => [item, `${item}_value`])) : {};
	const dynamicInput = promptNames.length
		? `请结合 ${promptNames.slice(0, 3).map((name) => `{{${name}}}`).join("，")} 完成本次任务`
		: "请根据输入完成本次任务";
	const input = promptStrategy === "bound_prompt_exact" ? dynamicInput : "请根据输入完成本次任务";
  const header = `curl ${publicBaseUrl}${appRunPath(agentType)} \\
  -H "Content-Type: application/json" \\
  -H "Authorization: Bearer ${runKey}" \\
  -d `;
  if (agentType === "chat") {
		const body: Record<string, unknown> = { input, ...(variables.length ? { variables: variableObject } : {}) };
    if (allowAttachments) {
      body.attachments = [
        { type: "image", url: "https://example.com/photo.png", name: "photo.png", mime_type: "image/png" },
        { type: "file", url: "https://example.com/report.pdf", name: "report.pdf" }
      ];
    }
    return header + `'${JSON.stringify(body, null, 2)}'`;
  }
	if (agentType === "image_generation") {
		return header + `'${JSON.stringify({ input, ...(variables.length ? { variables: variableObject } : {}) }, null, 2)}'`;
  }
  return (
    header +
    `'${JSON.stringify(
		{ input, ...(variables.length ? { variables: variableObject } : {}), images: [{ image_url: "https://example.com/reference.png" }] },
      null,
      2
    )}'`
  );
}

function objectValue(value: unknown): Record<string, unknown> | null {
  return value && typeof value === "object" && !Array.isArray(value) ? (value as Record<string, unknown>) : null;
}

function normalizeCreativity(value: unknown, fallback: PortalAppCreativity): PortalAppCreativity {
  return value === "precise" || value === "balanced" || value === "creative" ? value : fallback;
}

function normalizeResolution(value: unknown, fallback: string): string {
  return typeof value === "string" && PORTAL_IMAGE_RESOLUTIONS.some((item) => item.value === value) ? value : fallback;
}

function normalizeImageRuntimeSettings(
  resolution: unknown,
  aspectRatio: unknown,
  fallbackResolution: string,
  fallbackAspectRatio: string
): [string, string] {
  const configuredResolution = normalizeResolution(resolution, "");
  if (configuredResolution) {
    return [configuredResolution, normalizeImageAspectRatio(aspectRatio, fallbackAspectRatio)];
  }

  const legacy = legacyImageRuntimeSettings(resolution);
  if (!legacy) {
    return [fallbackResolution, normalizeImageAspectRatio(aspectRatio, fallbackAspectRatio)];
  }
  return [legacy.resolution, normalizeImageAspectRatio(aspectRatio, legacy.aspectRatio)];
}

function legacyImageRuntimeSettings(value: unknown): { resolution: PortalImageResolution; aspectRatio: string } | null {
  if (typeof value !== "string") return null;
  const match = /^\s*(\d+)\s*x\s*(\d+)\s*$/i.exec(value);
  if (!match) return null;
  const width = Number(match[1]);
  const height = Number(match[2]);
  const pixels = width * height;
  const ratio = width / height;
  if (
    !Number.isInteger(width) ||
    !Number.isInteger(height) ||
    width % 16 !== 0 ||
    height % 16 !== 0 ||
    width > 3840 ||
    height > 3840 ||
    pixels < 655_360 ||
    pixels > 3840 * 2160 ||
    ratio < 1 / 3 ||
    ratio > 3
  ) {
    return null;
  }
  if (pixels > 2048 * 2048) return null;
  return {
    resolution: pixels <= 1024 * 1024 ? "1k" : "2k",
    aspectRatio: normalizeImageAspectRatio(`${width}:${height}`)
  };
}

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

function normalizeImageOutputCount(value: unknown, fallback: number) {
  return typeof value === "number" && Number.isInteger(value) && value >= 1 && value <= PORTAL_MAX_IMAGE_OUTPUT_COUNT
    ? value
    : fallback;
}

function booleanValue(value: unknown, fallback: boolean) {
  return typeof value === "boolean" ? value : fallback;
}
