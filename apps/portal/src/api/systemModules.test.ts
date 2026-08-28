import { beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({ request: vi.fn() }));

vi.mock("./request", () => ({
  apiBaseUrl: "/",
  apiHeaders: { Accept: "application/json" },
  authenticatedRequest: () => mocks.request
}));

import { systemModulesApi } from "./systemModules";

const moduleStatus = {
  name: "proxy_egress",
  displayName: "代理出口节点",
  description: "代理出口",
  category: "integration",
  adminRoute: "/admin/proxy-nodes",
  order: 20,
  available: true,
  enabled: true,
  active: true,
  configValidated: true,
  health: "healthy"
};

const proxyNode = {
  id: "node-1",
  name: "primary",
  proxyType: "http",
  endpoint: "http://proxy.example.com",
  weight: 100,
  status: "active",
  healthStatus: "healthy",
  createdAt: "2026-08-28T04:00:00Z",
  updatedAt: "2026-08-28T04:00:00Z"
};

const cleanupPolicy = {
  enabled: true,
  requestBodyDays: 30,
  requestPayloadDays: 180,
  notificationDays: 90,
  moderationDays: 90,
  riskEventDays: 365,
  adminAuditDays: 365,
  auditBlobDays: 180,
  usageRollupDays: 730,
  batchSize: 1000
};

const cleanupRun = {
  id: "run-1",
  trigger: "manual",
  status: "queued",
  targets: null,
  summary: {},
  createdAt: "2026-08-28T04:00:00Z"
};

beforeEach(() => mocks.request.mockReset());

describe("system modules generated operation facade", () => {
  it("normalizes module and PII transport responses", async () => {
    mocks.request
      .mockResolvedValueOnce(null)
      .mockResolvedValueOnce(moduleStatus)
      .mockResolvedValueOnce({ enabled: true, rules: null, placeholderPrefix: "DAI" })
      .mockResolvedValueOnce({ enabled: false, rules: [], placeholderPrefix: "DAI" })
      .mockResolvedValueOnce({ enabled: true, rules: null, placeholderPrefix: "SAFE" })
      .mockResolvedValueOnce({ protectedText: "<SAFE_1>" });

    await expect(systemModulesApi.list()).resolves.toEqual([]);
    await expect(systemModulesApi.setEnabled("proxy/egress", true)).resolves.toEqual(moduleStatus);
    await expect(systemModulesApi.getPIIProtectionConfig()).resolves.toEqual({
      enabled: true,
      rules: [],
      placeholderPrefix: "DAI"
    });
    await expect(systemModulesApi.getPIIProtectionDefaults()).resolves.toMatchObject({ rules: [] });
    await expect(
      systemModulesApi.updatePIIProtectionConfig({ enabled: true, rules: [], placeholderPrefix: "SAFE" })
    ).resolves.toMatchObject({ rules: [], placeholderPrefix: "SAFE" });
    await expect(
      systemModulesApi.previewPIIProtection({
        config: { enabled: true, rules: [], placeholderPrefix: "SAFE" },
        text: "alice@example.com"
      })
    ).resolves.toEqual({ protectedText: "<SAFE_1>" });

    expect(mocks.request.mock.calls[1]?.[0]).toMatchObject({
      path: "/api/v1/admin/modules/proxy%2Fegress/enabled",
      pathParams: { name: "proxy/egress" },
      body: { enabled: true }
    });
  });

  it("normalizes proxy nodes and notification delivery responses", async () => {
    mocks.request
      .mockResolvedValueOnce(null)
      .mockResolvedValueOnce(proxyNode)
      .mockResolvedValueOnce({ ...proxyNode, name: "secondary" })
      .mockResolvedValueOnce({ message: "deleted" })
      .mockResolvedValueOnce({
        id: "notification-1",
        eventKey: "system.test",
        channel: "in_app",
        title: "Test",
        body: "Body",
        payload: {},
        status: "sent",
        attempts: 1,
        createdAt: "2026-08-28T04:00:00Z"
      });

    await expect(systemModulesApi.listProxyNodes()).resolves.toEqual([]);
    await expect(
      systemModulesApi.createProxyNode({
        name: "primary",
        proxyType: "http",
        endpoint: "http://proxy.example.com"
      })
    ).resolves.toMatchObject({ id: "node-1", proxyType: "http", status: "active" });
    await expect(
      systemModulesApi.updateProxyNode("node/1", {
        name: "secondary",
        proxyType: "http",
        endpoint: "http://proxy.example.com"
      })
    ).resolves.toMatchObject({ name: "secondary" });
    await expect(systemModulesApi.deleteProxyNode("node/1")).resolves.toBeUndefined();
    await expect(
      systemModulesApi.sendNotification({
        eventKey: "system.test",
        channel: "in_app",
        title: "Test",
        body: "Body"
      })
    ).resolves.toEqual({ id: "notification-1", status: "sent" });

    expect(mocks.request.mock.calls[2]?.[0]).toMatchObject({
      path: "/api/v1/admin/proxy-nodes/node%2F1",
      pathParams: { id: "node/1" }
    });
  });

  it("normalizes cleanup collections and accepted run responses", async () => {
    mocks.request
      .mockResolvedValueOnce(cleanupPolicy)
      .mockResolvedValueOnce(cleanupPolicy)
      .mockResolvedValueOnce({
        policy: cleanupPolicy,
        generatedAt: "2026-08-28T04:00:00Z",
        items: null
      })
      .mockResolvedValueOnce(null)
      .mockResolvedValueOnce(cleanupRun);

    await expect(systemModulesApi.getCleanupPolicy()).resolves.toEqual(cleanupPolicy);
    await expect(systemModulesApi.updateCleanupPolicy(cleanupPolicy)).resolves.toEqual(cleanupPolicy);
    await expect(systemModulesApi.previewCleanup()).resolves.toMatchObject({ items: [] });
    await expect(systemModulesApi.listCleanupRuns()).resolves.toEqual([]);
    await expect(
      systemModulesApi.startCleanup({ targets: ["notifications"], confirmation: "CLEANUP_DATA" })
    ).resolves.toMatchObject({ id: "run-1", status: "queued", targets: [] });
  });

  it("rejects unknown transport enums before they reach page state", async () => {
    mocks.request
      .mockResolvedValueOnce({ ...proxyNode, proxyType: "ftp" })
      .mockResolvedValueOnce({ ...cleanupRun, trigger: "remote", targets: [] });

    await expect(
      systemModulesApi.createProxyNode({ name: "invalid", proxyType: "http", endpoint: "http://proxy" })
    ).rejects.toThrow("Unexpected proxy type: ftp");
    await expect(
      systemModulesApi.startCleanup({ targets: ["notifications"], confirmation: "CLEANUP_DATA" })
    ).rejects.toThrow("Unexpected cleanup trigger: remote");
  });
});
