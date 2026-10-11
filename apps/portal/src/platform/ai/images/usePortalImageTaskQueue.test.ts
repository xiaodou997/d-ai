import { flushPromises, mount } from "@vue/test-utils";
import { defineComponent, h, shallowRef } from "vue";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { usePortalImageTaskQueue } from "./usePortalImageTaskQueue";
import type { PortalImageJobRecord } from "./types";

const running: PortalImageJobRecord = {
  id: "task-1", model_code: "image-model", prompt: "cat", status: "running", created_at: 1,
  storage_policy: "temporary", raw_image_retained: false, caller_charge_usd: 0,
  image_count: 0, inline_count: 0, url_count: 0
};
function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => { resolve = done; });
  return { promise, resolve };
}
const cleanups: Array<() => void> = [];
function setup(initial = [running]) {
  const jobs = shallowRef(initial);
  const api = { listJobs: vi.fn().mockResolvedValue(initial), getTask: vi.fn().mockResolvedValue(running) };
  let queue!: ReturnType<typeof usePortalImageTaskQueue>;
  const wrapper = mount(defineComponent({
    setup() {
      queue = usePortalImageTaskQueue({ api, jobs, pollIntervalMs: 1000 });
      return () => h("div");
    }
  }));
  cleanups.push(() => wrapper.unmount());
  return { jobs, api, queue, wrapper };
}
beforeEach(() => vi.useFakeTimers());
afterEach(() => {
  cleanups.splice(0).forEach((cleanup) => cleanup());
  vi.useRealTimers();
  vi.restoreAllMocks();
});

describe("image task polling", () => {
  it("waits for a slow refresh to finish before scheduling another", async () => {
    const { api } = setup();
    const response = deferred<PortalImageJobRecord>();
    api.getTask.mockReturnValueOnce(response.promise);
    await vi.advanceTimersByTimeAsync(1000);
    await vi.advanceTimersByTimeAsync(5000);
    expect(api.getTask).toHaveBeenCalledTimes(1);
    response.resolve(running);
    await flushPromises();
    await vi.advanceTimersByTimeAsync(999);
    expect(api.getTask).toHaveBeenCalledTimes(1);
    await vi.advanceTimersByTimeAsync(1);
    expect(api.getTask).toHaveBeenCalledTimes(2);
  });

  it("applies successful tasks independently and backs off only until recovery", async () => {
    const second = { ...running, id: "task-2" };
    const { api, jobs, queue } = setup([running, second]);
    api.getTask.mockImplementation(async (id: string) => {
      if (id === running.id) throw new Error("offline");
      return { ...second, status: "completed" };
    });
    await vi.advanceTimersByTimeAsync(1000);
    expect(jobs.value.find((job) => job.id === second.id)?.status).toBe("completed");
    expect(queue.syncError.value).toContain("自动重试");
    await vi.advanceTimersByTimeAsync(1999);
    expect(api.getTask).toHaveBeenCalledTimes(2);
    api.getTask.mockResolvedValue(running);
    await vi.advanceTimersByTimeAsync(1);
    expect(api.getTask).toHaveBeenCalledTimes(3);
    expect(queue.syncError.value).toBe("");
    await vi.advanceTimersByTimeAsync(1000);
    expect(api.getTask).toHaveBeenCalledTimes(4);
  });

  it("serializes manual refresh with polling and preserves newer cancellation", async () => {
    const { api, jobs, queue } = setup();
    const response = deferred<PortalImageJobRecord>();
    api.getTask.mockReturnValueOnce(response.promise);
    await vi.advanceTimersByTimeAsync(1000);
    const refresh = queue.refreshJobs();
    expect(api.listJobs).not.toHaveBeenCalled();
    const cancelled = { ...running, status: "cancelled" };
    queue.mergeJob(cancelled);
    api.listJobs.mockResolvedValue([cancelled]);
    response.resolve(running);
    await refresh;
    expect(jobs.value).toEqual([cancelled]);
    expect(api.listJobs).toHaveBeenCalledOnce();
  });

  it("ignores a stale list after deletion and retains accepted tasks missing from the list", async () => {
    const old = { ...running, status: "completed" };
    const { api, jobs, queue } = setup([old]);
    const response = deferred<PortalImageJobRecord[]>();
    api.listJobs.mockReturnValueOnce(response.promise);
    const refresh = queue.refreshJobs();
    queue.removeJob(old.id);
    const accepted = { ...running, id: "new-task", status: "pending" };
    queue.addAcceptedJob(accepted);
    api.getTask.mockRejectedValue(new Error("not replicated yet"));
    response.resolve([old]);
    await refresh;
    expect(jobs.value).toEqual([accepted]);
    expect(queue.syncError.value).toContain("请勿重复提交");
  });

  it("pauses hidden tabs, aborts the old read and refreshes on return", async () => {
    const visibility = vi.spyOn(document, "hidden", "get").mockReturnValue(false);
    const { api, jobs } = setup();
    const response = deferred<PortalImageJobRecord>();
    api.getTask.mockReturnValueOnce(response.promise);
    await vi.advanceTimersByTimeAsync(1000);
    const signal = api.getTask.mock.calls[0][1] as AbortSignal;
    visibility.mockReturnValue(true);
    document.dispatchEvent(new Event("visibilitychange"));
    expect(signal.aborted).toBe(true);
    response.resolve({ ...running, status: "completed" });
    await flushPromises();
    expect(jobs.value[0].status).toBe("running");
    await vi.advanceTimersByTimeAsync(5000);
    expect(api.getTask).toHaveBeenCalledOnce();
    visibility.mockReturnValue(false);
    document.dispatchEvent(new Event("visibilitychange"));
    await flushPromises();
    expect(api.listJobs).toHaveBeenCalledOnce();
    await vi.advanceTimersByTimeAsync(1000);
    expect(api.getTask).toHaveBeenCalledTimes(2);
  });

  it("aborts timed-out reads and ignores responses after unmount", async () => {
    const { api, jobs, queue, wrapper } = setup();
    api.getTask.mockImplementationOnce((_id: string, signal: AbortSignal) => new Promise((_resolve, reject) => {
      signal.addEventListener("abort", () => reject(new Error("timeout")), { once: true });
    }));
    await vi.advanceTimersByTimeAsync(16_000);
    expect(queue.syncError.value).toContain("自动重试");
    const response = deferred<PortalImageJobRecord>();
    api.getTask.mockReturnValueOnce(response.promise);
    await vi.advanceTimersByTimeAsync(2000);
    const signal = api.getTask.mock.calls[1][1] as AbortSignal;
    wrapper.unmount();
    expect(signal.aborted).toBe(true);
    response.resolve({ ...running, status: "completed" });
    await flushPromises();
    expect(jobs.value[0].status).toBe("running");
    await vi.advanceTimersByTimeAsync(60_000);
    expect(api.getTask).toHaveBeenCalledTimes(2);
  });
});
