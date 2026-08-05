import { computed, onUnmounted, shallowRef, toValue, watch } from "vue";
import type { MaybeRefOrGetter, ShallowRef } from "vue";

import type { PortalImageApi, PortalImageJobRecord } from "./types";

export const PORTAL_IMAGE_ACTIVE_STATUSES = new Set(["pending", "running"]);

interface PortalImageTaskQueueOptions {
  api: PortalImageApi;
  jobs: ShallowRef<PortalImageJobRecord[]>;
  fetchJobs: () => Promise<void>;
  pollIntervalMs?: MaybeRefOrGetter<number | undefined>;
}

export function usePortalImageTaskQueue(options: PortalImageTaskQueueOptions) {
  const pollTimer = shallowRef<number>();
  const activeCount = computed(() =>
    options.jobs.value.filter((job) => PORTAL_IMAGE_ACTIVE_STATUSES.has(job.status)).length
  );
  const intervalMs = computed(() => {
    const value = Number(toValue(options.pollIntervalMs));
    return Number.isFinite(value) && value > 0 ? value : 20_000;
  });

  function mergeJob(next: PortalImageJobRecord) {
    const rest = options.jobs.value.filter((item) => item.id !== next.id);
    options.jobs.value = [next, ...rest];
  }

  function mergeJobs(nextJobs: PortalImageJobRecord[]) {
    if (nextJobs.length === 0) return;
    const merged = new Map(options.jobs.value.map((job) => [job.id, job] as const));
    for (const job of nextJobs) merged.set(job.id, job);
    options.jobs.value = Array.from(merged.values());
  }

  async function refreshActiveJobs() {
    const active = options.jobs.value.filter((job) => PORTAL_IMAGE_ACTIVE_STATUSES.has(job.status));
    if (active.length === 0) return;
    try {
      mergeJobs(await Promise.all(active.map((job) => options.api.getTask(job.id))));
    } catch {
      await options.fetchJobs();
    }
  }

  function startPolling() {
    if (pollTimer.value !== undefined) return;
    pollTimer.value = window.setInterval(() => void refreshActiveJobs(), intervalMs.value);
  }

  function stopPolling() {
    if (pollTimer.value === undefined) return;
    window.clearInterval(pollTimer.value);
    pollTimer.value = undefined;
  }

  watch(activeCount, (count) => {
    if (count > 0) startPolling();
    else stopPolling();
  }, { immediate: true });
  onUnmounted(stopPolling);

  return { mergeJob, startPolling };
}
