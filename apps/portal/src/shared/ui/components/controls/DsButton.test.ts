import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";

import DsButton from "./DsButton.vue";

describe("DsButton", () => {
  it("keeps the native button contract and exposes loading state", async () => {
    const wrapper = mount(DsButton, {
      props: { loading: true },
      attrs: { "aria-label": "保存" },
      slots: { default: "保存" }
    });
    const button = wrapper.get("button");

    expect(button.attributes("type")).toBe("button");
    expect(button.attributes("aria-label")).toBe("保存");
    expect(button.attributes("aria-busy")).toBe("true");
    expect(button.attributes("aria-disabled")).toBe("true");
    expect((button.element as HTMLButtonElement).disabled).toBe(true);
    expect(wrapper.find(".ds-btn__spinner").exists()).toBe(true);

    await button.trigger("click");
    expect(wrapper.emitted("click")).toBeUndefined();
  });

  it("does not add busy semantics to an idle button", () => {
    const wrapper = mount(DsButton, { props: { disabled: true } });
    const button = wrapper.get("button");

    expect(button.attributes("aria-busy")).toBeUndefined();
    expect(button.attributes("aria-disabled")).toBe("true");
    expect((button.element as HTMLButtonElement).disabled).toBe(true);
  });
});
