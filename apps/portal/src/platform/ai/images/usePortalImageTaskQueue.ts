import { computed, onUnmounted, shallowRef, toValue, watch } from "vue";
import type { MaybeRefOrGetter, ShallowRef } from "vue";
import type { PortalImageApi, PortalImageJobRecord } from "./types";

export const PORTAL_IMAGE_ACTIVE_STATUSES = new Set(["pending", "running"]);

interface PortalImageTaskQueueOptions {
  api: Pick<PortalImageApi, "listJobs" | "getTask">;
  jobs: ShallowRef<PortalImageJobRecord[]>;
  pollIntervalMs?: MaybeRefOrGetter<number | undefined>;
}

export function usePortalImageTaskQueue(options: PortalImageTaskQueueOptions) {
  const pendingDetails = shallowRef(new Set<string>());
  const listError = shallowRef("");
  const syncError = shallowRef("");
  let timer: number | undefined;
  let controller: AbortController | undefined;
  let inFlight: Promise<void> | undefined;
  let failures = 0;
  let disposed = false;
  const intervalMs = computed(() => {
    const value = Number(toValue(options.pollIntervalMs));
    return Number.isFinite(value) && value > 0 ? value : 20_000;
  });
  const needsRefresh = (job: PortalImageJobRecord) =>
    PORTAL_IMAGE_ACTIVE_STATUSES.has(job.status) || pendingDetails.value.has(job.id);
  const activeCount = computed(() => options.jobs.value.filter(needsRefresh).length);

  function clearPending(id: string) {
    if (!pendingDetails.value.has(id)) return;
    const next = new Set(pendingDetails.value);
    next.delete(id);
    pendingDetails.value = next;
  }

  function mergeJob(job: PortalImageJobRecord) {
    if (disposed) return;
    clearPending(job.id);
    options.jobs.value = [job, ...options.jobs.value.filter((item) => item.id !== job.id)];
  }

  // The create response is authoritative acceptance, even if the next GET fails.
  function addAcceptedJob(job: PortalImageJobRecord) {
    mergeJob(job);
    pendingDetails.value = new Set([...pendingDetails.value, job.id]);
  }

  function removeJob(id: string) {
    if (disposed) return;
    clearPending(id);
    options.jobs.value = options.jobs.value.filter((item) => item.id !== id);
  }

  function stopPolling() {
    window.clearTimeout(timer);
    timer = undefined;
  }

  function startPolling() {
    if (disposed || document.hidden || inFlight || timer !== undefined || activeCount.value === 0) return;
    const delay = Math.min(intervalMs.value * 2 ** Math.min(failures, 3), Math.max(intervalMs.value, 120_000));
    timer = window.setTimeout(() => {
      timer = undefined;
      void refreshActiveJobs();
    }, delay);
  }

  function reconcileList(items: PortalImageJobRecord[], before: Map<string, PortalImageJobRecord>) {
    const current = new Map(options.jobs.value.map((job) => [job.id, job]));
    const next = new Map<string, PortalImageJobRecord>();
    for (const job of items) {
      // A cancel/delete/create that completed during the GET wins over its stale response.
      if (current.get(job.id) !== before.get(job.id)) continue;
      next.set(job.id, job);
      clearPending(job.id);
    }
    for (const job of current.values()) {
      if (job !== before.get(job.id) || (needsRefresh(job) && !next.has(job.id))) next.set(job.id, job);
    }
    options.jobs.value = [...next.values()];
  }

  function runRefresh(includeList: boolean): Promise<void> {
    if (disposed || document.hidden) return Promise.resolve();
    if (inFlight) return inFlight;
    if (!includeList && activeCount.value === 0) return Promise.resolve();
    stopPolling();
    const request = new AbortController();
    controller = request;
    const timeout = window.setTimeout(() => request.abort(), 15_000);
    // Aborted reads are ignored after hiding/unmounting; a timeout while visible is a failure.
    const inactive = () => disposed || document.hidden || controller !== request;
    inFlight = (async () => {
      let failed = false;
      let listedIds: Set<string> | undefined;
      try {
        if (includeList) {
          const before = new Map(options.jobs.value.map((job) => [job.id, job]));
          try {
            const items = await options.api.listJobs(request.signal);
            if (inactive()) return;
            reconcileList(items, before);
            listedIds = new Set(items.map((job) => job.id));
            listError.value = "";
          } catch {
            if (inactive()) return;
            listError.value = "任务列表加载失败，已保留现有记录，请刷新重试。";
            failed = true;
          }
        }
        const active = options.jobs.value.filter((job) => needsRefresh(job) && !listedIds?.has(job.id));
        const results = await Promise.allSettled(active.map((job) => options.api.getTask(job.id, request.signal)));
        if (inactive()) return;
        let detailFailed = false;
        results.forEach((result, index) => {
          const previous = active[index];
          if (options.jobs.value.find((job) => job.id === previous.id) !== previous) return;
          if (result.status === "fulfilled") mergeJob(result.value);
          else detailFailed = true;
        });
        syncError.value = detailFailed ? "部分任务状态更新失败，任务仍保留在列表中，将自动重试；请勿重复提交。" : "";
        failed ||= detailFailed;
      } finally {
        window.clearTimeout(timeout);
        inFlight = undefined;
        if (!inactive()) failures = failed ? failures + 1 : 0;
        if (controller === request) controller = undefined;
        startPolling();
      }
    })();
    return inFlight;
  }

  function refreshActiveJobs() {
    return runRefresh(false);
  }

  async function refreshJobs() {
    // Manual list refreshes also serialize with polling, so older results cannot race newer reads.
    if (inFlight) await inFlight;
    return runRefresh(true);
  }

  function onVisibilityChange() {
    stopPolling();
    if (document.hidden) {
      controller?.abort();
      controller = undefined;
    } else {
      void refreshJobs();
    }
  }

  watch(activeCount, (count) => {
    if (count > 0) startPolling();
    else {
      stopPolling();
      syncError.value = "";
    }
  }, { immediate: true });
  document.addEventListener("visibilitychange", onVisibilityChange);
  onUnmounted(() => {
    disposed = true;
    stopPolling();
    controller?.abort();
    controller = undefined;
    document.removeEventListener("visibilitychange", onVisibilityChange);
  });

  return {
    mergeJob, addAcceptedJob, removeJob, refreshJobs, refreshActiveJobs, listError, syncError,
    pendingDetails: computed(() => pendingDetails.value)
  };
}
