// 主流模型参考表：成本价（USD/百万 token）+ 公开模型默认 token 配置。
// 历史数值保持不变，统一按 USD 解释。
//
// 字段说明：
//   - input/output:    上游成本价，单位 USD/M tokens
//   - context:         上下文窗口 tokens（公开模型默认值）
//   - defaultOutput:   默认最大输出 tokens
//   - maxOutput:       硬性最大输出 tokens

const VENDOR_CACHE_RULES: Record<string, { writeRatio: number; readRatio: number }> = {
  anthropic: { writeRatio: 1.25, readRatio: 0.1 },
  openai: { writeRatio: 0, readRatio: 0.5 },
  gemini: { writeRatio: 0, readRatio: 0.25 },
  deepseek: { writeRatio: 0, readRatio: 0.1 },
  qwen: { writeRatio: 0, readRatio: 0 },
  xai: { writeRatio: 0, readRatio: 0 },
  none: { writeRatio: 0, readRatio: 0 }
};

// matcher 必须使用正则；label 用于 banner 提示；vendor 决定缓存价倍率
export const MODEL_PRICING_CATALOG: any[] = [
  // ===== Anthropic Claude =====
  { matcher: /^claude.*haiku.*3[._-]?5/i, label: 'Claude Haiku 3.5', vendor: 'anthropic', input: 5.6, output: 28.0, context: 200000, defaultOutput: 4096, maxOutput: 8192 },
  { matcher: /^claude.*haiku.*4[._-]?5/i, label: 'Claude Haiku 4.5', vendor: 'anthropic', input: 7.0, output: 35.0, context: 200000, defaultOutput: 4096, maxOutput: 8192 },
  { matcher: /^claude.*sonnet.*3[._-]?5/i, label: 'Claude Sonnet 3.5', vendor: 'anthropic', input: 21.0, output: 105.0, context: 200000, defaultOutput: 4096, maxOutput: 8192 },
  { matcher: /^claude.*sonnet.*5/i, label: 'Claude Sonnet 5', vendor: 'anthropic', input: 14.0, output: 70.0, context: 1000000, defaultOutput: 8192, maxOutput: 128000 },
  { matcher: /^claude.*fable.*5/i, label: 'Claude Fable 5', vendor: 'anthropic', input: 70.0, output: 350.0, context: 1000000, defaultOutput: 8192, maxOutput: 128000 },
  { matcher: /^claude.*sonnet.*4[._-]?5/i, label: 'Claude Sonnet 4.5', vendor: 'anthropic', input: 21.0, output: 105.0, context: 200000, defaultOutput: 8192, maxOutput: 64000 },
  { matcher: /^claude.*sonnet.*4/i, label: 'Claude Sonnet 4', vendor: 'anthropic', input: 21.0, output: 105.0, context: 200000, defaultOutput: 8192, maxOutput: 64000 },
  { matcher: /^claude.*opus.*4[._-]?5/i, label: 'Claude Opus 4.5', vendor: 'anthropic', input: 35.0, output: 175.0, context: 200000, defaultOutput: 8192, maxOutput: 32000 },
  { matcher: /^claude.*opus.*4/i, label: 'Claude Opus 4', vendor: 'anthropic', input: 105.0, output: 525.0, context: 200000, defaultOutput: 8192, maxOutput: 32000 },
  { matcher: /^claude.*opus.*3/i, label: 'Claude Opus 3', vendor: 'anthropic', input: 105.0, output: 525.0, context: 200000, defaultOutput: 4096, maxOutput: 4096 },

  // ===== OpenAI =====
  { matcher: /^gpt[-_]?5[-_]?mini/i, label: 'GPT-5 mini', vendor: 'openai', input: 1.75, output: 14.0, context: 400000, defaultOutput: 16384, maxOutput: 128000 },
  { matcher: /^gpt[-_]?5[-_]?nano/i, label: 'GPT-5 nano', vendor: 'openai', input: 0.35, output: 2.8, context: 400000, defaultOutput: 8192, maxOutput: 128000 },
  { matcher: /^gpt[-_]?5/i, label: 'GPT-5', vendor: 'openai', input: 8.75, output: 70.0, context: 400000, defaultOutput: 16384, maxOutput: 128000 },
  { matcher: /^gpt[-_]?4\.1[-_]?mini/i, label: 'GPT-4.1 mini', vendor: 'openai', input: 2.8, output: 11.2, context: 1000000, defaultOutput: 8192, maxOutput: 32768 },
  { matcher: /^gpt[-_]?4\.1[-_]?nano/i, label: 'GPT-4.1 nano', vendor: 'openai', input: 0.7, output: 2.8, context: 1000000, defaultOutput: 4096, maxOutput: 32768 },
  { matcher: /^gpt[-_]?4\.1/i, label: 'GPT-4.1', vendor: 'openai', input: 14.0, output: 56.0, context: 1000000, defaultOutput: 8192, maxOutput: 32768 },
  { matcher: /^gpt[-_]?4o[-_]?mini/i, label: 'GPT-4o mini', vendor: 'openai', input: 1.05, output: 4.2, context: 128000, defaultOutput: 4096, maxOutput: 16384 },
  { matcher: /^gpt[-_]?4o/i, label: 'GPT-4o', vendor: 'openai', input: 17.5, output: 70.0, context: 128000, defaultOutput: 4096, maxOutput: 16384 },
  { matcher: /^o4[-_]?mini/i, label: 'o4-mini', vendor: 'openai', input: 7.7, output: 30.8, context: 200000, defaultOutput: 16384, maxOutput: 100000 },
  { matcher: /^o3[-_]?mini/i, label: 'o3-mini', vendor: 'openai', input: 7.7, output: 30.8, context: 200000, defaultOutput: 16384, maxOutput: 100000 },
  { matcher: /^o3/i, label: 'o3', vendor: 'openai', input: 14.0, output: 56.0, context: 200000, defaultOutput: 16384, maxOutput: 100000 },

  // ===== Google Gemini =====
  { matcher: /^gemini.*2\.5.*pro/i, label: 'Gemini 2.5 Pro', vendor: 'gemini', input: 8.75, output: 70.0, context: 2000000, defaultOutput: 8192, maxOutput: 65536 },
  { matcher: /^gemini.*2\.5.*flash.*lite/i, label: 'Gemini 2.5 Flash-Lite', vendor: 'gemini', input: 0.7, output: 2.8, context: 1000000, defaultOutput: 8192, maxOutput: 65536 },
  { matcher: /^gemini.*2\.5.*flash/i, label: 'Gemini 2.5 Flash', vendor: 'gemini', input: 2.1, output: 17.5, context: 1000000, defaultOutput: 8192, maxOutput: 65536 },
  { matcher: /^gemini.*2\.0.*flash.*lite/i, label: 'Gemini 2.0 Flash-Lite', vendor: 'gemini', input: 0.525, output: 2.1, context: 1000000, defaultOutput: 8192, maxOutput: 8192 },
  { matcher: /^gemini.*2\.0.*flash/i, label: 'Gemini 2.0 Flash', vendor: 'gemini', input: 0.7, output: 2.8, context: 1000000, defaultOutput: 8192, maxOutput: 8192 },

  // ===== Deepseek =====
  { matcher: /^deepseek.*reasoner|^deepseek.*r1/i, label: 'Deepseek R1', vendor: 'deepseek', input: 4.0, output: 16.0, context: 64000, defaultOutput: 8192, maxOutput: 64000 },
  { matcher: /^deepseek.*chat|^deepseek.*v3/i, label: 'Deepseek V3', vendor: 'deepseek', input: 2.0, output: 8.0, context: 64000, defaultOutput: 4096, maxOutput: 8192 },

  // ===== Qwen =====
  { matcher: /^qwen.*max/i, label: 'Qwen Max', vendor: 'qwen', input: 2.4, output: 9.6, context: 32000, defaultOutput: 8192, maxOutput: 8192 },
  { matcher: /^qwen.*plus/i, label: 'Qwen Plus', vendor: 'qwen', input: 0.8, output: 2.0, context: 131000, defaultOutput: 8192, maxOutput: 8192 },
  { matcher: /^qwen.*turbo/i, label: 'Qwen Turbo', vendor: 'qwen', input: 0.3, output: 0.6, context: 1000000, defaultOutput: 8192, maxOutput: 8192 },

  // ===== xAI Grok =====
  { matcher: /^grok[-_]?4/i, label: 'Grok 4', vendor: 'xai', input: 21.0, output: 105.0, context: 256000, defaultOutput: 8192, maxOutput: 131072 },
  { matcher: /^grok[-_]?3[-_]?mini/i, label: 'Grok 3 mini', vendor: 'xai', input: 2.1, output: 3.5, context: 131072, defaultOutput: 8192, maxOutput: 131072 },
  { matcher: /^grok[-_]?3/i, label: 'Grok 3', vendor: 'xai', input: 21.0, output: 105.0, context: 131072, defaultOutput: 8192, maxOutput: 131072 }
];

const round2 = (v: number) => Math.round(v * 10000) / 10000;

const matchCatalog = (modelCode: any): any => {
  if (!modelCode) return null;
  const name = String(modelCode).trim();
  return MODEL_PRICING_CATALOG.find((c) => c.matcher.test(name)) || null;
};

// 上游成本价建议：基于上游模型名 → { label, input/output/cache_* per_1m }
export function suggestPricingForModel(upstreamModel: any): any {
  const hit = matchCatalog(upstreamModel);
  if (!hit) return null;
  const rule = VENDOR_CACHE_RULES[hit.vendor] || VENDOR_CACHE_RULES.none;
  return {
    label: hit.label,
    vendor: hit.vendor,
    input_per_1m: round2(hit.input),
    output_per_1m: round2(hit.output),
    cache_write_per_1m: round2(hit.input * rule.writeRatio),
    cache_read_per_1m: round2(hit.input * rule.readRatio)
  };
}

// 公开模型默认配置建议：基于模型编码 → { label, context_window, default_max_output_tokens, max_output_tokens }
export function suggestModelConfig(modelCode: any): any {
  const hit = matchCatalog(modelCode);
  if (!hit) return null;
  return {
    label: hit.label,
    vendor: hit.vendor,
    context_window: hit.context ?? null,
    default_max_output_tokens: hit.defaultOutput ?? null,
    max_output_tokens: hit.maxOutput ?? null,
    suggested_input_per_1m: round2(hit.input),
    suggested_output_per_1m: round2(hit.output)
  };
}
