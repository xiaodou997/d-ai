import { flushPromises, shallowMount } from "@vue/test-utils";
import { createMemoryHistory, createRouter } from "vue-router";
import { beforeEach, describe, expect, it, vi } from "vitest";

import UsageDetailWorkspace from "./UsageDetailWorkspace.vue";

const getDetail = vi.hoisted(() => vi.fn());

vi.mock("@/features/ai/usage/api", () => ({
  adminUsageApi: { getDetail }
}));

describe("UsageDetailWorkspace", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    getDetail.mockResolvedValue({ request_id: "request-1", model_code: "gpt-4o" });
  });

  it("loads the request detail selected by the route", async () => {
    const wrapper = await mountWorkspace("request-1");

    expect(getDetail).toHaveBeenCalledWith("request-1");
    expect(wrapper.get("[data-testid='detail-request']").text()).toBe("request-1");
    wrapper.unmount();
  });

  it("renders the feature-owned empty state when detail loading fails", async () => {
    getDetail.mockRejectedValueOnce(new Error("missing"));
    const wrapper = await mountWorkspace("missing");

    expect(wrapper.get("[data-testid='detail-empty']").text()).toContain("未找到该请求的详情");
    wrapper.unmount();
  });
});

async function mountWorkspace(requestId: string) {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: "/admin/ai/usage/:requestId", component: UsageDetailWorkspace },
      { path: "/admin/ai/usage", name: "ai-usage", component: { template: "<div />" } }
    ]
  });
  await router.push(`/admin/ai/usage/${requestId}`);
  await router.isReady();

  const wrapper = shallowMount(UsageDetailWorkspace, {
    global: {
      plugins: [router],
      stubs: {
        PortalPagePanel: { template: "<main><slot /></main>" },
        UsageDetailContent: {
          props: ["detail"],
          template: "<div data-testid='detail-request'>{{ detail?.request_id || 'loading' }}</div>"
        },
        DsEmpty: {
          props: ["title"],
          template: "<section data-testid='detail-empty'>{{ title }}<slot name='action' /></section>"
        },
        "el-button": { template: "<button><slot /></button>" }
      }
    }
  });
  await flushPromises();
  return wrapper;
}
