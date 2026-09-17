import { flushPromises, mount } from "@vue/test-utils";
import { createMemoryHistory, createRouter } from "vue-router";
import { beforeEach, afterEach, describe, expect, it, vi } from "vitest";
import RequestRecordsWorkspace from "./RequestRecordsWorkspace.vue";
import RecordRangeSelector from "./components/RecordRangeSelector.vue";

const state = vi.hoisted(() => ({ role: 1, list: vi.fn(), summary: vi.fn() }));
vi.mock("./recordsApi", async original => ({ ...await original<typeof import("./recordsApi")>(), recordsApi: state }));
vi.mock("@/stores/auth", () => ({ useAuthStore: () => ({ userInfo: { userType: state.role } }) }));
const control = { props: ["modelValue"], emits: ["update:modelValue"], template: '<input :value="modelValue" @input="$emit(\'update:modelValue\', $event.target.value)" />' };
async function render() {
  const router = createRouter({ history: createMemoryHistory(), routes: [{ path: "/", component: RequestRecordsWorkspace }] });
  await router.push("/");
  return mount(RequestRecordsWorkspace, { global: { plugins: [router], stubs: {
    PortalPagePanel: { template: "<main><slot /></main>" }, PortalMetricGrid: true, DsTabs: true, DsTable: true, DsDrawer: true,
    DsFilterBar: { template: '<div><slot /><slot name="actions" /></div>' },
    "el-input": control, "el-select": control, "el-option": true, "el-date-picker": true,
    "el-button": { template: '<button><slot /></button>' }
  } } });
}
describe("record search", () => {
  beforeEach(() => {
    sessionStorage.clear();
    vi.useFakeTimers(); vi.setSystemTime(new Date(2026, 8, 15, 14));
    state.role = 1; state.list.mockReset().mockResolvedValue({ records: [], next_cursor: "next" });
    state.summary.mockReset().mockResolvedValue({ requests: 0 });
  });
  afterEach(() => vi.useRealTimers());
  it.each([
    ["昨天", new Date(2026, 8, 14), new Date(2026, 8, 15)],
    ["本月", new Date(2026, 8, 1), new Date(2026, 8, 15, 14)],
    ["上月", new Date(2026, 7, 1), new Date(2026, 8, 1)],
    ["最近七天", new Date(2026, 8, 8, 14), new Date(2026, 8, 15, 14)],
    ["近30天", new Date(2026, 7, 16, 14), new Date(2026, 8, 15, 14)]
  ])("automatically queries %s and resets pagination", async (label, from, to) => {
    const wrapper = await render(); await flushPromises();
    expect(wrapper.findAll("button").some(button => button.text() === "近90天")).toBe(false);
    await wrapper.findAll("button").find(button => button.text() === "下一页")!.trigger("click"); await flushPromises();
    const calls = state.list.mock.calls.length;
    await wrapper.findAll("button").find(button => button.text() === label)!.trigger("click"); await flushPromises();
    expect(state.list).toHaveBeenCalledTimes(calls + 1);
    const window = { from: (from as Date).toISOString(), to: (to as Date).toISOString() };
    expect(state.list.mock.lastCall![1]).toMatchObject({ ...window, cursor: undefined });
    expect(state.summary.mock.lastCall![0]).toMatchObject(window);
    expect(wrapper.text()).toContain("第 1 页");
    wrapper.unmount();
  });
  it("waits for a valid custom window and automatically queries it", async () => {
    const wrapper = await render(); await flushPromises();
    const selector = wrapper.getComponent(RecordRangeSelector);
    await wrapper.findAll("button").find(button => button.text() === "自定义")!.trigger("click"); await flushPromises();
    expect(state.list).toHaveBeenCalledTimes(1);
    const from = new Date(2026, 8, 1, 10), to = new Date(2026, 8, 2, 11);
    selector.vm.$emit("update:customRange", [to, from]); await flushPromises();
    expect(state.list).toHaveBeenCalledTimes(1);
    selector.vm.$emit("update:customRange", [from, to]); await flushPromises();
    expect(state.list).toHaveBeenCalledTimes(2);
    expect(state.list.mock.lastCall![1]).toMatchObject({ from: from.toISOString(), to: to.toISOString() });
    expect(state.summary.mock.lastCall![0]).toMatchObject({ from: from.toISOString(), to: to.toISOString() });
    selector.vm.$emit("update:customRange", null); await flushPromises();
    expect(state.list).toHaveBeenCalledTimes(2);
    wrapper.unmount();
  });
  it("replaces a saved 90-day filter with 30 days", async () => {
    const wrapper = await render(); await flushPromises(); wrapper.unmount();
    const key = sessionStorage.key(0)!;
    const saved = JSON.parse(sessionStorage.getItem(key)!);
    sessionStorage.setItem(key, JSON.stringify({ ...saved, range: "90d", appliedRange: "90d", cursor: "old-page", history: [null] }));
    const restored = await render(); await flushPromises();
    expect(state.list.mock.lastCall![1]).toMatchObject({ from: new Date(2026, 7, 16, 14).toISOString(), cursor: undefined });
    expect(restored.findAll("button").find(button => button.text() === "近30天")!.attributes("aria-pressed")).toBe("true");
    restored.unmount();
  });
  it("applies all filters to list and summary and keeps pagination on the applied query", async () => {
    const wrapper = await render(); await flushPromises();
    await wrapper.get('[aria-label="模型 ID"]').setValue(" test-model ");
    await wrapper.get('[aria-label="租户名"]').setValue(" Acme ");
    await wrapper.get('[aria-label="用户名"]').setValue(" Alice ");
    await wrapper.get('[aria-label="分组"]').setValue(" premium ");
    await wrapper.get('[aria-label="API Key 名称"]').setValue(" production ");
    await wrapper.findAll("button").find(b => b.text() === "上月")!.trigger("click");
    await wrapper.findAll("button").find(b => b.text() === "查询")!.trigger("click"); await flushPromises();
    const query = state.list.mock.lastCall![1];
    expect(query).toMatchObject({ model: "test-model", tenant_name: "Acme", user_name: "Alice", group: "premium", api_key_name: "production", from: new Date(2026, 7, 1).toISOString(), to: new Date(2026, 8, 1).toISOString() });
    const { cursor: _cursor, limit: _limit, ...filters } = query;
    expect(state.summary.mock.lastCall![0]).toEqual(filters);
    await wrapper.get('[aria-label="模型 ID"]').setValue("unsubmitted");
    await wrapper.findAll("button").find(b => b.text() === "下一页")!.trigger("click"); await flushPromises();
    expect(state.list.mock.lastCall![1]).toMatchObject({ ...query, cursor: "next" });
    await wrapper.findAll("button").find(b => b.text() === "刷新")!.trigger("click"); await flushPromises();
    expect(state.list.mock.lastCall![1]).toMatchObject({ model: "test-model", cursor: undefined });
    await wrapper.findAll("button").find(b => b.text() === "重置")!.trigger("click"); await flushPromises();
    expect(state.list.mock.lastCall![1]).toMatchObject({ model: undefined, tenant_name: undefined, cursor: undefined, from: new Date(2026, 8, 15).toISOString() });
    wrapper.unmount();
  });
  it("restores the applied filters and cursor after returning from detail", async () => {
    const wrapper = await render(); await flushPromises();
    await wrapper.get('[aria-label="模型 ID"]').setValue("applied-model");
    await wrapper.findAll("button").find(b => b.text() === "查询")!.trigger("click"); await flushPromises();
    await wrapper.findAll("button").find(b => b.text() === "下一页")!.trigger("click"); await flushPromises();
    await wrapper.get('[aria-label="模型 ID"]').setValue("draft-model");
    wrapper.unmount();
    const restored = await render(); await flushPromises();
    expect(state.list.mock.lastCall![1]).toMatchObject({ model: "applied-model", cursor: "next" });
    expect((restored.get('[aria-label="模型 ID"]').element as HTMLInputElement).value).toBe("draft-model");
    expect(restored.text()).toContain("第 2 页");
    restored.unmount();
  });
  it.each([3, 4])("only offers applicable identity filters for role %i", async role => {
    state.role = role;
    const wrapper = await render(); await flushPromises();
    expect(wrapper.find('[aria-label="租户名"]').exists()).toBe(false);
    expect(wrapper.find('[aria-label="用户名"]').exists()).toBe(role === 3);
    expect(wrapper.find('[aria-label="分组"]').exists()).toBe(true);
    expect(wrapper.find('[aria-label="API Key 名称"]').exists()).toBe(true);
    wrapper.unmount();
  });
});
