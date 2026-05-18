// 主流模型上游成本价催化表（CNY/百万 token）
// USD → CNY 按 1:7 折算；CNY 原生模型直接使用厂商挂牌价
//
// 数据更新口径：参考厂商官网公开定价，截至 2026-05。
// 入参：input/output 为 CNY/M tokens 数值，缓存价由 vendor 规则自动推导。
// 匹配顺序：从上到下，命中即返回，"更具体"的型号必须排在通用型号之前
// （例如 gpt-5-mini 排在 gpt-5 之前）。

const VENDOR_CACHE_RULES = {
  anthropic: { writeRatio: 1.25, readRatio: 0.1 },
  openai:    { writeRatio: 0,    readRatio: 0.5 },
  gemini:    { writeRatio: 0,    readRatio: 0.25 },
  deepseek:  { writeRatio: 0,    readRatio: 0.1 },
  qwen:      { writeRatio: 0,    readRatio: 0 },
  xai:       { writeRatio: 0,    readRatio: 0 },
  none:      { writeRatio: 0,    readRatio: 0 }
}

// matcher 必须使用正则；label 用于 banner 提示；vendor 决定缓存价倍率
export const MODEL_PRICING_CATALOG = [
  // ===== Anthropic Claude =====
  { matcher: /^claude.*haiku.*3[._-]?5/i,  label: 'Claude Haiku 3.5',  vendor: 'anthropic', input: 5.60,  output: 28.00 },
  { matcher: /^claude.*haiku.*4[._-]?5/i,  label: 'Claude Haiku 4.5',  vendor: 'anthropic', input: 7.00,  output: 35.00 },
  { matcher: /^claude.*sonnet.*3[._-]?5/i, label: 'Claude Sonnet 3.5', vendor: 'anthropic', input: 21.00, output: 105.00 },
  { matcher: /^claude.*sonnet.*4[._-]?5/i, label: 'Claude Sonnet 4.5', vendor: 'anthropic', input: 21.00, output: 105.00 },
  { matcher: /^claude.*sonnet.*4/i,        label: 'Claude Sonnet 4',   vendor: 'anthropic', input: 21.00, output: 105.00 },
  { matcher: /^claude.*opus.*4[._-]?5/i,   label: 'Claude Opus 4.5',   vendor: 'anthropic', input: 35.00, output: 175.00 },
  { matcher: /^claude.*opus.*4/i,          label: 'Claude Opus 4',     vendor: 'anthropic', input: 105.00, output: 525.00 },
  { matcher: /^claude.*opus.*3/i,          label: 'Claude Opus 3',     vendor: 'anthropic', input: 105.00, output: 525.00 },

  // ===== OpenAI =====
  { matcher: /^gpt[-_]?5[-_]?mini/i,       label: 'GPT-5 mini',     vendor: 'openai', input: 1.75,  output: 14.00 },
  { matcher: /^gpt[-_]?5[-_]?nano/i,       label: 'GPT-5 nano',     vendor: 'openai', input: 0.35,  output: 2.80 },
  { matcher: /^gpt[-_]?5/i,                label: 'GPT-5',          vendor: 'openai', input: 8.75,  output: 70.00 },
  { matcher: /^gpt[-_]?4\.1[-_]?mini/i,    label: 'GPT-4.1 mini',   vendor: 'openai', input: 2.80,  output: 11.20 },
  { matcher: /^gpt[-_]?4\.1[-_]?nano/i,    label: 'GPT-4.1 nano',   vendor: 'openai', input: 0.70,  output: 2.80 },
  { matcher: /^gpt[-_]?4\.1/i,             label: 'GPT-4.1',        vendor: 'openai', input: 14.00, output: 56.00 },
  { matcher: /^gpt[-_]?4o[-_]?mini/i,      label: 'GPT-4o mini',    vendor: 'openai', input: 1.05,  output: 4.20 },
  { matcher: /^gpt[-_]?4o/i,               label: 'GPT-4o',         vendor: 'openai', input: 17.50, output: 70.00 },
  { matcher: /^o4[-_]?mini/i,              label: 'o4-mini',        vendor: 'openai', input: 7.70,  output: 30.80 },
  { matcher: /^o3[-_]?mini/i,              label: 'o3-mini',        vendor: 'openai', input: 7.70,  output: 30.80 },
  { matcher: /^o3/i,                       label: 'o3',             vendor: 'openai', input: 14.00, output: 56.00 },

  // ===== Google Gemini =====
  { matcher: /^gemini.*2\.5.*pro/i,         label: 'Gemini 2.5 Pro',         vendor: 'gemini', input: 8.75, output: 70.00 },
  { matcher: /^gemini.*2\.5.*flash.*lite/i, label: 'Gemini 2.5 Flash-Lite',  vendor: 'gemini', input: 0.70, output: 2.80 },
  { matcher: /^gemini.*2\.5.*flash/i,       label: 'Gemini 2.5 Flash',       vendor: 'gemini', input: 2.10, output: 17.50 },
  { matcher: /^gemini.*2\.0.*flash.*lite/i, label: 'Gemini 2.0 Flash-Lite',  vendor: 'gemini', input: 0.525, output: 2.10 },
  { matcher: /^gemini.*2\.0.*flash/i,       label: 'Gemini 2.0 Flash',       vendor: 'gemini', input: 0.70, output: 2.80 },

  // ===== Deepseek (CNY native) =====
  { matcher: /^deepseek.*reasoner|^deepseek.*r1/i, label: 'Deepseek R1',  vendor: 'deepseek', input: 4.00, output: 16.00 },
  { matcher: /^deepseek.*chat|^deepseek.*v3/i,     label: 'Deepseek V3',  vendor: 'deepseek', input: 2.00, output: 8.00 },

  // ===== Qwen (CNY native) =====
  { matcher: /^qwen.*max/i,    label: 'Qwen Max',    vendor: 'qwen', input: 2.40, output: 9.60 },
  { matcher: /^qwen.*plus/i,   label: 'Qwen Plus',   vendor: 'qwen', input: 0.80, output: 2.00 },
  { matcher: /^qwen.*turbo/i,  label: 'Qwen Turbo',  vendor: 'qwen', input: 0.30, output: 0.60 },

  // ===== xAI Grok =====
  { matcher: /^grok[-_]?4/i,         label: 'Grok 4',       vendor: 'xai', input: 21.00, output: 105.00 },
  { matcher: /^grok[-_]?3[-_]?mini/i, label: 'Grok 3 mini', vendor: 'xai', input: 2.10,  output: 3.50 },
  { matcher: /^grok[-_]?3/i,         label: 'Grok 3',       vendor: 'xai', input: 21.00, output: 105.00 }
]

const round2 = (v) => Math.round(v * 10000) / 10000

// 根据上游模型名找匹配项；返回 { label, input, output, cache_write_per_1m, cache_read_per_1m } 或 null
export function suggestPricingForModel(upstreamModel) {
  if (!upstreamModel) return null
  const name = String(upstreamModel).trim()
  const hit = MODEL_PRICING_CATALOG.find((c) => c.matcher.test(name))
  if (!hit) return null
  const rule = VENDOR_CACHE_RULES[hit.vendor] || VENDOR_CACHE_RULES.none
  return {
    label: hit.label,
    vendor: hit.vendor,
    input_per_1m: round2(hit.input),
    output_per_1m: round2(hit.output),
    cache_write_per_1m: round2(hit.input * rule.writeRatio),
    cache_read_per_1m: round2(hit.input * rule.readRatio),
    reasoning_per_1m: 0
  }
}
