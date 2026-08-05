import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";

import DsTabs from "./DsTabs.vue";

const tabs = [
  { key: "all", label: "全部" },
  { key: "active", label: "进行中" },
  { key: "done", label: "已完成" }
];

describe("DsTabs", () => {
  it("renders every tab label inside the segmented list", () => {
    const wrapper = mount(DsTabs, { props: { tabs, modelValue: "all" } });
    const buttons = wrapper.findAll("button");

    expect(wrapper.find(".ds-tabs__list").exists()).toBe(true);
    expect(buttons).toHaveLength(3);
    expect(buttons.map((button) => button.text())).toEqual(["全部", "进行中", "已完成"]);
  });

  it("emits update:modelValue with the tab key when clicked", async () => {
    const wrapper = mount(DsTabs, { props: { tabs, modelValue: "all" } });

    await wrapper.findAll("button")[1].trigger("click");

    expect(wrapper.emitted("update:modelValue")).toEqual([["active"]]);
  });

  it("marks the active tab with the active class and aria-selected", () => {
    const wrapper = mount(DsTabs, { props: { tabs, modelValue: "active" } });
    const buttons = wrapper.findAll("button");

    expect(buttons[0].classes()).not.toContain("ds-tabs__tab--active");
    expect(buttons[1].classes()).toContain("ds-tabs__tab--active");
    expect(buttons[1].attributes("aria-selected")).toBe("true");
    expect(buttons[0].attributes("aria-selected")).toBe("false");
  });
});
