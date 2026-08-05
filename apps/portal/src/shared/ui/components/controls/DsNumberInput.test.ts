import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";

import DsNumberInput from "./DsNumberInput.vue";

describe("DsNumberInput", () => {
  it("preserves decimal editing states and emits the parsed number", async () => {
    const wrapper = mount(DsNumberInput, {
      props: { modelValue: 1, min: 0, precision: 4 }
    });
    const input = wrapper.get("input");

    await input.trigger("focus");
    await input.setValue("0.");
    expect((input.element as HTMLInputElement).value).toBe("0.");

    await input.setValue("0.15");
    expect((input.element as HTMLInputElement).value).toBe("0.15");
    expect(wrapper.emitted("update:modelValue")?.at(-1)).toEqual([0.15]);
  });

  it("keeps trailing zeroes while the user continues typing", async () => {
    const wrapper = mount(DsNumberInput, {
      props: { modelValue: 1, min: 0, precision: 4 }
    });
    const input = wrapper.get("input");

    await input.trigger("focus");
    await input.setValue("1.0");
    expect((input.element as HTMLInputElement).value).toBe("1.0");

    await input.setValue("1.05");
    expect((input.element as HTMLInputElement).value).toBe("1.05");
    expect(wrapper.emitted("update:modelValue")?.at(-1)).toEqual([1.05]);
  });

  it("rounds and compacts the value on blur", async () => {
    const wrapper = mount(DsNumberInput, {
      props: { modelValue: 1, precision: 4 }
    });
    const input = wrapper.get("input");

    await input.trigger("focus");
    await input.setValue("1.23456");
    await input.trigger("blur");

    expect((input.element as HTMLInputElement).value).toBe("1.2346");
    expect(wrapper.emitted("update:modelValue")?.at(-1)).toEqual([1.2346]);
    expect(wrapper.emitted("change")?.at(-1)).toEqual([1.2346]);
  });

  it("clamps an out-of-range draft when editing finishes", async () => {
    const wrapper = mount(DsNumberInput, {
      props: { modelValue: 1, min: 0.1, max: 2, precision: 4 }
    });
    const input = wrapper.get("input");

    await input.trigger("focus");
    await input.setValue("0.01");
    await input.trigger("blur");

    expect((input.element as HTMLInputElement).value).toBe("0.1");
    expect(wrapper.emitted("update:modelValue")?.at(-1)).toEqual([0.1]);
  });

  it("supports an empty value when the caller allows it", async () => {
    const wrapper = mount(DsNumberInput, {
      props: { modelValue: 1, allowEmpty: true, precision: 4 }
    });
    const input = wrapper.get("input");

    await input.trigger("focus");
    await input.setValue("");
    await input.trigger("blur");

    expect((input.element as HTMLInputElement).value).toBe("");
    expect(wrapper.emitted("update:modelValue")?.at(-1)).toEqual([null]);
    expect(wrapper.emitted("change")?.at(-1)).toEqual([null]);
  });

  it("syncs external values when the field is not being edited", async () => {
    const wrapper = mount(DsNumberInput, {
      props: { modelValue: 1, precision: 4 }
    });

    await wrapper.setProps({ modelValue: 0.25 });

    expect((wrapper.get("input").element as HTMLInputElement).value).toBe("0.25");
  });
});
