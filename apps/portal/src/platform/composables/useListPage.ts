import { reactive, ref, type Ref } from "vue";

export interface UseListPageOptions<TQuery extends object, TRow> {
  /** Filter values; deep-cloned internally so later mutation does not affect reset. */
  initialQuery: TQuery;
  fetcher: (params: TQuery & { page: number; pageSize: number }) => Promise<{ items: TRow[]; total: number }>;
  /** Default 10. */
  pageSize?: number;
  /** Default true → load() on setup. */
  immediate?: boolean;
}

export interface UseListPageReturn<TQuery extends object, TRow> {
  rows: Ref<TRow[]>;
  total: Ref<number>;
  loading: Ref<boolean>;
  error: Ref<string | null>;
  page: Ref<number>;
  pageSize: Ref<number>;
  query: TQuery;
  load(): Promise<void>;
  refresh(): Promise<void>;
  search(): Promise<void>;
  resetQuery(): Promise<void>;
  handlePageChange(p: number): void;
  handlePageSizeChange(s: number): void;
}

/**
 * Standard wiring for list pages: query filters + pagination + fetch state.
 * Overlapping load() calls are guarded by a sequence counter — only the
 * latest request may mutate rows/total/error/loading.
 */
export function useListPage<TQuery extends object, TRow>(
  options: UseListPageOptions<TQuery, TRow>
): UseListPageReturn<TQuery, TRow> {
  const { fetcher, immediate = true } = options;

  const initialQuery = structuredClone(options.initialQuery);
  const query = reactive(structuredClone(options.initialQuery)) as TQuery;

  const rows = ref<TRow[]>([]) as Ref<TRow[]>;
  const total = ref(0);
  const loading = ref(false);
  const error = ref<string | null>(null);
  const page = ref(1);
  const pageSize = ref(options.pageSize ?? 10);

  let seq = 0;

  async function load(): Promise<void> {
    const current = ++seq;
    loading.value = true;
    error.value = null;
    const params = {
      ...(query as Record<string, unknown>),
      page: page.value,
      pageSize: pageSize.value
    } as TQuery & { page: number; pageSize: number };

    try {
      const result = await fetcher(params);
      if (current !== seq) return; // stale response, a newer request is in flight
      rows.value = result.items;
      total.value = result.total;
      error.value = null;
    } catch (e) {
      if (current !== seq) return;
      error.value = e instanceof Error ? e.message : String(e);
    } finally {
      if (current === seq) loading.value = false;
    }
  }

  function refresh(): Promise<void> {
    return load();
  }

  function search(): Promise<void> {
    page.value = 1;
    return load();
  }

  function resetQuery(): Promise<void> {
    const fresh = structuredClone(initialQuery) as Record<string, unknown>;
    const target = query as Record<string, unknown>;
    for (const key of Object.keys(target)) {
      if (!(key in fresh)) delete target[key];
    }
    Object.assign(target, fresh);
    page.value = 1;
    return load();
  }

  function handlePageChange(p: number): void {
    // DsPagination 在切换页大小时会连续抛出 update:pageSize 和 update:page(1),
    // handlePageSizeChange 已把 page 置 1 并触发 load,此处跳过重复请求。
    if (p === page.value) return;
    page.value = p;
    void load();
  }

  function handlePageSizeChange(s: number): void {
    pageSize.value = s;
    page.value = 1;
    void load();
  }

  if (immediate) void load();

  return {
    rows,
    total,
    loading,
    error,
    page,
    pageSize,
    query,
    load,
    refresh,
    search,
    resetQuery,
    handlePageChange,
    handlePageSizeChange
  };
}
