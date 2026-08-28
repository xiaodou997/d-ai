import { beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({ request: vi.fn() }));

vi.mock("./request", () => ({
  apiBaseUrl: "/api",
  apiHeaders: { Accept: "application/json" },
  authenticatedRequest: () => mocks.request
}));

import { platformAdminApi } from "./platformAdmin";

beforeEach(() => mocks.request.mockReset());

const withdrawal = {
  withdrawalId: "withdrawal-1",
  currency: "USD",
  amountMicroUsd: 1_000_000,
  feeAmountMicroUsd: 10_000,
  payoutAmountMicroUsd: 990_000,
  accountName: "Tenant One",
  bankName: "Example Bank",
  accountNo: "1234",
  status: "pending",
  applyNote: "request",
  reviewNote: undefined,
  paymentRef: undefined,
  paidAt: undefined,
  createdAt: 100,
  $schema: "ignored"
};

describe("platform admin withdrawals generated operation facade", () => {
  it("normalizes nullable withdrawal pages and forwards typed filters", async () => {
    mocks.request.mockResolvedValueOnce({ items: null, total: 0, page: 1, size: 20, $schema: "ignored" });

    await expect(platformAdminApi.listWithdrawals({ status: "pending", page: 1, size: 20 })).resolves.toEqual({
      items: [],
      total: 0,
      page: 1,
      size: 20
    });
    expect(mocks.request.mock.calls[0]?.[0]).toMatchObject({
      path: "/api/v1/admin/withdrawals",
      query: { status: "pending", page: 1, size: 20 }
    });
  });

  it("maps created withdrawals and removes transport schema", async () => {
    mocks.request.mockResolvedValueOnce(withdrawal);

    await expect(platformAdminApi.createWithdrawal({
      tenantId: "tenant-1",
      amountMicroUsd: 1_000_000,
      accountName: "Tenant One",
      bankName: "Example Bank",
      accountNo: "1234",
      note: "request"
    })).resolves.toEqual({
      withdrawalId: "withdrawal-1",
      currency: "USD",
      amountMicroUsd: 1_000_000,
      feeAmountMicroUsd: 10_000,
      payoutAmountMicroUsd: 990_000,
      accountName: "Tenant One",
      bankName: "Example Bank",
      accountNo: "1234",
      status: "pending",
      applyNote: "request",
      reviewNote: undefined,
      paymentRef: undefined,
      paidAt: null,
      createdAt: 100
    });
    expect(mocks.request.mock.calls[0]?.[0]).toMatchObject({
      path: "/api/v1/admin/withdrawals",
      body: { tenantId: "tenant-1", amountMicroUsd: 1_000_000, accountNo: "1234" }
    });
  });

  it("keeps withdrawal status as a page string while preserving generated response shape", async () => {
    mocks.request.mockResolvedValueOnce({ items: [{ ...withdrawal, status: "paid" }], total: 1, page: 1, size: 20 });
    await expect(platformAdminApi.listWithdrawals()).resolves.toMatchObject({ items: [{ status: "paid" }] });
  });
});
