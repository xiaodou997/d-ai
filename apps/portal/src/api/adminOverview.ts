import { createTypedOperationRequest, type OperationQuery, type OperationResponse } from ".";
import { authenticatedRequest, apiBaseUrl, apiHeaders } from "./request";
import type { OverviewSnapshot } from "./types/overview";

export type AdminOverviewQuery = OperationQuery<"admin-overview-snapshot">;
export type AdminOverviewSnapshotResponse = OperationResponse<"admin-overview-snapshot">;

const request = createTypedOperationRequest(authenticatedRequest());

export const adminOverviewApi = {
  async getSnapshot(query: AdminOverviewQuery, signal?: AbortSignal): Promise<OverviewSnapshot> {
    const value = await request<"admin-overview-snapshot">({
      method: "GET",
      path: "/api/v1/admin/overview/snapshot",
      query,
      headers: apiHeaders,
      baseUrl: apiBaseUrl,
      signal
    });
    return {
      meta: value.meta,
      summary: value.summary,
      comparison: value.comparison,
      trends: value.trends ?? [],
      accounts: value.accounts ?? [],
      models: value.models ?? [],
      tenants: value.tenants ?? [],
      users: value.users ?? [],
      included: value.included
    } as OverviewSnapshot;
  }
};
