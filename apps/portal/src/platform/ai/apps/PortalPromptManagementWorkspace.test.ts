import { flushPromises, shallowMount } from "@vue/test-utils";
import { defineComponent, h } from "vue";
import { describe, expect, it, vi } from "vitest";

import PortalPromptManagementWorkspace from "./PortalPromptManagementWorkspace.vue";

import type { PortalAppPromptApi, PortalAppPromptRecord } from "./types";

// 一体面板:透出所有插槽,让列表/详情内容在 shallowMount 下仍可断言
const PortalPagePanelStub = defineComponent({
  name: "PortalPagePanel",
  template:
    '<section><slot name="actions" /><slot name="filters" /><slot /><slot name="pagination" /></section>'
});

const PortalContentCardStub = defineComponent({
  template: '<section><slot name="meta" /><slot name="actions" /><slot /></section>'
});

// DsTable:对首行渲染全部 cell-* 插槽(无数据时渲染 empty 插槽)
const DsTableStub = defineComponent({
  name: "DsTable",
  props: { rows: { type: Array, default: () => [] } },
  setup(props, { slots }) {
    return () => {
      const row = (props.rows as PortalAppPromptRecord[])[0];
      if (!row) return h("div", slots.empty?.());
      return h(
        "div",
        Object.entries(slots)
          .filter(([name]) => name.startsWith("cell-"))
          .map(([, render]) => h("div", render?.({ row })))
      );
    };
  }
});

const ElButtonStub = defineComponent({
  name: "ElButton",
  props: { disabled: Boolean, loading: Boolean },
  emits: ["click"],
  template: '<button :disabled="disabled || loading" @click="$emit(\'click\', $event)"><slot /></button>'
});

const ElSwitchStub = defineComponent({
  name: "ElSwitch",
  props: { modelValue: Boolean, loading: Boolean },
  emits: ["click", "change"],
  template: '<button data-role="status-switch" :disabled="loading" @click="$emit(\'click\', $event); $emit(\'change\', !modelValue)">{{ modelValue ? "启用" : "停用" }}</button>'
});

const PortalPromptEditorDialogStub = defineComponent({
  name: "PortalPromptEditorDialog",
  props: {
    visible: Boolean,
    mode: String,
    detail: Object
  },
  emits: ["submit"],
  template: '<div v-if="visible" data-role="prompt-editor" />'
});

const ElDialogStub = defineComponent({
  props: { modelValue: Boolean },
  template: '<section v-if="modelValue"><slot /><slot name="footer" /></section>'
});

const global = {
  config: { warnHandler: () => undefined },
  stubs: {
    PortalPagePanel: PortalPagePanelStub,
    PortalContentCard: PortalContentCardStub,
    PortalPromptEditorDialog: PortalPromptEditorDialogStub,
    DsTable: DsTableStub,
    ElButton: ElButtonStub,
    ElSwitch: ElSwitchStub,
    ElTag: true,
    ElDialog: ElDialogStub
  }
};

function prompt(overrides: Partial<PortalAppPromptRecord> = {}): PortalAppPromptRecord {
  return {
    owner_type: "tenant",
    id: "prompt-1",
    name: "客户背景",
    description: "客服回复背景",
    status: "active",
    template_text: "客户名称是 {{客户名称}}",
    variables: ["客户名称"],
    updated_at: Date.now(),
    ...overrides
  };
}

function promptApi(item: PortalAppPromptRecord): PortalAppPromptApi {
  return {
    listPrompts: vi.fn().mockResolvedValue({ items: [item] }),
    getPrompt: vi.fn().mockResolvedValue({ prompt: item }),
    createPrompt: vi.fn().mockResolvedValue({ prompt: item }),
    updatePrompt: vi.fn().mockImplementation(async (_promptId, payload) => ({ prompt: { ...item, ...payload } })),
    deletePrompt: vi.fn().mockResolvedValue({ deleted: true })
  };
}

function actionButton(wrapper: ReturnType<typeof shallowMount>, label: string) {
  const button = wrapper.findAll("button").find((item) => item.text() === label);
  if (!button) throw new Error(`missing ${label} button`);
  return button;
}

describe("PortalPromptManagementWorkspace", () => {
  it("stays on the list when a saved prompt has no variables", async () => {
    const created = { ...prompt(), variables: null } as unknown as PortalAppPromptRecord;
    const api = promptApi(created);
    vi.mocked(api.listPrompts)
      .mockResolvedValueOnce({ items: [] })
      .mockResolvedValueOnce({ items: [created] });
    vi.mocked(api.createPrompt).mockResolvedValue({ prompt: created });
    const wrapper = shallowMount(PortalPromptManagementWorkspace, {
      props: { api, scope: "tenant" },
      global
    });
    await flushPromises();

    await actionButton(wrapper, "新建提示词").trigger("click");
    wrapper.findComponent(PortalPromptEditorDialogStub).vm.$emit("submit", {
      name: created.name,
      description: created.description,
      status: created.status,
      template_text: created.template_text
    });
    await flushPromises();

    expect(api.createPrompt).toHaveBeenCalled();
    expect(actionButton(wrapper, "新建提示词").exists()).toBe(true);
    expect(wrapper.findComponent(PortalPromptEditorDialogStub).props("visible")).toBe(false);
  });

  it("opens the shared editor dialog from the list after loading prompt detail", async () => {
    const item = prompt();
    const api = promptApi(item);
    const wrapper = shallowMount(PortalPromptManagementWorkspace, {
      props: { api, scope: "tenant" },
      global
    });
    await flushPromises();

    await actionButton(wrapper, "编辑").trigger("click");
    await flushPromises();

    expect(api.getPrompt).toHaveBeenCalledWith(item.id);
    const editor = wrapper.findComponent(PortalPromptEditorDialogStub);
    expect(editor.props("visible")).toBe(true);
    expect(editor.props("mode")).toBe("edit");
    expect(editor.props("detail")).toEqual({ prompt: item });
    expect(wrapper.find("[data-role='prompt-editor']").exists()).toBe(true);
  });

  it("renders prompt detail safely when the API returns null variables", async () => {
    const item = { ...prompt(), variables: null } as unknown as PortalAppPromptRecord;
    const api = promptApi(item);
    const wrapper = shallowMount(PortalPromptManagementWorkspace, {
      props: { api, scope: "tenant" },
      global
    });
    await flushPromises();

    await actionButton(wrapper, "详情").trigger("click");
    await flushPromises();

    expect(api.getPrompt).toHaveBeenCalledWith(item.id);
    expect(wrapper.text()).toContain("无变量");
  });

  it("enables and disables a prompt directly from the list", async () => {
    const item = prompt();
    const api = promptApi(item);
    const notifySuccess = vi.fn();
    const wrapper = shallowMount(PortalPromptManagementWorkspace, {
      props: { api, scope: "user", notifySuccess },
      global
    });
    await flushPromises();

    await wrapper.get("[data-role='status-switch']").trigger("click");
    await flushPromises();

    expect(api.updatePrompt).toHaveBeenCalledWith(item.id, { status: "disabled" });
    expect(wrapper.get("[data-role='status-switch']").text()).toBe("停用");
    expect(notifySuccess).toHaveBeenCalledWith("我的提示词已停用");
  });

  it("deletes a prompt from the list after confirmation", async () => {
    const item = prompt();
    const api = promptApi(item);
    vi.mocked(api.listPrompts)
      .mockResolvedValueOnce({ items: [item] })
      .mockResolvedValueOnce({ items: [] });
    const confirmDelete = vi.fn().mockResolvedValue(true);
    const wrapper = shallowMount(PortalPromptManagementWorkspace, {
      props: { api, scope: "tenant", confirmDelete },
      global
    });
    await flushPromises();

    await actionButton(wrapper, "删除").trigger("click");
    await flushPromises();

    expect(confirmDelete).toHaveBeenCalledWith(item.name);
    expect(api.deletePrompt).toHaveBeenCalledWith(item.id);
    expect(api.listPrompts).toHaveBeenCalledTimes(2);
  });
});
