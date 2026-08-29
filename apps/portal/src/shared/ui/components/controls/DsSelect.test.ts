import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";

import DsSelect from "./DsSelect.vue";

describe("DsSelect", () => {
  it("forwards the accessible name to the native select and links errors", () => {
    const wrapper = mount(DsSelect, {
      props: {
        options: [{ label: "管理员", value: "admin" }],
        error: "请选择角色"
      },
      attrs: { id: "role", "aria-label": "角色", "aria-describedby": "role-hint" }
    });
    const select = wrapper.get("select");

    expect(select.attributes("id")).toBe("role");
    expect(select.attributes("aria-label")).toBe("角色");
    expect(select.attributes("aria-invalid")).toBe("true");
    expect(select.attributes("aria-describedby")).toBe("role-hint role-error");
    expect(wrapper.get("#role-error").text()).toBe("请选择角色");
  });

  it("preserves option value types when a numeric option is selected", async () => {
    const wrapper = mount(DsSelect, {
      props: { modelValue: "", options: [{ label: "十", value: 10 }] }
    });

    await wrapper.get("select").setValue("10");

    expect(wrapper.emitted("update:modelValue")).toEqual([[10]]);
  });
});
