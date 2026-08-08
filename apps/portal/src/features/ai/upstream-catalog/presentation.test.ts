import { describe, expect, it } from "vitest";

import type { TenantAiPriceBookEntry, TenantAiUpstreamResource } from "@/api/types/aiTenant";
import {
  buildPricingCards,
  formatMultiplier,
  protocolLabel,
  protocolTagTone,
  resourceProtocolLabels
} from "./presentation";

function tokenPrice(): TenantAiPriceBookEntry {
  return {
    model_code: "claude-test",
    capability_type: "chat",
    token_price_tiers: [
      {
        up_to_input_tokens: null,
        input_per_1m_usd: 10,
        output_per_1m_usd: 20,
        cache_write_per_1m_usd: 4,
        cache_read_per_1m_usd: 1
      }
    ],
    image_default_price_usd: 0,
    video_default_price_usd: 0,
    audio_tts_per_1m_chars_usd: 0,
    audio_stt_per_minute_usd: 0
  };
}

function upstream(price?: TenantAiPriceBookEntry): TenantAiUpstreamResource {
  return {
    id: "account-1",
    resource_kind: "direct_upstream",
    name: "Claude 长期稳定",
    tenant_multiplier: 0.8,
    models: [
      {
        model_code: "claude-test",
        capability_type: "chat",
        api_format: "anthropic",
        availability: price ? "available" : "no_price_configured",
        price
      }
    ]
  };
}

describe("upstream pricing presentation", () => {
  it("applies the upstream account multiplier to displayed prices", () => {
    const [card] = buildPricingCards(upstream(tokenPrice()));

    expect(card.sections[0].title).toBe("输入上下文 无上限");
    expect(card.sections[0].lines.map((line) => line.usd)).toEqual([
      "$8.00/1M",
      "$16.00/1M",
      "$3.20/1M",
      "$0.80/1M"
    ]);
  });

  it("hides models that cannot be called because they have no price", () => {
    expect(buildPricingCards(upstream())).toEqual([]);
  });

  it("formats resource metadata consistently", () => {
    expect(formatMultiplier(0.15749)).toBe("×0.1575");
    expect(protocolLabel("openai_chat")).toBe("OpenAI");
    expect(protocolLabel("gemini_images")).toBe("Gemini");
    expect(protocolTagTone("openai_chat")).toBe("positive");
    expect(protocolTagTone("Anthropic")).toBe("warning");
    expect(protocolTagTone("gemini_images")).toBe("info");
    expect(protocolTagTone("custom_protocol")).toBe("neutral");
    expect(resourceProtocolLabels(upstream())).toEqual(["Anthropic"]);
  });
});
