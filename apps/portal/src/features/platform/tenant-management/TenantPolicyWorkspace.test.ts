import { flushPromises, shallowMount } from "@vue/test-utils";
import ElementPlus from "element-plus";
import { createMemoryHistory, createRouter } from "vue-router";
import { beforeEach, describe, expect, it, vi } from "vitest";

import TenantPolicyWorkspace from "./TenantPolicyWorkspace.vue";

const getTenant = vi.hoisted(() => vi.fn());

vi.mock("@/api/platformAdmin", () => ({
  platformAdminApi: { getTenant }
}));

describe("TenantPolicyWorkspace", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    getTenant.mockResolvedValue({
      tenantId: "tenant-1",
      tenantName: "Acme",
      status: 1,
      statusDisplay: "启用"
    });
  });

  it("loads the tenant policy subject from the route id", async () => {
    const { wrapper } = await mountWorkspace();

    expect(getTenant).toHaveBeenCalledWith("tenant-1");
    expect(wrapper.get("[data-testid='policy-tenant']").text()).toBe("Acme");
    wrapper.unmount();
  });

  it("keeps policy controls mounted inside the tenant-management feature", async () => {
    const { wrapper } = await mountWorkspace();

    expect(wrapper.find("[data-testid='policy-panel']").exists()).toBe(true);
    expect(wrapper.text()).toContain("Acme");
    wrapper.unmount();
  });
});

async function mountWorkspace() {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: "/admin/organization/tenants/:id/policy", component: TenantPolicyWorkspace }]
  });
  await router.push("/admin/organization/tenants/tenant-1/policy");
  await router.isReady();

  const wrapper = shallowMount(TenantPolicyWorkspace, {
    global: {
      plugins: [router, ElementPlus],
      stubs: {
        PortalPagePanel: { template: "<main><slot name='actions' /><slot /></main>" },
        DsTag: { template: "<span><slot /></span>" },
        AdminTenantPolicyPanel: {
          props: ["tenant"],
          template: "<section data-testid='policy-panel'><span data-testid='policy-tenant'>{{ tenant?.tenantName || 'none' }}</span></section>"
        }
      }
    }
  });
  await flushPromises();
  return { router, wrapper };
}
