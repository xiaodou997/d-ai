import { beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({ request: vi.fn() }));

vi.mock("./request", () => ({
  apiBaseUrl: "/api",
  apiHeaders: { Accept: "application/json" },
  authenticatedRequest: () => mocks.request
}));

import { platformAdminApi } from "./platformAdmin";

beforeEach(() => mocks.request.mockReset());

const paymentPackage = {
  id: "pkg-1",
  name: "Starter",
  paymentAmountMicroUsd: 1_000_000,
  giftAmountMicroUsd: 100_000,
  validityDays: 30,
  badge: "popular",
  enabled: true,
  sortOrder: 10
};

const wechat = {
  enabled: true,
  mock: false,
  verifyMode: "platform_cert",
  appId: "app-1",
  mchId: "mch-1",
  mchCertSerialNo: "serial-1",
  notifyBaseUrl: "https://example.com",
  orderTtlSeconds: 900,
  hasPrivateKey: true,
  hasApiv3Key: true,
  wechatPayPublicKeyId: "key-1",
  hasWechatPayPublicKey: true,
  $schema: "ignored"
};

describe("platform admin payment generated operation facade", () => {
  it("normalizes payment settings and maps nullable package fields", async () => {
    mocks.request.mockResolvedValueOnce({
      tenantCustomTopupFeeBp: 100,
      tenantWithdrawFeeBp: 50,
      tenantCustomValidityDays: undefined,
      tenantTopupPackages: [paymentPackage, { ...paymentPackage, id: "pkg-2", validityDays: undefined }],
      $schema: "ignored"
    }).mockResolvedValueOnce({
      tenantCustomTopupFeeBp: 100,
      tenantWithdrawFeeBp: 50,
      tenantCustomValidityDays: 7,
      tenantTopupPackages: null,
      $schema: "ignored"
    });

    await expect(platformAdminApi.getPaymentSettings()).resolves.toEqual({
      tenantCustomTopupFeeBp: 100,
      tenantWithdrawFeeBp: 50,
      tenantCustomValidityDays: null,
      tenantTopupPackages: [
        { ...paymentPackage },
        { ...paymentPackage, id: "pkg-2", validityDays: null }
      ]
    });
    await expect(platformAdminApi.updatePaymentSettings({
      tenantCustomTopupFeeBp: 100,
      tenantWithdrawFeeBp: 50,
      tenantCustomValidityDays: null,
      tenantTopupPackages: [{ ...paymentPackage, validityDays: null }]
    })).resolves.toMatchObject({ tenantCustomValidityDays: 7, tenantTopupPackages: [] });

    expect(mocks.request.mock.calls[1]?.[0]).toMatchObject({
      body: { tenantCustomValidityDays: undefined, tenantTopupPackages: [{ id: "pkg-1", validityDays: undefined }] }
    });
  });

  it("maps WeChat config and fills nullable secret fields for generated writes", async () => {
    mocks.request.mockResolvedValueOnce(wechat).mockResolvedValueOnce(wechat);

    await expect(platformAdminApi.getWechatConfig()).resolves.toMatchObject({ verifyMode: "platform_cert", hasApiv3Key: true });
    await expect(platformAdminApi.updateWechatConfig({
      enabled: true,
      mock: false,
      verifyMode: "public_key",
      appId: "app-1",
      mchId: "mch-1",
      mchCertSerialNo: "serial-1",
      notifyBaseUrl: "https://example.com",
      orderTtlSeconds: 900
    })).resolves.toMatchObject({ verifyMode: "platform_cert" });

    expect(mocks.request.mock.calls[1]?.[0]).toMatchObject({
      body: {
        verifyMode: "public_key",
        mchPrivateKey: null,
        apiv3Key: null,
        wechatPayPublicKeyId: null,
        wechatPayPublicKey: null
      }
    });
  });

  it("maps payment order pages and binds sync path parameters", async () => {
    const order = {
      orderId: "order-1",
      scene: "tenant_topup",
      tenantName: "Tenant One",
      username: "alice",
      status: "paid",
      paymentCurrency: "CNY",
      paymentAmountMinor: 100,
      grossAmountMicroUsd: 1_000_000,
      feeAmountMicroUsd: 10_000,
      giftAmountMicroUsd: 100_000,
      creditedAmountMicroUsd: 1_090_000,
      topupMode: "package",
      packageName: "Starter",
      transactionId: "tx-1",
      createdAt: 100,
      paidAt: undefined,
      balanceExpiresAt: undefined
    };
    const sync = {
      orderId: "order-1",
      status: "paid",
      topupMode: "package",
      packageName: "Starter",
      paymentCurrency: "CNY",
      paymentAmountMinor: 100,
      grossAmountMicroUsd: 1_000_000,
      feeAmountMicroUsd: 10_000,
      giftAmountMicroUsd: 100_000,
      creditedAmountMicroUsd: 1_090_000,
      paidAt: 100,
      balanceExpiresAt: 200,
      transactionId: "tx-1",
      $schema: "ignored"
    };
    mocks.request.mockResolvedValueOnce({ items: [order], total: 1, page: 1, size: 20, $schema: "ignored" }).mockResolvedValueOnce(sync);

    await expect(platformAdminApi.listPaymentOrders({ scene: "tenant_topup", page: 1, size: 20 })).resolves.toMatchObject({
      items: [{ orderId: "order-1", scene: "tenant_topup", topupMode: "package", paidAt: null, balanceExpiresAt: null }],
      total: 1,
      page: 1,
      size: 20
    });
    await expect(platformAdminApi.syncPaymentOrder("order/1")).resolves.toMatchObject({ orderId: "order-1", status: "paid" });

    expect(mocks.request.mock.calls[0]?.[0]).toMatchObject({ query: { scene: "tenant_topup", page: 1, size: 20 } });
    expect(mocks.request.mock.calls[1]?.[0]).toMatchObject({
      path: "/api/v1/admin/payment-orders/order%2F1/sync",
      pathParams: { orderId: "order/1" }
    });
  });

  it("rejects unknown WeChat modes and payment order dimensions", async () => {
    expect(() => platformAdminApi.updateWechatConfig({ ...wechat, verifyMode: "legacy" as never })).toThrow("Unexpected WeChat verify mode");

    mocks.request.mockResolvedValueOnce({
      items: [{
        orderId: "order-1",
        scene: "other",
        status: "paid",
        paymentCurrency: "CNY",
        paymentAmountMinor: 100,
        grossAmountMicroUsd: 1,
        feeAmountMicroUsd: 0,
        giftAmountMicroUsd: 0,
        creditedAmountMicroUsd: 1,
        topupMode: "package",
        createdAt: 100
      }],
      total: 1,
      page: 1,
      size: 20
    });
    await expect(platformAdminApi.listPaymentOrders()).rejects.toThrow("Unexpected payment order scene");
  });
});
