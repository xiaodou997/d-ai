import { beforeEach, describe, expect, it, vi } from "vitest";

import { urmAdminApi } from "../../../api/urmAdmin";
import type { ServiceRegistryDetail, ServiceRegistryItem } from "../../../types/admin";
import { useServiceRegistry } from "./useServiceRegistry";

vi.mock("../../../api/urmAdmin", () => ({
  urmAdminApi: {
    listServices: vi.fn(),
    getService: vi.fn()
  }
}));
vi.mock("element-plus", () => ({ ElMessage: { error: vi.fn() } }));

const summary: ServiceRegistryItem = {
  id: 1,
  serviceId: "uni-ai-api",
  displayName: "AI Gateway",
  description: "AI service",
  status: "active",
  portalEnabled: true,
  sourceCount: 0,
  instanceCount: 1,
  onlineInstances: 1,
  createdAt: "2026-07-11T00:00:00Z",
  updatedAt: "2026-07-11T00:00:00Z"
};

describe("useServiceRegistry", () => {
  beforeEach(() => vi.clearAllMocks());

  it("keeps the list summary when an older detail response omits its fields", async () => {
    vi.mocked(urmAdminApi.getService).mockResolvedValue({
      sources: [],
      instances: []
    } as unknown as ServiceRegistryDetail);
    const registry = useServiceRegistry();

    await registry.selectService(summary);

    expect(registry.selected.value).toEqual(expect.objectContaining({
      serviceId: "uni-ai-api",
      displayName: "AI Gateway",
      status: "active",
      portalEnabled: true,
      sources: [],
      instances: []
    }));
  });

  it("does not let an older detail request replace the latest selection", async () => {
    let resolveFirst!: (value: ServiceRegistryDetail) => void;
    let resolveSecond!: (value: ServiceRegistryDetail) => void;
    vi.mocked(urmAdminApi.getService)
      .mockReturnValueOnce(new Promise((resolve) => { resolveFirst = resolve; }))
      .mockReturnValueOnce(new Promise((resolve) => { resolveSecond = resolve; }));
    const proxySummary: ServiceRegistryItem = {
      ...summary,
      id: 2,
      serviceId: "uni-api-proxy",
      displayName: "API Proxy"
    };
    const registry = useServiceRegistry();

    const firstRequest = registry.selectService(summary);
    const secondRequest = registry.selectService(proxySummary);
    resolveSecond({ ...proxySummary, sources: [], instances: [] });
    await secondRequest;
    resolveFirst({ ...summary, sources: [], instances: [] });
    await firstRequest;

    expect(registry.selected.value?.serviceId).toBe("uni-api-proxy");
    expect(registry.detailLoading.value).toBe(false);
  });
});
