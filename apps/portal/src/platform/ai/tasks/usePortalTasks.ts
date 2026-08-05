import { computed, onMounted, onUnmounted, readonly, shallowRef } from "vue";

import type { PortalTaskApi, PortalTaskQuery, PortalTaskRecord } from "./types";

const activeStatuses = new Set(["pending", "running"]);

export function usePortalTasks(options: {
  api: PortalTaskApi;
  initialQuery?: PortalTaskQuery;
  pollIntervalMs?: number;
  notifyError?: (message: string) => void;
}) {
  const tasks = shallowRef<PortalTaskRecord[]>([]);
  const loading = shallowRef(false);
  const loadingMore = shallowRef(false);
  const hasMore = shallowRef(false);
  const query = shallowRef<PortalTaskQuery>({ limit: 20, ...options.initialQuery });
  const operationTaskID = shallowRef("");
  const requestVersion = shallowRef(0);
  let pollTimer: number | undefined;

  const activeCount = computed(() => tasks.value.filter((task) => activeStatuses.has(task.status)).length);
  const pollIntervalMs = Number.isFinite(options.pollIntervalMs) && (options.pollIntervalMs || 0) > 0
    ? Number(options.pollIntervalMs)
    : 20_000;

  async function refresh(nextQuery: PortalTaskQuery = query.value): Promise<void> {
    const version = requestVersion.value + 1;
    requestVersion.value = version;
    query.value = { limit: 20, ...nextQuery, starting_after: undefined };
    loading.value = true;
    try {
      const page = await options.api.listTasks(query.value);
      if (version !== requestVersion.value) return;
      tasks.value = page.items;
      hasMore.value = page.has_more;
    } catch (error) {
      if (version === requestVersion.value) {
        options.notifyError?.((error as Error).message || "任务列表加载失败");
      }
    } finally {
      if (version === requestVersion.value) loading.value = false;
    }
  }

  async function loadMore(): Promise<void> {
    if (!hasMore.value || loadingMore.value || tasks.value.length === 0) return;
    loadingMore.value = true;
    try {
      const page = await options.api.listTasks({
        ...query.value,
        starting_after: tasks.value[tasks.value.length - 1]?.id
      });
      const merged = new Map(tasks.value.map((task) => [task.id, task] as const));
      page.items.forEach((task) => merged.set(task.id, task));
      tasks.value = Array.from(merged.values());
      hasMore.value = page.has_more;
    } catch (error) {
      options.notifyError?.((error as Error).message || "更多任务加载失败");
    } finally {
      loadingMore.value = false;
    }
  }

  function replaceTask(next: PortalTaskRecord): void {
    tasks.value = tasks.value.map((task) => (task.id === next.id ? next : task));
  }

  async function refreshActive(): Promise<void> {
    const active = tasks.value.filter((task) => activeStatuses.has(task.status));
    if (active.length === 0) return;
    const settled = await Promise.allSettled(active.map((task) => options.api.getTask(task.id)));
    settled.forEach((result) => {
      if (result.status === "fulfilled") replaceTask(result.value);
    });
  }

  async function getTask(taskId: string): Promise<PortalTaskRecord> {
    const task = await options.api.getTask(taskId);
    replaceTask(task);
    return task;
  }

  async function cancelTask(task: PortalTaskRecord): Promise<PortalTaskRecord | undefined> {
    if (!task.permissions.can_cancel || operationTaskID.value) return undefined;
    operationTaskID.value = task.id;
    try {
      const updated = await options.api.cancelTask(task.id);
      replaceTask(updated);
      return updated;
    } catch (error) {
      options.notifyError?.((error as Error).message || "取消任务失败");
      return undefined;
    } finally {
      operationTaskID.value = "";
    }
  }

  async function deleteTask(task: PortalTaskRecord): Promise<boolean> {
    if (!task.permissions.can_delete || operationTaskID.value) return false;
    operationTaskID.value = task.id;
    try {
      await options.api.deleteTask(task.id);
      tasks.value = tasks.value.filter((item) => item.id !== task.id);
      return true;
    } catch (error) {
      options.notifyError?.((error as Error).message || "删除任务失败");
      return false;
    } finally {
      operationTaskID.value = "";
    }
  }

  onMounted(() => {
    void refresh();
    pollTimer = window.setInterval(() => void refreshActive(), pollIntervalMs);
  });
  onUnmounted(() => {
    if (pollTimer !== undefined) window.clearInterval(pollTimer);
  });

  return {
    tasks: readonly(tasks),
    loading: readonly(loading),
    loadingMore: readonly(loadingMore),
    hasMore: readonly(hasMore),
    activeCount,
    operationTaskID: readonly(operationTaskID),
    refresh,
    loadMore,
    getTask,
    cancelTask,
    deleteTask
  };
}
