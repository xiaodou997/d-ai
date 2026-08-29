import { nextTick } from "vue";
import { mount } from "@vue/test-utils";
import { afterEach, describe, expect, it } from "vitest";

import DsConfirmDialog from "./DsConfirmDialog.vue";
import DsDrawer from "./DsDrawer.vue";
import DsModal from "./DsModal.vue";

afterEach(() => {
  document.body.style.overflow = "";
  document.querySelectorAll(".ds-modal, .ds-drawer, .ds-confirm").forEach((node) => node.remove());
});

describe("DsModal", () => {
  it("labels the dialog, locks scroll, restores focus, and closes on Escape", async () => {
    const trigger = document.createElement("button");
    document.body.append(trigger);
    trigger.focus();

    const wrapper = mount(DsModal, {
      props: { open: true, title: "编辑配置" },
      slots: { default: "表单内容" },
      attachTo: document.body
    });
    await nextTick();

    const dialog = document.body.querySelector<HTMLElement>('[role="dialog"]');
    expect(dialog).not.toBeNull();
    expect(dialog?.getAttribute("aria-labelledby")).toBe(dialog?.querySelector("h2")?.id);
    expect(dialog?.getAttribute("aria-describedby")).toBe(dialog?.querySelector(".ds-modal__body")?.id);
    expect(document.body.style.overflow).toBe("hidden");
    expect(document.activeElement).toBe(dialog);

    dialog?.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true }));
    await nextTick();
    expect(wrapper.emitted("close")).toHaveLength(1);

    await wrapper.setProps({ open: false });
    await nextTick();
    expect(document.body.style.overflow).toBe("");
    expect(document.activeElement).toBe(trigger);
    wrapper.unmount();
    trigger.remove();
  });
});

describe("DsDrawer", () => {
  it("exposes labelled dialog semantics and closes on Escape", async () => {
    const wrapper = mount(DsDrawer, {
      props: { open: true, title: "任务详情", subtitle: "运行结果" },
      slots: { default: "详情内容" },
      attachTo: document.body
    });
    await nextTick();

    const drawer = document.body.querySelector<HTMLElement>('[role="dialog"]');
    expect(drawer).not.toBeNull();
    expect(drawer?.getAttribute("aria-labelledby")).toBe(drawer?.querySelector("h2")?.id);
    expect(drawer?.getAttribute("aria-describedby")).toBe(drawer?.querySelector(".ds-drawer__body")?.id);
    expect(document.activeElement).toBe(drawer);

    drawer?.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true }));
    await nextTick();
    expect(wrapper.emitted("close")).toHaveLength(1);

    wrapper.unmount();
  });
});

describe("DsConfirmDialog", () => {
  it("keeps the alert dialog labelled and makes loading confirmation busy", async () => {
    const wrapper = mount(DsConfirmDialog, {
      props: { open: true, title: "删除密钥", message: "此操作不可撤销", loading: true },
      attachTo: document.body
    });
    await nextTick();

    const dialog = document.body.querySelector<HTMLElement>('[role="alertdialog"]');
    expect(dialog).not.toBeNull();
    expect(dialog?.getAttribute("aria-labelledby")).toBe(dialog?.querySelector("h3")?.id);
    expect(dialog?.getAttribute("aria-describedby")).toBe(dialog?.querySelector(".ds-confirm__message")?.id);
    expect(dialog?.querySelector(".ds-btn__spinner")).not.toBeNull();

    dialog?.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true }));
    await nextTick();
    expect(wrapper.emitted("cancel")).toBeUndefined();
    expect(wrapper.emitted("update:open")).toBeUndefined();

    wrapper.unmount();
  });
});
