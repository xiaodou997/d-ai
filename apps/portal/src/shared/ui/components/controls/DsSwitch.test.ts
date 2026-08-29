import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";

import DsSwitch from "./DsSwitch.vue";

describe("DsSwitch", () => {
  it("exposes switch state and emits a toggle", async () => {
    const wrapper = mount(DsSwitch, { props: { modelValue: false }, attrs: { "aria-label": "启用功能" } });
    const button = wrapper.get("button");

    expect(button.attributes("role")).toBe("switch");
    expect(button.attributes("aria-label")).toBe("启用功能");
    expect(button.attributes("aria-checked")).toBe("false");

    await button.trigger("click");
    expect(wrapper.emitted("update:modelValue")).toEqual([[true]]);
  });

  it("marks disabled switches and ignores activation", async () => {
    const wrapper = mount(DsSwitch, { props: { modelValue: true, disabled: true } });
    const button = wrapper.get("button");

    expect(button.attributes("aria-disabled")).toBe("true");
    expect((button.element as HTMLButtonElement).disabled).toBe(true);
    await button.trigger("click");
    expect(wrapper.emitted("update:modelValue")).toBeUndefined();
  });
});
