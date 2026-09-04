import { createTypedOperationRequest, type OperationBody, type OperationQuery, type OperationResponse } from '@/api'
import type { components } from '@/api/generated/dai'
import { apiBaseUrl, apiHeaders, authenticatedRequest } from '@/api/request'

const typedRequest = createTypedOperationRequest(authenticatedRequest())

export type PromptAuditConfig = components['schemas']['PromptAuditConfigDTO']
export type PromptAuditEndpoint = components['schemas']['PromptAuditEndpointDTO']
export type PromptAuditEndpointWrite = components['schemas']['PromptAuditEndpointWriteDTO']
export type PromptAuditConfigWrite = OperationBody<'ai-update-prompt-audit-config'>
export type PromptAuditEvent = components['schemas']['StoredEvent']
export type PromptAuditEventPage = OperationResponse<'ai-list-prompt-audit-events'>
export type PromptAuditEventQuery = OperationQuery<'ai-list-prompt-audit-events'>
export type PromptAuditProbe = OperationResponse<'ai-probe-prompt-audit-endpoint'>
export type PromptAuditRuntime = OperationResponse<'ai-get-prompt-audit-runtime'>

export function getPromptAuditConfig() {
  return typedRequest<'ai-get-prompt-audit-config'>({ method: 'GET', path: '/api/v1/prompt-audit/config', headers: apiHeaders, baseUrl: apiBaseUrl })
}

export function updatePromptAuditConfig(body: PromptAuditConfigWrite) {
  return typedRequest<'ai-update-prompt-audit-config'>({ method: 'PUT', path: '/api/v1/prompt-audit/config', headers: apiHeaders, baseUrl: apiBaseUrl, body })
}

export function probePromptAuditEndpoint(body: PromptAuditEndpointWrite) {
  return typedRequest<'ai-probe-prompt-audit-endpoint'>({ method: 'POST', path: '/api/v1/prompt-audit/endpoints/probe', headers: apiHeaders, baseUrl: apiBaseUrl, body })
}

export function listPromptAuditEvents(query: PromptAuditEventQuery = {}) {
  return typedRequest<'ai-list-prompt-audit-events'>({ method: 'GET', path: '/api/v1/prompt-audit/events', headers: apiHeaders, baseUrl: apiBaseUrl, query })
}

export function getPromptAuditRuntime() {
  return typedRequest<'ai-get-prompt-audit-runtime'>({ method: 'GET', path: '/api/v1/prompt-audit/runtime', headers: apiHeaders, baseUrl: apiBaseUrl })
}

export function deletePromptAuditEvent(eventID: string) {
  return typedRequest<'ai-delete-prompt-audit-event'>({ method: 'DELETE', path: `/api/v1/prompt-audit/events/${encodeURIComponent(eventID)}`, pathParams: { eventID }, headers: apiHeaders, baseUrl: apiBaseUrl })
}
