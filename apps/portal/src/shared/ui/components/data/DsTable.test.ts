import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";

import DsTable from "./DsTable.vue";

import type { DsTableColumn } from "./types";

const columns: DsTableColumn[] = [
  { key: "name", title: "名称" },
  { key: "status", title: "状态", align: "center" }
];

const rows = [
  { id: 1, name: "Alpha", status: "启用" },
  { id: 2, name: "Beta", status: "停用" }
];

function mountTable(props: Record<string, unknown> = {}, options: Record<string, unknown> = {}) {
  return mount(DsTable, {
    props: { columns, rows, rowKey: "id", ...props },
    ...options
  });
}

describe("DsTable", () => {
  it("renders column headers and rows", () => {
    const wrapper = mountTable();

    const headers = wrapper.findAll("thead th");
    expect(headers.map((th) => th.text())).toEqual(["名称", "状态"]);
    expect(headers.every((th) => th.attributes("scope") === "col")).toBe(true);
    expect(wrapper.find("table").attributes("aria-label")).toBe("数据表格");

    const bodyRows = wrapper.findAll("tbody tr.ds-table__row");
    expect(bodyRows).toHaveLength(2);
    expect(bodyRows[0].text()).toContain("Alpha");
    expect(bodyRows[1].text()).toContain("Beta");
  });

  it("renders default cell value and supports cell slot override", () => {
    const wrapper = mountTable(
      {},
      {
        slots: {
          "cell-status": `<template #cell-status="{ value }"><b class="status-override">{{ value }}</b></template>`
        }
      }
    );

    const overrides = wrapper.findAll(".status-override");
    expect(overrides).toHaveLength(2);
    expect(overrides[0].text()).toBe("启用");
  });

  it("keeps operational cells single-line unless a column opts into wrapping", () => {
    const wrapper = mountTable({
      columns: [columns[0], { ...columns[1], wrap: true }]
    });

    expect(wrapper.findAll("tbody td")[0].classes()).not.toContain("ds-table__cell--wrap");
    expect(wrapper.findAll("tbody td")[1].classes()).toContain("ds-table__cell--wrap");
  });

  it("emits update:selection when a row checkbox is toggled", async () => {
    const wrapper = mountTable({ selectable: true, selection: [] });

    const rowCheckboxes = wrapper.findAll('tbody input[type="checkbox"]');
    expect(rowCheckboxes).toHaveLength(2);
    expect(wrapper.find('thead input[type="checkbox"]').attributes("aria-label")).toBe("全选当前页");
    expect(rowCheckboxes[0].attributes("aria-label")).toBe("选择1");

    await rowCheckboxes[0].setValue(true);
    expect(wrapper.emitted("update:selection")?.[0]).toEqual([[rows[0]]]);
  });

  it("select-all toggles all current page rows", async () => {
    const wrapper = mountTable({ selectable: true, selection: [] });
    const headerCheckbox = wrapper.find('thead input[type="checkbox"]');

    await headerCheckbox.setValue(true);
    expect(wrapper.emitted("update:selection")?.[0]).toEqual([[rows[0], rows[1]]]);

    await wrapper.setProps({ selection: [rows[0], rows[1]] });
    await headerCheckbox.setValue(false);
    expect(wrapper.emitted("update:selection")?.[1]).toEqual([[]]);
  });

  it("shows empty state when there are no rows", () => {
    const wrapper = mountTable({ rows: [] });

    expect(wrapper.find(".ds-table__empty").exists()).toBe(true);
    expect(wrapper.find(".ds-table__empty-title").text()).toBe("暂无数据");
    expect(wrapper.findAll("tbody tr")).toHaveLength(0);
  });

  it("supports custom empty slot", () => {
    const wrapper = mountTable(
      { rows: [] },
      { slots: { empty: '<div class="custom-empty">Nothing here</div>' } }
    );

    expect(wrapper.find(".custom-empty").exists()).toBe(true);
    expect(wrapper.find(".ds-table__empty-title").exists()).toBe(false);
  });

  it("shows skeleton rows while loading instead of data rows", () => {
    const wrapper = mountTable({ loading: true });

    expect(wrapper.findAll("tr.ds-table__row--skeleton")).toHaveLength(6);
    expect(wrapper.findAll(".ds-table__skeleton").length).toBeGreaterThan(0);
    expect(wrapper.text()).not.toContain("Alpha");
    expect(wrapper.find(".ds-table__empty").exists()).toBe(false);
  });

  it("toggles expand slot content when expandable", async () => {
    const wrapper = mountTable(
      { expandable: true },
      {
        slots: {
          expand: `<template #expand="{ row }"><div class="expand-content">{{ row.name }}</div></template>`
        }
      }
    );

    expect(wrapper.findAll(".expand-content")).toHaveLength(0);

    const toggles = wrapper.findAll(".ds-table__expand-toggle");
    await toggles[0].trigger("click");

    const expanded = wrapper.findAll(".expand-content");
    expect(expanded).toHaveLength(1);
    expect(expanded[0].text()).toBe("Alpha");
  });
});
