import type { RequestAdapter } from "@/api";
import type { operations } from "@/api/ai";

import { authenticatedRequest, portalHeadersFor, serviceBaseUrl } from "@/api/request";

type AccountBatchDeleteOperation = operations["ai-batch-delete-account-model-bindings"];
type PoolBatchDeleteOperation = operations["ai-batch-delete-pool-model-bindings"];
type BatchDeleteRequest = AccountBatchDeleteOperation["requestBody"]["content"]["application/json"];
type AccountBatchDeleteResult = AccountBatchDeleteOperation["responses"][200]["content"]["application/json"];
type PoolBatchDeleteResult = PoolBatchDeleteOperation["responses"][200]["content"]["application/json"];

export interface UpstreamModelBindingBatchApi {
  deleteAccountBindings(accountId: string, bindingIds: string[]): Promise<AccountBatchDeleteResult>;
  deletePoolBindings(poolId: string, bindingIds: string[]): Promise<PoolBatchDeleteResult>;
}

export function createUpstreamModelBindingBatchApi(
  adapter: RequestAdapter = authenticatedRequest("ai")
): UpstreamModelBindingBatchApi {
  const headers = portalHeadersFor("ai");
  const baseUrl = serviceBaseUrl("ai");
  const body = (bindingIds: string[]): BatchDeleteRequest => ({ binding_ids: bindingIds });

  return {
    deleteAccountBindings: (accountId, bindingIds) => adapter<AccountBatchDeleteResult>({
      method: "POST",
      path: `/api/v1/upstream-accounts/${encodeURIComponent(accountId)}/model-bindings/batch-delete`,
      headers,
      body: body(bindingIds),
      baseUrl
    }),
    deletePoolBindings: (poolId, bindingIds) => adapter<PoolBatchDeleteResult>({
      method: "POST",
      path: `/api/v1/credential-pools/${encodeURIComponent(poolId)}/model-bindings/batch-delete`,
      headers,
      body: body(bindingIds),
      baseUrl
    })
  };
}

export const upstreamModelBindingBatchApi = createUpstreamModelBindingBatchApi();
