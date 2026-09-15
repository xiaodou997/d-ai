import { flushPromises, mount } from "@vue/test-utils";
import { createMemoryHistory, createRouter } from "vue-router";
import { beforeEach, afterEach, describe, expect, it, vi } from "vitest";
import RequestRecordsWorkspace from "./RequestRecordsWorkspace.vue";

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
    vi.useFakeTimers(); vi.setSystemTime(new Date(2026, 8, 15, 14));
    state.role = 1; state.list.mockReset().mockResolvedValue({ records: [], next_cursor: "next" });
    state.summary.mockReset().mockResolvedValue({ requests: 0 });
  });
  afterEach(() => vi.useRealTimers());
  it("applies all filters to list and summary and keeps pagination on the applied query", async () => {
    const wrapper = await render(); await flushPromises();
    await wrapper.get('[aria-label="模型 ID"]').setValue(" test-model ");
    await wrapper.get('[aria-label="租户名"]').setValue(" Acme ");
    await wrapper.get('[aria-label="用户名"]').setValue(" Alice ");
    await wrapper.get('[aria-label="分组"]').setValue(" premium ");
    await wrapper.get('[aria-label="API Key 名称"]').setValue(" production ");
    await wrapper.get('[aria-label="时间范围"]').setValue("lastMonth");
    await wrapper.findAll("button").find(b => b.text() === "查询")!.trigger("click"); await flushPromises();
    const query = state.list.mock.lastCall![1];
    expect(query).toMatchObject({ model: "test-model", tenant_name: "Acme", user_name: "Alice", group: "premium", api_key_name: "production", from: new Date(2026, 7, 1).toISOString(), to: new Date(2026, 8, 1).toISOString() });
    const { cursor: _cursor, ...filters } = query;
    expect(state.summary.mock.lastCall![0]).toEqual(filters);
    await wrapper.get('[aria-label="模型 ID"]').setValue("unsubmitted");
    await wrapper.findAll("button").find(b => b.text() === "下一页")!.trigger("click"); await flushPromises();
    expect(state.list.mock.lastCall![1]).toMatchObject({ ...query, cursor: "next" });
    await wrapper.findAll("button").find(b => b.text() === "重置")!.trigger("click"); await flushPromises();
    expect(state.list.mock.lastCall![1]).toMatchObject({ model: undefined, tenant_name: undefined, cursor: undefined, from: new Date(2026, 8, 15).toISOString() });
    wrapper.unmount();
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
