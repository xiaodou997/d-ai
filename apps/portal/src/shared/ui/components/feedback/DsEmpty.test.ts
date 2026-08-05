import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";

import DsEmpty from "./DsEmpty.vue";

describe("DsEmpty", () => {
  it("renders the default title and a default icon", () => {
    const wrapper = mount(DsEmpty);

    expect(wrapper.find(".ds-empty__title").text()).toBe("暂无数据");
    expect(wrapper.find(".ds-empty__icon svg").exists()).toBe(true);
    expect(wrapper.find(".ds-empty__description").exists()).toBe(false);
    expect(wrapper.find(".ds-empty__action").exists()).toBe(false);
  });

  it("renders the provided title and description", () => {
    const wrapper = mount(DsEmpty, {
      props: { title: "没有任务", description: "当前筛选条件下暂无任务记录" }
    });

    expect(wrapper.find(".ds-empty__title").text()).toBe("没有任务");
    expect(wrapper.find(".ds-empty__description").text()).toBe("当前筛选条件下暂无任务记录");
  });

  it("renders the action slot", () => {
    const wrapper = mount(DsEmpty, {
      props: { title: "暂无数据" },
      slots: { action: '<button class="retry">重新加载</button>' }
    });

    const action = wrapper.find(".ds-empty__action");
    expect(action.exists()).toBe(true);
    expect(action.find("button.retry").text()).toBe("重新加载");
  });

  it("renders a custom icon slot instead of the default", () => {
    const wrapper = mount(DsEmpty, {
      slots: { icon: '<span class="custom-icon">icon</span>' }
    });

    expect(wrapper.find(".custom-icon").exists()).toBe(true);
    expect(wrapper.find(".ds-empty__icon svg").exists()).toBe(false);
  });
});
