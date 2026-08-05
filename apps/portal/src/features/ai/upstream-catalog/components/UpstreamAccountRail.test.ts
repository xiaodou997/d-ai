import { mount } from "@vue/test-utils";
import { describe, expect, it, vi } from "vitest";

import type { TenantAiUpstreamResource } from "@/api/types/aiTenant";
import UpstreamAccountRail from "./UpstreamAccountRail.vue";

vi.mock("@/platform", () => ({
  PortalContentCard: {
    props: ["title"],
    template: "<section><h2>{{ title }}</h2><slot /></section>"
  }
}));

function resource(id: string, name: string, apiFormat: string): TenantAiUpstreamResource {
  return {
    id,
    name,
    resource_kind: "direct_upstream",
    tenant_multiplier: 1,
    models: [{
      model_code: `${id}-model`,
      capability_type: "chat",
      api_format: apiFormat,
      availability: "available"
    }]
  };
}

describe("UpstreamAccountRail", () => {
  it("uses distinct semantic tag tones for provider families", () => {
    const wrapper = mount(UpstreamAccountRail, {
      props: {
        accounts: [
          resource("openai", "OpenAI 资源", "openai_responses"),
          resource("anthropic", "Anthropic 资源", "anthropic_messages"),
          resource("gemini", "Gemini 资源", "gemini_generate")
        ],
        selectedId: "openai",
        loading: false
      },
      global: {
        directives: { loading: () => {} }
      }
    });

    const tags = wrapper.findAll(".account-option__protocol");
    expect(tags.map((tag) => tag.text())).toEqual(["OpenAI", "Anthropic", "Gemini"]);
    expect(tags[0].classes()).toContain("ds-tag--positive");
    expect(tags[1].classes()).toContain("ds-tag--warning");
    expect(tags[2].classes()).toContain("ds-tag--info");
  });
});
