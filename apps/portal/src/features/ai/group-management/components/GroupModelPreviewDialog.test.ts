import { flushPromises, mount } from "@vue/test-utils";
import { describe, expect, it, vi } from "vitest";

import type {
  TenantAiGroupEffectivePrice,
  TenantAiGroupEffectivePricesOutputBody,
  TenantAiVisibleGroup
} from "../../../../types/aiTenant";
import GroupModelPreviewDialog from "./GroupModelPreviewDialog.vue";

const ElDialogStub = {
  props: ["modelValue", "title"],
  template: "<section v-if='modelValue' data-test='dialog'><h2>{{ title }}</h2><slot /><footer><slot name='footer' /></footer></section>"
};

const ElButtonStub = {
  emits: ["click"],
  template: "<button type='button' @click='$emit(\"click\")'><slot /></button>"
};

function group(id: string, name: string): TenantAiVisibleGroup {
  return { id, name } as TenantAiVisibleGroup;
}

function model(modelCode: string, capabilityType = "chat"): TenantAiGroupEffectivePrice {
  return {
    model_code: modelCode,
    capability_type: capabilityType,
    token_price_tiers: [],
    image_default_price_credits: 0,
    video_default_price_credits: 0,
    audio_tts_per_1m_chars_credits: 0,
    audio_stt_per_minute_credits: 0
  };
}

function response(items: TenantAiGroupEffectivePrice[]): TenantAiGroupEffectivePricesOutputBody {
  return {
    group_id: "group-1",
    retail_price_book_id: "book-1",
    effective_user_multiplier: 1,
    credits_per_usd: 100,
    items,
    total: items.length
  };
}

function mountDialog(loadModels: (groupId: string) => Promise<TenantAiGroupEffectivePricesOutputBody>) {
  return mount(GroupModelPreviewDialog, {
    props: {
      modelValue: true,
      group: group("group-1", "稳定分组"),
      loadModels
    },
    global: {
      stubs: {
        ElButton: ElButtonStub,
        ElDialog: ElDialogStub
      }
    }
  });
}

describe("GroupModelPreviewDialog", () => {
  it("loads and labels the models available to the selected group", async () => {
    const loadModels = vi.fn(async () => response([
      model("gpt-5.4-mini"),
      model("gemini-image", "image")
    ]));
    const wrapper = mountDialog(loadModels);
    await flushPromises();

    expect(loadModels).toHaveBeenCalledWith("group-1");
    expect(wrapper.text()).toContain("可用模型预览 · 稳定分组");
    expect(wrapper.text()).toContain("2 个模型");
    expect(wrapper.text()).toContain("gpt-5.4-mini");
    expect(wrapper.text()).toContain("对话");
    expect(wrapper.text()).toContain("gemini-image");
    expect(wrapper.text()).toContain("图片");
  });

  it("does not let a previous group response replace the current preview", async () => {
    let resolveFirst!: (value: TenantAiGroupEffectivePricesOutputBody) => void;
    const loadModels = vi.fn()
      .mockImplementationOnce(() => new Promise<TenantAiGroupEffectivePricesOutputBody>((resolve) => {
        resolveFirst = resolve;
      }))
      .mockResolvedValueOnce(response([model("new-model")]));
    const wrapper = mountDialog(loadModels);

    await wrapper.setProps({ group: group("group-2", "新分组") });
    await flushPromises();
    resolveFirst(response([model("stale-model")]));
    await flushPromises();

    expect(loadModels).toHaveBeenNthCalledWith(2, "group-2");
    expect(wrapper.text()).toContain("new-model");
    expect(wrapper.text()).not.toContain("stale-model");
  });

  it("offers a retry after loading fails", async () => {
    const loadModels = vi.fn()
      .mockRejectedValueOnce(new Error("网络暂时不可用"))
      .mockResolvedValueOnce(response([model("recovered-model")]));
    const wrapper = mountDialog(loadModels);
    await flushPromises();

    expect(wrapper.text()).toContain("网络暂时不可用");
    await wrapper.get("button").trigger("click");
    await flushPromises();

    expect(loadModels).toHaveBeenCalledTimes(2);
    expect(wrapper.text()).toContain("recovered-model");
  });
});
