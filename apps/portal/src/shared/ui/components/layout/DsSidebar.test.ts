import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";

import DsSidebar from "./DsSidebar.vue";

const groups = [
  {
    id: "group-a",
    label: "第一组",
    active: true,
    children: [{ id: "a", label: "入口 A", to: "/a", active: true }]
  },
  {
    id: "group-b",
    label: "第二组",
    children: [{ id: "b", label: "入口 B", to: "/b" }]
  }
];

function mountSidebar(collapsed = false) {
  return mount(DsSidebar, {
    props: { collapsed, groups },
    global: {
      stubs: {
        RouterLink: { props: ["to"], template: "<a><slot /></a>" }
      }
    }
  });
}

describe("DsSidebar grouped navigation", () => {
  it("renders category headings as static labels and keeps every menu visible", () => {
    const wrapper = mountSidebar();
    const bodies = wrapper.findAll(".ds-sidebar__group-body");

    expect(wrapper.findAll(".ds-sidebar__group-head")).toHaveLength(2);
    expect(wrapper.findAll(".ds-sidebar__group-head")[0].element.tagName).toBe("H2");
    expect(wrapper.findAll(".ds-sidebar__group-head button")).toHaveLength(0);
    expect(bodies).toHaveLength(2);
    expect(bodies.every((body) => !(body.attributes("style") ?? "").includes("display: none"))).toBe(true);
  });

  it("keeps every category menu mounted in the collapsed icon rail", () => {
    const wrapper = mountSidebar(true);

    expect(wrapper.findAll(".ds-sidebar__group-body")).toHaveLength(2);
    expect(wrapper.findAll("a")).toHaveLength(2);
  });
});
