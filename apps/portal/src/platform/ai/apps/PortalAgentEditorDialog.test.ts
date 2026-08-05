import { flushPromises, shallowMount } from "@vue/test-utils";
import { defineComponent } from "vue";
import { describe, expect, it } from "vitest";

import PortalAgentEditorDialog from "./PortalAgentEditorDialog.vue";

import type { PortalAppModelRecord, PortalAppRecord } from "./types";

const SlotStub = defineComponent({
  template: "<div><slot /></div>"
});

const ElDialogStub = defineComponent({
  props: { modelValue: Boolean },
  template: '<section v-if="modelValue"><slot /><slot name="footer" /></section>'
});

const ElInputNumberStub = defineComponent({
  name: "ElInputNumber",
  props: {
    modelValue: Number,
    min: Number,
    max: Number,
    disabled: Boolean
  },
  emits: ["update:modelValue"],
  template: '<input type="number" :value="modelValue" :disabled="disabled" />'
});

const ElButtonStub = defineComponent({
  name: "ElButton",
  props: { disabled: Boolean, loading: Boolean },
  emits: ["click"],
  template: '<button :disabled="disabled || loading" @click="$emit(\'click\')"><slot /></button>'
});

const global = {
  config: { warnHandler: () => undefined },
  stubs: {
    ElDialog: ElDialogStub,
    ElForm: SlotStub,
    ElFormItem: SlotStub,
    ElInput: true,
    ElRadioGroup: SlotStub,
    ElRadio: SlotStub,
    ElRadioButton: SlotStub,
		ElSegmented: SlotStub,
    ElTag: SlotStub,
    ElSelect: SlotStub,
    ElOption: true,
    ElSwitch: true,
    ElInputNumber: ElInputNumberStub,
    ElButton: ElButtonStub
  }
};

function imageApp(maxOutputCount = 5): PortalAppRecord {
  return {
    owner_type: "tenant",
    id: "app-1",
    name: "商品生图",
    description: "",
    status: "active",
		capability: "image_generation",
		prompt_strategy: "none",
		prompt_bindings: [],
    group_id: "group-1",
    model_code: "image-model",
    runtime_config: {
      image: {
        resolution: "1k",
        aspect_ratio: "1:1",
        default_output_count: 1,
        max_output_count: maxOutputCount,
        allow_output_count_override: true
      }
    }
  };
}

function imageModel(maxOutputCount: number): PortalAppModelRecord {
  return {
    group_id: "group-1",
    group_name: "默认分组",
    model_code: "image-model",
    capability_type: "image",
    status: "available",
    max_output_count: maxOutputCount,
    edit_max_output_count: maxOutputCount
  };
}

function mountEditor(options: { app?: PortalAppRecord; models?: PortalAppModelRecord[]; loadingModels?: boolean } = {}) {
  return shallowMount(PortalAgentEditorDialog, {
    props: {
      visible: true,
      loading: false,
      scope: "tenant",
		template: {
			id: "text_to_image",
			name: "文生图应用",
			description: "",
			defaultCapability: "image_generation",
			allowedCapabilities: ["image_generation"],
			promptStrategy: "none",
			minPromptBindings: 0,
			maxPromptBindings: 0
		},
      app: options.app ?? imageApp(),
      prompts: [],
      models: options.models ?? [imageModel(1)],
      loadingModels: options.loadingModels ?? false,
      modelSelectorEnabled: true,
    },
    global
  });
}

describe("PortalAgentEditorDialog image output policy", () => {
  it("keeps an over-limit value visible and blocks saving instead of silently changing it", async () => {
    const wrapper = mountEditor({ app: imageApp(5), models: [imageModel(1)] });
    await flushPromises();

    const countInputs = wrapper.findAllComponents(ElInputNumberStub);
    expect(countInputs).toHaveLength(2);
    expect(countInputs[1].props("modelValue")).toBe(5);
    const saveButton = wrapper.findAllComponents(ElButtonStub).at(-1)!;
    expect(saveButton.props("disabled")).toBe(true);
    saveButton.vm.$emit("click");
    await flushPromises();
    expect(wrapper.emitted("submit")).toBeUndefined();
  });

  it("preserves configured counts while model capabilities are loading", async () => {
    const wrapper = mountEditor({ app: imageApp(5), models: [], loadingModels: true });
    await flushPromises();

    const countInputs = wrapper.findAllComponents(ElInputNumberStub);
    expect(countInputs[1].props("modelValue")).toBe(5);
    expect(countInputs.every((input) => input.props("disabled"))).toBe(true);
    expect(wrapper.findAllComponents(ElButtonStub).at(-1)!.props("disabled")).toBe(true);
  });

  it("submits the configured maximum when the selected model supports it", async () => {
    const wrapper = mountEditor({ app: imageApp(5), models: [imageModel(5)] });
    await flushPromises();

    const saveButton = wrapper.findAllComponents(ElButtonStub).at(-1)!;
    expect(saveButton.props("disabled")).toBe(false);
    saveButton.vm.$emit("click");
    await flushPromises();

    const payload = wrapper.emitted("submit")?.[0]?.[0] as { runtime_config: PortalAppRecord["runtime_config"] };
    expect(payload.runtime_config.image?.max_output_count).toBe(5);
  });
});
