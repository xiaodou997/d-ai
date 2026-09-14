import { createTypedOperationRequest, type OperationResponse } from '@/api';
import { authenticatedRequest, apiHeaders, apiBaseUrl } from '@/api/request';

export type Stability = OperationResponse<'ai-get-upstream-stability'>;
export type StabilityWindow = '1h' | '24h' | '7d';
export type ResourceKind = 'direct_upstream' | 'oauth_pool';
const request = createTypedOperationRequest(authenticatedRequest());
export const stabilityApi = {
  list: (kind: ResourceKind, window: StabilityWindow, signal?: AbortSignal) => request<'ai-list-upstream-stability'>({ method: 'GET', path: '/api/v1/upstream-stability', query: { kind, window }, headers: apiHeaders, baseUrl: apiBaseUrl, signal }),
  detail: (kind: ResourceKind, id: string, window: StabilityWindow, signal?: AbortSignal) => request<'ai-get-upstream-stability'>({ method: 'GET', path: `/api/v1/upstream-stability/${kind}/${encodeURIComponent(id)}`, pathParams: { kind, id }, query: { window }, headers: apiHeaders, baseUrl: apiBaseUrl, signal }),
  resume: (kind: ResourceKind, id: string) => request<'ai-resume-upstream'>({ method: 'POST', path: `/api/v1/upstream-stability/${kind}/${encodeURIComponent(id)}/resume`, pathParams: { kind, id }, headers: apiHeaders, baseUrl: apiBaseUrl })
};
