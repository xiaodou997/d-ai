import { defineComponent } from "vue";
import { flushPromises, shallowMount } from "@vue/test-utils";
import { beforeEach, describe, expect, it, vi } from "vitest";

import CustomerWorkspace from "./CustomerWorkspace.vue";

const api = vi.hoisted(() => ({
  listMyGroups: vi.fn(),
  listWorkspaceChatSessions: vi.fn(),
  listWorkspaceImageJobs: vi.fn()
}));
const balanceApi = vi.hoisted(() => ({ getBalance: vi.fn() }));
const usageApi = vi.hoisted(() => ({
  getCustomerUsageSummary: vi.fn(),
  listCustomerUsageRecords: vi.fn()
}));
const router = vi.hoisted(() => ({ push: vi.fn() }));

vi.mock("@/api/aiCustomer", () => ({ aiCustomerApi: api }));
vi.mock("@/api/platformCustomer", () => ({ platformCustomerApi: balanceApi }));
vi.mock("@/features/ai/usage", () => usageApi);
vi.mock("vue-router", () => ({ useRouter: () => router }));
vi.mock("echarts/core", () => ({
  init: vi.fn(() => ({ dispose: vi.fn(), resize: vi.fn(), setOption: vi.fn() })),
  use: vi.fn()
}));
vi.mock("echarts/charts", () => ({ LineChart: {}, PieChart: {} }));
vi.mock("echarts/components", () => ({ GridComponent: {}, LegendComponent: {}, TooltipComponent: {} }));
vi.mock("echarts/renderers", () => ({ CanvasRenderer: {} }));

const PanelStub = defineComponent({
  template: "<div><slot name=\"actions\" /><slot /></div>"
});
const CardStub = defineComponent({
  template: "<section><slot name=\"header\" /><slot name=\"actions\" /><slot /></section>"
});
const ButtonStub = defineComponent({
  emits: ["click"],
  template: "<button @click=\"$emit('click')\"><slot /></button>"
});

describe("CustomerWorkspace", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    api.listMyGroups.mockResolvedValue({ items: [{ id: "group-1", name: "默认分组", status: "active", effective_user_multiplier: 1.25 }] });
    api.listWorkspaceChatSessions.mockResolvedValue({ items: [{ id: "session-1", title: "最近对话", model_code: "gpt-5" }] });
    api.listWorkspaceImageJobs.mockResolvedValue({ items: [] });
    balanceApi.getBalance.mockResolvedValue({ remainingUsd: 12.5, availableUsd: 10, outstandingDebtMicroUsd: 2500000 });
    usageApi.listCustomerUsageRecords.mockResolvedValue({ items: [] });
    usageApi.getCustomerUsageSummary.mockResolvedValue({
      total_user_charged_usd: 1.2,
      total_prompt_tokens: 10,
      total_completion_tokens: 20,
      request_count: 2
    });
  });

  it("loads the customer overview and routes through workspace actions", async () => {
    const wrapper = shallowMount(CustomerWorkspace, {
      global: {
        stubs: {
          PortalPagePanel: PanelStub,
          PortalContentCard: CardStub,
          PortalMetricGrid: true,
          DsTable: true,
          DsTag: true,
          PortalImageJobTable: true,
          UsageCostCell: true,
          UsageTag: true,
          UsageTokenCell: true,
          ElButton: ButtonStub,
          ElSelect: true,
          ElOption: true,
          ElAlert: true
        }
      }
    });
    await flushPromises();

    expect(api.listMyGroups).toHaveBeenCalledOnce();
    expect(api.listWorkspaceChatSessions).toHaveBeenCalledWith({ limit: 6 }, expect.any(AbortSignal));
    expect(api.listWorkspaceImageJobs).toHaveBeenCalledWith({ limit: 6 }, expect.any(AbortSignal));
    expect(balanceApi.getBalance).toHaveBeenCalledWith(false);
    expect(usageApi.listCustomerUsageRecords).toHaveBeenCalledWith(
      { limit: 100, request_source: undefined },
      expect.any(AbortSignal)
    );

    await wrapper.findAll("button").find((button) => button.text() === "进入对话")!.trigger("click");
    expect(router.push).toHaveBeenCalledWith("/customer/workbench/chat");

    await wrapper.findAll("button").find((button) => button.text() === "进入生图")!.trigger("click");
    expect(router.push).toHaveBeenCalledWith("/customer/workbench/images");

    wrapper.unmount();
  });
});
