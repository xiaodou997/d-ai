import { computed, onMounted, onUnmounted, reactive, shallowRef } from "vue";

import type { TenantUsageApi } from "../api";
import {
  defaultTenantUsageFilters,
  EMPTY_TENANT_USAGE_STATS,
  mapTenantUsageRows,
  type TenantUsageFilters,
  type TenantUsageLog,
  type TenantUsageRow,
  type TenantUsageStats,
  type TenantUsageUser
} from "../model";

interface UseTenantUsageOptions {
  api: TenantUsageApi;
  immediate?: boolean;
  onError: (message: string) => void;
}

export function useTenantUsage(options: UseTenantUsageOptions) {
  const filters = reactive<TenantUsageFilters>(defaultTenantUsageFilters());
  const page = shallowRef(1);
  const pageSize = shallowRef(20);
  const total = shallowRef(0);
  const loading = shallowRef(false);
  const usersLoading = shallowRef(false);
  const rows = shallowRef<TenantUsageRow[]>([]);
  const rawRecords = shallowRef<TenantUsageLog[]>([]);
  const users = shallowRef<TenantUsageUser[]>([]);
  const stats = shallowRef<TenantUsageStats>({ ...EMPTY_TENANT_USAGE_STATS });
  const selectedRecord = shallowRef<TenantUsageRow | null>(null);
  const detailOpen = shallowRef(false);

  let recordsGeneration = 0;
  let usersGeneration = 0;
  let recordsController: AbortController | undefined;
  let usersController: AbortController | undefined;
  let disposed = false;

  const successRate = computed(() => stats.value.total_requests
    ? `${((stats.value.success_count / stats.value.total_requests) * 100).toFixed(1)}%`
    : "-");

  function query() {
    return {
      limit: pageSize.value,
      offset: (page.value - 1) * pageSize.value,
      user_id: filters.userId || undefined,
      model_code: filters.modelCode || undefined,
      request_status: filters.requestStatus || undefined,
      request_source: filters.requestSource || undefined,
      date_from: filters.dateRange?.[0] ? new Date(filters.dateRange[0]).toISOString() : undefined,
      date_to: filters.dateRange?.[1] ? new Date(filters.dateRange[1]).toISOString() : undefined
    };
  }

  async function loadRecords() {
    recordsController?.abort();
    const controller = new AbortController();
    recordsController = controller;
    const generation = ++recordsGeneration;
    loading.value = true;
    try {
      const response = await options.api.listRecords(query(), controller.signal);
      if (disposed || controller.signal.aborted || generation !== recordsGeneration) return;
      rawRecords.value = response.records ?? [];
      rows.value = mapTenantUsageRows(rawRecords.value, users.value);
      stats.value = response.stats ?? { ...EMPTY_TENANT_USAGE_STATS };
      total.value = response.total ?? 0;
    } catch (error) {
      if (!isAbortError(error) && generation === recordsGeneration) options.onError(errorMessage(error, "加载消耗明细失败"));
    } finally {
      if (!disposed && !controller.signal.aborted && generation === recordsGeneration) loading.value = false;
    }
  }

  async function loadUsers() {
    usersController?.abort();
    const controller = new AbortController();
    usersController = controller;
    const generation = ++usersGeneration;
    usersLoading.value = true;
    try {
      const response = await options.api.listUsers(controller.signal);
      if (disposed || controller.signal.aborted || generation !== usersGeneration) return;
      users.value = response.items ?? [];
      rows.value = mapTenantUsageRows(rawRecords.value, users.value);
    } catch (error) {
      if (!isAbortError(error) && generation === usersGeneration) {
        users.value = [];
        rows.value = mapTenantUsageRows(rawRecords.value, []);
      }
    } finally {
      if (!disposed && !controller.signal.aborted && generation === usersGeneration) usersLoading.value = false;
    }
  }

  async function search() {
    page.value = 1;
    await loadRecords();
  }

  async function reset() {
    Object.assign(filters, defaultTenantUsageFilters());
    page.value = 1;
    await loadRecords();
  }

  async function changePage(value: number) {
    page.value = value;
    await loadRecords();
  }

  async function changePageSize(value: number) {
    pageSize.value = value;
    page.value = 1;
    await loadRecords();
  }

  function openDetail(row: TenantUsageRow) {
    selectedRecord.value = row;
    detailOpen.value = true;
  }

  onMounted(() => {
    if (options.immediate !== false) void Promise.allSettled([loadUsers(), loadRecords()]);
  });

  onUnmounted(() => {
    disposed = true;
    recordsGeneration++;
    usersGeneration++;
    recordsController?.abort();
    usersController?.abort();
  });

  return {
    changePage,
    changePageSize,
    detailOpen,
    filters,
    loadRecords,
    loadUsers,
    loading,
    openDetail,
    page,
    pageSize,
    reset,
    rows,
    search,
    selectedRecord,
    stats,
    successRate,
    total,
    users,
    usersLoading
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
