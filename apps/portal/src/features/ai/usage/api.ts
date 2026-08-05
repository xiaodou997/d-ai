import type { RequestAdapter } from "@dai/api-client";
import type { components } from "@dai/api-client/ai";

import { authenticatedRequest, portalHeadersFor, serviceBaseUrl } from "../../../api/request";
import type { CustomerUsageQuery } from "./model";

type Schemas = components["schemas"];

export interface CustomerUsageApi {
  listRecords: (query: CustomerUsageQuery, signal?: AbortSignal) => Promise<Schemas["UserUsageLogsOutputBody"]>;
  getSummary: (requestSource?: string, signal?: AbortSignal) => Promise<Schemas["UserUsageSummaryDTO"]>;
}

export function createCustomerUsageApi(adapter: RequestAdapter = authenticatedRequest("ai")): CustomerUsageApi {
  return {
    listRecords: (query, signal) => adapter({
      method: "GET",
      path: "/api/v1/user-usage-logs",
      query,
      headers: portalHeadersFor("ai"),
      baseUrl: serviceBaseUrl("ai"),
      signal
    }),
    getSummary: (requestSource, signal) => adapter({
      method: "GET",
      path: "/api/v1/user-usage-summary",
      query: { request_source: requestSource || undefined },
      headers: portalHeadersFor("ai"),
      baseUrl: serviceBaseUrl("ai"),
      signal
    })
  };
}

export const customerUsageApi = createCustomerUsageApi();

export function listCustomerUsageRecords(query: CustomerUsageQuery, signal?: AbortSignal) {
  return customerUsageApi.listRecords(query, signal);
}

export function getCustomerUsageSummary(requestSource?: string, signal?: AbortSignal) {
  return customerUsageApi.getSummary(requestSource, signal);
}
