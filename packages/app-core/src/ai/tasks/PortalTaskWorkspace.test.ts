import { flushPromises, shallowMount } from "@vue/test-utils";
import { afterEach, describe, expect, it, vi } from "vitest";

import PortalTaskTable from "./PortalTaskTable.vue";
import PortalTaskWorkspace from "./PortalTaskWorkspace.vue";

import type { PortalTaskApi, PortalTaskRecord } from "./types";

function task(overrides: Partial<PortalTaskRecord> = {}): PortalTaskRecord {
  return {
    id: "task-1",
    type: "images.generation",
    source: "app_key",
    status: "running",
    model: "image-model",
    owner: { scope: "tenant" },
    permissions: { read_only: false, can_cancel: true, can_delete: false },
    attempt: 1,
    result_available: false,
    created_at: Date.now(),
    ...overrides
  };
}

function apiWith(items: PortalTaskRecord[]): PortalTaskApi {
  return {
    listTasks: vi.fn().mockResolvedValue({ items, has_more: false }),
    getTask: vi.fn().mockImplementation(async (id) => items.find((item) => item.id === id) || items[0]),
    cancelTask: vi.fn(),
    deleteTask: vi.fn().mockResolvedValue({ deleted: true })
  };
}

const workspaceStubs = {
  // DsUI 迁移后页骨架是 PortalPagePanel:显式 stub 以渲染各插槽(auto-stub 不渲染插槽)
  PortalPagePanel: {
    template:
      "<section><slot name='actions' /><slot name='filters' /><slot /><slot name='pagination' /></section>"
  }
};

afterEach(() => {
  vi.useRealTimers();
  vi.restoreAllMocks();
});

describe("PortalTaskWorkspace", () => {
  it("keeps tenant-visible user tasks read-only", async () => {
    const userTask = task({
      owner: { scope: "user", user_id: "user-a" },
      permissions: { read_only: true, can_cancel: false, can_delete: false }
    });
    const api = apiWith([userTask]);
    const wrapper = shallowMount(PortalTaskWorkspace, {
      props: { api, mode: "tenant" },
      global: { config: { warnHandler: () => undefined }, stubs: workspaceStubs }
    });
    await flushPromises();

    const table = wrapper.findComponent(PortalTaskTable);
    expect(table.props("tasks")).toEqual([userTask]);
    table.vm.$emit("cancel", userTask);
    table.vm.$emit("delete", userTask);
    await flushPromises();

    expect(api.cancelTask).not.toHaveBeenCalled();
    expect(api.deleteTask).not.toHaveBeenCalled();
    wrapper.unmount();
  });

  it("deletes an owned terminal task after confirmation", async () => {
    const completed = task({
      id: "task-completed",
      status: "completed",
      permissions: { read_only: false, can_cancel: false, can_delete: true }
    });
    const api = apiWith([completed]);
    const confirmDelete = vi.fn().mockResolvedValue(true);
    const wrapper = shallowMount(PortalTaskWorkspace, {
      props: { api, mode: "user", confirmDelete },
      global: { config: { warnHandler: () => undefined }, stubs: workspaceStubs }
    });
    await flushPromises();

    wrapper.findComponent(PortalTaskTable).vm.$emit("delete", completed);
    await flushPromises();

    expect(confirmDelete).toHaveBeenCalledWith(completed);
    expect(api.deleteTask).toHaveBeenCalledWith(completed.id);
    expect(wrapper.findComponent(PortalTaskTable).props("tasks")).toEqual([]);
    wrapper.unmount();
  });

  it("polls active tasks with the configured interval", async () => {
    vi.useFakeTimers();
    const active = task();
    const api = apiWith([active]);
    vi.mocked(api.getTask).mockResolvedValue({
      ...active,
      status: "completed",
      permissions: { read_only: false, can_cancel: false, can_delete: true }
    });
    const wrapper = shallowMount(PortalTaskWorkspace, {
      props: { api, mode: "user", pollIntervalMs: 1_250 },
      global: { config: { warnHandler: () => undefined }, stubs: workspaceStubs }
    });
    await flushPromises();

    await vi.advanceTimersByTimeAsync(1_249);
    expect(api.getTask).not.toHaveBeenCalled();
    await vi.advanceTimersByTimeAsync(1);
    await flushPromises();
    expect(api.getTask).toHaveBeenCalledWith(active.id);

    wrapper.unmount();
  });
});
