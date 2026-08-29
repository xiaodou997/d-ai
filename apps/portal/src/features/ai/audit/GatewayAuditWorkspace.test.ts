import { flushPromises, shallowMount } from "@vue/test-utils";
import ElementPlus from "element-plus";
import { beforeEach, describe, expect, it, vi } from "vitest";

import GatewayAuditWorkspace from "./GatewayAuditWorkspace.vue";

const api = vi.hoisted(() => ({ listGatewayAuditLogs: vi.fn() }));

vi.mock("@/api/aiAdmin", () => ({ aiAdminApi: api }));

describe("GatewayAuditWorkspace", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    api.listGatewayAuditLogs.mockResolvedValue({
      items: [{
        id: "audit-1",
        actor: "admin-1",
        action: "update_account",
        object_type: "upstream_account",
        object_id: "account-1",
        request_summary: { status: "active" },
        result: "success",
        http_status: 200,
        created_at: 1_700_000_000_000
      }],
      total: 1
    });
  });

  it("loads typed audit rows with the declared limit", async () => {
    const wrapper = await mountWorkspace();

    expect(api.listGatewayAuditLogs).toHaveBeenCalledWith({ limit: 100 });
    expect(wrapper.get("[data-testid='audit-rows']").text()).toBe("1");
    wrapper.unmount();
  });

  it("refreshes the audit list without sending unsupported filters", async () => {
    const wrapper = await mountWorkspace();
    const refreshButton = wrapper.findAll("button").find((button) => button.text().includes("刷新"));
    expect(refreshButton).toBeDefined();

    await refreshButton!.trigger("click");
    await flushPromises();

    expect(api.listGatewayAuditLogs).toHaveBeenLastCalledWith({ limit: 100 });
    expect(api.listGatewayAuditLogs).toHaveBeenCalledTimes(2);
    wrapper.unmount();
  });
});

async function mountWorkspace() {
  const wrapper = shallowMount(GatewayAuditWorkspace, {
    global: {
      plugins: [ElementPlus],
      stubs: {
        PortalPagePanel: { template: "<main><slot name='actions' /><slot name='filters' /><slot /></main>" },
        DsTable: { props: ["rows"], template: "<div data-testid='audit-rows'>{{ rows.length }}</div>" },
        "el-button": { inheritAttrs: false, template: "<button data-testid='refresh-button' v-bind='$attrs'><slot /></button>" }
      }
    }
  });
  await flushPromises();
  return wrapper;
}
