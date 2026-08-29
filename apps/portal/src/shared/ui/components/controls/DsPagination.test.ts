import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";

import DsPagination from "./DsPagination.vue";

function mountPagination(props: Record<string, unknown> = {}) {
  return mount(DsPagination, {
    props: { page: 1, pageSize: 10, total: 95, ...props }
  });
}

function pageButtons(wrapper: ReturnType<typeof mountPagination>) {
  return wrapper.findAll(".ds-pagination__btn:not(.ds-pagination__btn--icon)");
}

describe("DsPagination", () => {
  it("renders total info text", () => {
    const wrapper = mountPagination();
    expect(wrapper.find(".ds-pagination__info").text()).toBe("共 95 条");
  });

  it("computes page window with ellipsis for 95 items at 10 per page", () => {
    const wrapper = mountPagination({ page: 5 });

    // 10 total pages: 1 … 3 4 5 6 7 … 10
    expect(pageButtons(wrapper).map((btn) => btn.text())).toEqual([
      "1",
      "3",
      "4",
      "5",
      "6",
      "7",
      "10"
    ]);
    expect(wrapper.findAll(".ds-pagination__ellipsis")).toHaveLength(2);
  });

  it("renders all pages without ellipsis when few pages", () => {
    const wrapper = mountPagination({ total: 30, page: 2 });

    expect(pageButtons(wrapper).map((btn) => btn.text())).toEqual(["1", "2", "3"]);
    expect(wrapper.findAll(".ds-pagination__ellipsis")).toHaveLength(0);
  });

  it("marks the current page as active", () => {
    const wrapper = mountPagination({ page: 3 });
    const active = wrapper.find(".ds-pagination__btn--active");
    expect(active.exists()).toBe(true);
    expect(active.text()).toBe("3");
    expect(active.attributes("aria-current")).toBe("page");
    expect(wrapper.find("nav").attributes("aria-label")).toBe("分页");
    expect(wrapper.find("select").attributes("aria-label")).toBe("每页条数");
  });

  it("emits update:page when navigating", async () => {
    const wrapper = mountPagination({ page: 1 });

    const next = wrapper.find('button[title="下一页"]');
    await next.trigger("click");
    expect(wrapper.emitted("update:page")?.[0]).toEqual([2]);

    const last = wrapper.find('button[title="最后一页"]');
    await last.trigger("click");
    expect(wrapper.emitted("update:page")?.[1]).toEqual([10]);
  });

  it("does not emit when navigating out of bounds or to the current page", async () => {
    const wrapper = mountPagination({ page: 1 });

    const prev = wrapper.find('button[title="上一页"]');
    expect((prev.element as HTMLButtonElement).disabled).toBe(true);
    await prev.trigger("click");

    const first = wrapper.find('button[title="第一页"]');
    expect((first.element as HTMLButtonElement).disabled).toBe(true);
    await first.trigger("click");

    // clicking the active page button is a no-op
    const active = wrapper.find(".ds-pagination__btn--active");
    await active.trigger("click");

    expect(wrapper.emitted("update:page")).toBeUndefined();
  });

  it("disables next/last on the final page", () => {
    const wrapper = mountPagination({ page: 10 });

    expect((wrapper.find('button[title="下一页"]').element as HTMLButtonElement).disabled).toBe(true);
    expect((wrapper.find('button[title="最后一页"]').element as HTMLButtonElement).disabled).toBe(true);
    expect((wrapper.find('button[title="上一页"]').element as HTMLButtonElement).disabled).toBe(false);
  });

  it("emits update:pageSize and resets to page 1 when page size changes", async () => {
    const wrapper = mountPagination({ page: 4 });

    const select = wrapper.find("select");
    await select.setValue("20");

    expect(wrapper.emitted("update:pageSize")?.[0]).toEqual([20]);
    expect(wrapper.emitted("update:page")?.[0]).toEqual([1]);
  });

  it("does not emit when selecting the current page size", async () => {
    const wrapper = mountPagination();

    await wrapper.find("select").setValue("10");

    expect(wrapper.emitted("update:pageSize")).toBeUndefined();
    expect(wrapper.emitted("update:page")).toBeUndefined();
  });
});
