import { defineComponent } from "vue";
import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";

import ServiceAccessEditor from "./ServiceAccessEditor.vue";

const SegmentedStub = defineComponent({
  name: "ElSegmented",
  props: {
    modelValue: String,
    options: { type: Array, default: () => [] }
  },
  emits: ["update:modelValue"],
  template: `
    <div>
      <span data-test="options">{{ options.map((option) => option.value).join(",") }}</span>
      <button data-test="select-all" @click="$emit('update:modelValue', 'all')">all</button>
    </div>
  `
});

describe("ServiceAccessEditor", () => {
  it("removes all mode for a restricted platform administrator", () => {
    const wrapper = mountEditor(false);
    expect(wrapper.get('[data-test="options"]').text()).toBe("selected");
  });

  it("clears selected service IDs when switching to all", async () => {
    const wrapper = mountEditor(true);
    await wrapper.get('[data-test="select-all"]').trigger("click");

    expect(wrapper.emitted("update:mode")?.at(-1)).toEqual(["all"]);
    expect(wrapper.emitted("update:serviceIds")?.at(-1)).toEqual([[]]);
  });
});

function mountEditor(allowAll: boolean) {
  return mount(ServiceAccessEditor, {
    props: {
      mode: "selected",
      serviceIds: ["ai"],
      services: [],
      allowAll
    },
    global: {
      stubs: {
        "el-segmented": SegmentedStub,
        "el-checkbox-group": true,
        "el-checkbox": true
      }
    }
  });
}
