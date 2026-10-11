import { flushPromises, mount } from "@vue/test-utils";
import { defineComponent, h } from "vue";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { usePortalImageJobs } from "./usePortalImageJobs";
import type { PortalImageApi, PortalImageJobRecord } from "./types";

const completed: PortalImageJobRecord = {
  id: "accepted-task", model_code: "image-model", prompt: "cat", status: "completed", created_at: 1,
  storage_policy: "temporary", raw_image_retained: false, caller_charge_usd: 0.1,
  image_count: 0, inline_count: 0, url_count: 0
};
const cleanups: Array<() => void> = [];
function setup() {
  const api: PortalImageApi = {
    listJobs: vi.fn().mockResolvedValue([]), listModels: vi.fn().mockResolvedValue([]),
    createTask: vi.fn().mockResolvedValue({ task_id: completed.id, status: "pending" }),
    getTask: vi.fn().mockRejectedValue(new Error("offline")),
    cancelTask: vi.fn(), deleteTask: vi.fn()
  };
  const notifySuccess = vi.fn();
  const notifyError = vi.fn();
  let state!: ReturnType<typeof usePortalImageJobs>;
  const wrapper = mount(defineComponent({
    setup() {
      state = usePortalImageJobs({ api, usageMessage: "usage", pollIntervalMs: 1000, notifySuccess, notifyError }, vi.fn());
      return () => h("div");
    }
  }));
  cleanups.push(() => wrapper.unmount());
  return { api, state, notifyError, notifySuccess };
}
beforeEach(() => vi.useFakeTimers());
afterEach(() => {
  cleanups.splice(0).forEach((cleanup) => cleanup());
  vi.useRealTimers();
  vi.restoreAllMocks();
});

describe("image task acceptance and refresh", () => {
  it("keeps an accepted task through failed reads and recovers without another POST", async () => {
    const { api, state, notifySuccess, notifyError } = setup();
    const onCreated = vi.fn();
    await state.submitTask({ model: "image-model", prompt: "cat" }, onCreated);
    expect(onCreated).toHaveBeenCalledOnce();
    expect(notifySuccess).toHaveBeenCalledOnce();
    expect(notifyError).not.toHaveBeenCalled();
    expect(state.jobs.value[0]).toMatchObject({ id: completed.id, prompt: "cat", status: "pending" });
    expect(state.errors.value.join()).toContain("请勿重复提交");
    await state.refreshAll();
    expect(state.jobs.value[0].id).toBe(completed.id);
    vi.mocked(api.getTask).mockResolvedValue(completed);
    await vi.advanceTimersByTimeAsync(4000);
    expect(state.jobs.value).toEqual([completed]);
    expect(state.errors.value).toEqual([]);
    expect(api.createTask).toHaveBeenCalledOnce();
  });

  it("shares the submission lock between retry and the composer", async () => {
    const { api, state } = setup();
    let accept!: (value: { task_id: string; status: string }) => void;
    vi.mocked(api.createTask).mockReturnValueOnce(new Promise((resolve) => { accept = resolve; }));
    const submit = state.submitTask({ model: "image-model", prompt: "cat" });
    await state.retryJob({ ...completed, status: "failed" });
    await state.submitTask({ model: "image-model", prompt: "another cat" });
    expect(api.createTask).toHaveBeenCalledOnce();
    accept({ task_id: completed.id, status: "pending" });
    await submit;
    expect(state.submitting.value).toBe(false);
  });

  it("preserves existing records when list/model refresh fails and clears errors on retry", async () => {
    const { api, state } = setup();
    vi.mocked(api.listJobs).mockResolvedValue([completed]);
    await state.refreshAll();
    vi.mocked(api.listJobs).mockRejectedValue(new Error("offline"));
    vi.mocked(api.listModels).mockRejectedValue(new Error("offline"));
    await state.refreshAll();
    expect(state.jobs.value).toEqual([completed]);
    expect(state.errors.value).toHaveLength(2);
    expect(state.loading.value).toBe(false);
    vi.mocked(api.listJobs).mockResolvedValue([completed]);
    vi.mocked(api.listModels).mockResolvedValue([]);
    await state.refreshAll();
    expect(state.errors.value).toEqual([]);
  });

  it("leaves editor inputs intact when creation fails", async () => {
    const { api, state, notifyError, notifySuccess } = setup();
    vi.mocked(api.createTask).mockRejectedValue(new Error("额度不足"));
    const reset = vi.fn();
    await state.submitTask({ model: "image-model", prompt: "cat" }, reset);
    await flushPromises();
    expect(reset).not.toHaveBeenCalled();
    expect(notifyError).toHaveBeenCalledWith("额度不足");
    expect(notifySuccess).not.toHaveBeenCalled();
    expect(state.jobs.value).toEqual([]);
    expect(state.submitting.value).toBe(false);
  });
});
