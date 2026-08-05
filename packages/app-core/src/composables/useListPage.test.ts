import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { useListPage, type UseListPageOptions } from "./useListPage";

interface Query {
  keyword: string;
  status: string | null;
}

interface Row {
  id: number;
  name: string;
}

interface Page {
  items: Row[];
  total: number;
}

type Fetcher = UseListPageOptions<Query, Row>["fetcher"];

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

async function flush() {
  await Promise.resolve();
  await Promise.resolve();
}

function makeFetcher() {
  const calls: Array<Parameters<Fetcher>[0]> = [];
  const pending: Array<ReturnType<typeof deferred<Page>>> = [];
  const fetcher = vi.fn<Fetcher>((params) => {
    calls.push(params);
    const d = deferred<Page>();
    pending.push(d);
    return d.promise;
  });
  return { fetcher, calls, pending };
}

const initialQuery: Query = { keyword: "", status: null };

const page = (n: number, total = 40): Page => ({
  items: [{ id: n, name: `row-${n}` }],
  total
});

describe("useListPage", () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("loads immediately on setup and populates rows/total", async () => {
    const { fetcher, calls, pending } = makeFetcher();
    const list = useListPage<Query, Row>({ initialQuery, fetcher });

    expect(calls).toEqual([{ keyword: "", status: null, page: 1, pageSize: 10 }]);
    expect(list.loading.value).toBe(true);

    pending[0].resolve(page(1, 23));
    await flush();

    expect(list.rows.value).toEqual([{ id: 1, name: "row-1" }]);
    expect(list.total.value).toBe(23);
    expect(list.loading.value).toBe(false);
    expect(list.error.value).toBeNull();
  });

  it("does not load on setup when immediate is false", () => {
    const { fetcher } = makeFetcher();
    useListPage<Query, Row>({ initialQuery, fetcher, immediate: false });
    expect(fetcher).not.toHaveBeenCalled();
  });

  it("search resets to page 1 and refetches with current query", async () => {
    const { fetcher, calls, pending } = makeFetcher();
    const list = useListPage<Query, Row>({ initialQuery, fetcher });
    pending[0].resolve(page(1));
    await flush();

    list.handlePageChange(3);
    pending[1].resolve(page(3));
    await flush();
    expect(list.page.value).toBe(3);

    list.query.keyword = "abc";
    void list.search();
    expect(list.page.value).toBe(1);
    expect(calls[2]).toEqual({ keyword: "abc", status: null, page: 1, pageSize: 10 });
    pending[2].resolve(page(9));
    await flush();
    expect(list.rows.value).toEqual([{ id: 9, name: "row-9" }]);
  });

  it("handlePageSizeChange resets page and passes the new size", async () => {
    const { fetcher, calls, pending } = makeFetcher();
    const list = useListPage<Query, Row>({ initialQuery, fetcher });
    pending[0].resolve(page(1));
    await flush();

    list.handlePageChange(4);
    pending[1].resolve(page(4));
    await flush();

    list.handlePageSizeChange(50);
    expect(list.pageSize.value).toBe(50);
    expect(list.page.value).toBe(1);
    expect(calls[2]).toEqual({ keyword: "", status: null, page: 1, pageSize: 50 });
  });

  it("handlePageChange with the current page does not refetch (DsPagination size-change flow)", async () => {
    const { fetcher, calls, pending } = makeFetcher();
    const list = useListPage<Query, Row>({ initialQuery, fetcher });
    pending[0].resolve(page(1));
    await flush();

    list.handlePageChange(1);
    expect(calls).toHaveLength(1);

    // DsPagination 页大小切换会连续抛出 update:pageSize + update:page(1),只应触发一次请求
    list.handlePageSizeChange(50);
    list.handlePageChange(1);
    expect(calls).toHaveLength(2);
  });

  it("resetQuery restores initial filters, resets page, and reloads", async () => {
    const { fetcher, calls, pending } = makeFetcher();
    const list = useListPage<Query, Row>({ initialQuery, fetcher });
    pending[0].resolve(page(1));
    await flush();

    list.query.keyword = "abc";
    list.query.status = "active";
    list.handlePageChange(2);
    pending[1].resolve(page(2));
    await flush();

    void list.resetQuery();
    expect(list.query.keyword).toBe("");
    expect(list.query.status).toBeNull();
    expect(list.page.value).toBe(1);
    expect(calls[2]).toEqual({ keyword: "", status: null, page: 1, pageSize: 10 });
    // the caller's initialQuery object must not have been mutated
    expect(initialQuery).toEqual({ keyword: "", status: null });
  });

  it("ignores a stale response resolving after a newer one", async () => {
    const { fetcher, pending } = makeFetcher();
    const list = useListPage<Query, Row>({ initialQuery, fetcher });

    void list.search(); // second request while the first is still in flight
    pending[1].resolve(page(2, 100)); // newer resolves first
    await flush();
    expect(list.rows.value).toEqual([{ id: 2, name: "row-2" }]);
    expect(list.total.value).toBe(100);

    pending[0].resolve(page(1, 1)); // stale response arrives late
    await flush();
    expect(list.rows.value).toEqual([{ id: 2, name: "row-2" }]);
    expect(list.total.value).toBe(100);
    expect(list.loading.value).toBe(false);
  });

  it("sets error from Error.message and clears it on the next success", async () => {
    const { fetcher, pending } = makeFetcher();
    const list = useListPage<Query, Row>({ initialQuery, fetcher });

    pending[0].reject(new Error("boom"));
    await flush();
    expect(list.error.value).toBe("boom");
    expect(list.loading.value).toBe(false);
    expect(list.rows.value).toEqual([]);

    void list.refresh();
    expect(list.error.value).toBeNull();
    pending[1].resolve(page(7, 5));
    await flush();
    expect(list.error.value).toBeNull();
    expect(list.rows.value).toEqual([{ id: 7, name: "row-7" }]);
  });
});
