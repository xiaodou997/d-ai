import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";

import ServiceInstancesPanel from "./ServiceInstancesPanel.vue";

describe("ServiceInstancesPanel", () => {
  it("renders the online-only empty state", () => {
    const wrapper = mount(ServiceInstancesPanel, { props: { instances: [] } });

    expect(wrapper.text()).toContain("暂无在线实例");
    expect(wrapper.text()).not.toContain("观察到的实例");
  });
});
