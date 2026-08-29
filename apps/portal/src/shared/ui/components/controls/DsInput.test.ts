import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";

import DsInput from "./DsInput.vue";

describe("DsInput", () => {
  it("forwards native field attributes and associates validation text", () => {
    const wrapper = mount(DsInput, {
      props: { modelValue: "Ada", error: "请输入有效邮箱" },
      attrs: { id: "email", name: "email", autocomplete: "email", "aria-describedby": "email-hint" }
    });
    const input = wrapper.get("input");

    expect(input.attributes("id")).toBe("email");
    expect(input.attributes("name")).toBe("email");
    expect(input.attributes("autocomplete")).toBe("email");
    expect(input.attributes("aria-invalid")).toBe("true");
    expect(input.attributes("aria-describedby")).toBe("email-hint email-error");
    expect(wrapper.get("#email-error").attributes("aria-live")).toBe("polite");
    expect(wrapper.find(".ds-input").attributes("id")).toBeUndefined();
  });

  it("emits a string value from native input events", async () => {
    const wrapper = mount(DsInput, { props: { modelValue: "" } });

    await wrapper.get("input").setValue("hello");

    expect(wrapper.emitted("update:modelValue")).toEqual([["hello"]]);
  });
});
