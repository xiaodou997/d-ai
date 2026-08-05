import { computed, onMounted, onUnmounted, reactive, shallowRef } from "vue";

import type { CustomerUsageApi } from "../api";
import {
  filterCustomerUsageRows,
  summarizeCustomerUsage,
  type CustomerUsageFilters,
  type CustomerUsageLog
} from "../model";

interface UseCustomerUsageOptions {
  api: CustomerUsageApi;
  immediate?: boolean;
  onError: (message: string) => void;
}

export function useCustomerUsage(options: UseCustomerUsageOptions) {
  const loading = shallowRef(false);
  const records = shallowRef<CustomerUsageLog[]>([]);
  const selectedRecord = shallowRef<CustomerUsageLog | null>(null);
  const detailOpen = shallowRef(false);
  const page = shallowRef(1);
  const pageSize = shallowRef(20);
  const filters = reactive<CustomerUsageFilters>({ requestSource: "", requestStatus: "", keyword: "", limit: 100 });

  let generation = 0;
  let controller: AbortController | undefined;
  let disposed = false;

  const filteredRecords = computed(() => filterCustomerUsageRows(records.value, filters));
  const pagedRecords = computed(() => {
    const start = (page.value - 1) * pageSize.value;
    return filteredRecords.value.slice(start, start + pageSize.value);
  });
  const stats = computed(() => summarizeCustomerUsage(filteredRecords.value));
  const successRate = computed(() => stats.value.totalRequests
    ? `${((stats.value.successRequests / stats.value.totalRequests) * 100).toFixed(1)}%`
    : "-");

  async function loadRecords() {
    controller?.abort();
    controller = new AbortController();
    const requestController = controller;
    const requestGeneration = ++generation;
    loading.value = true;
    try {
      const response = await options.api.listRecords({
        limit: filters.limit,
        request_source: filters.requestSource || undefined
      }, requestController.signal);
      if (disposed || requestController.signal.aborted || requestGeneration !== generation) return;
      records.value = response.items ?? [];
      page.value = 1;
    } catch (error) {
      if (!isAbortError(error) && requestGeneration === generation) options.onError(errorMessage(error, "加载使用记录失败"));
    } finally {
      if (!disposed && !requestController.signal.aborted && requestGeneration === generation) loading.value = false;
    }
  }

  function search() {
    page.value = 1;
  }

  async function reset() {
    Object.assign(filters, { requestSource: "", requestStatus: "", keyword: "", limit: 100 });
    pageSize.value = 20;
    page.value = 1;
    await loadRecords();
  }

  async function changeServerFilter() {
    await loadRecords();
  }

  function openDetail(row: CustomerUsageLog) {
    selectedRecord.value = row;
    detailOpen.value = true;
  }

  onMounted(() => {
    if (options.immediate !== false) void loadRecords();
  });

  onUnmounted(() => {
    disposed = true;
    generation++;
    controller?.abort();
  });

  return {
    changeServerFilter,
    detailOpen,
    filteredRecords,
    filters,
    loadRecords,
    loading,
    openDetail,
    page,
    pageSize,
    pagedRecords,
    records,
    reset,
    search,
    selectedRecord,
    stats,
    successRate
  };
}

function isAbortError(error: unknown) {
  return error instanceof DOMException
    ? error.name === "AbortError"
    : Boolean(error && typeof error === "object" && "name" in error && error.name === "AbortError");
}

function errorMessage(error: unknown, fallback: string) {
  return error instanceof Error && error.message ? error.message : fallback;
}
