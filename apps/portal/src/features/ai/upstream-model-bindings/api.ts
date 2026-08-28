import {
  createTypedOperationRequest,
  type OperationBody,
  type OperationResponse,
  type RequestAdapter
} from "@/api";

import { authenticatedRequest, apiHeaders, apiBaseUrl } from "@/api/request";

type BatchDeleteRequest = OperationBody<"ai-batch-delete-account-model-bindings">;
type AccountBatchDeleteResult = OperationResponse<"ai-batch-delete-account-model-bindings">;
type PoolBatchDeleteResult = OperationResponse<"ai-batch-delete-pool-model-bindings">;

export interface UpstreamModelBindingBatchApi {
  deleteAccountBindings(accountId: string, bindingIds: string[]): Promise<AccountBatchDeleteResult>;
  deletePoolBindings(poolId: string, bindingIds: string[]): Promise<PoolBatchDeleteResult>;
}

export function createUpstreamModelBindingBatchApi(
  adapter: RequestAdapter = authenticatedRequest()
): UpstreamModelBindingBatchApi {
  const request = createTypedOperationRequest(adapter);
  const headers = apiHeaders;
  const baseUrl = apiBaseUrl;
  const body = (bindingIds: string[]): BatchDeleteRequest => ({ binding_ids: bindingIds });

  return {
    deleteAccountBindings: (accountId, bindingIds) => request<"ai-batch-delete-account-model-bindings">({
      method: "POST",
      path: `/api/v1/upstream-accounts/${encodeURIComponent(accountId)}/model-bindings/batch-delete`,
      pathParams: { accountID: accountId },
      headers,
      body: body(bindingIds),
      baseUrl
    }),
    deletePoolBindings: (poolId, bindingIds) => request<"ai-batch-delete-pool-model-bindings">({
      method: "POST",
      path: `/api/v1/credential-pools/${encodeURIComponent(poolId)}/model-bindings/batch-delete`,
      pathParams: { poolID: poolId },
      headers,
      body: body(bindingIds),
      baseUrl
    })
  };
}

export const upstreamModelBindingBatchApi = createUpstreamModelBindingBatchApi();
