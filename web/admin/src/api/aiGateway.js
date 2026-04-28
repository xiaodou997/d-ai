import axios from 'axios'
import { ElMessage } from 'element-plus'

const gatewayRequest = axios.create({
  baseURL: import.meta.env.VITE_AI_GATEWAY_BASE_URL || '',
  timeout: 30000,
  headers: {
    'Content-Type': 'application/json'
  }
})

gatewayRequest.interceptors.request.use((config) => {
  const adminToken =
    localStorage.getItem('uni-ai-api-admin-token') ||
    import.meta.env.VITE_AI_GATEWAY_ADMIN_TOKEN
  const accessToken = localStorage.getItem('admin_accessToken')

  if (adminToken) {
    config.headers['X-Admin-Token'] = adminToken
  }
  if (accessToken) {
    config.headers.Authorization = `Bearer ${accessToken}`
  }

  return config
})

gatewayRequest.interceptors.response.use(
  (response) => response.data,
  (error) => {
    const message = error.response?.data?.error || error.response?.data?.message || 'AI Gateway 请求失败'
    ElMessage.error(message)
    return Promise.reject(error)
  }
)

export const statusOptions = [
  { label: '启用', value: 'active' },
  { label: '停用', value: 'inactive' },
  { label: '禁用', value: 'disabled' }
]

export const providerTypeOptions = [
  { label: '官方厂商', value: 'official' },
  { label: '兼容协议', value: 'compatible' },
  { label: '私有化', value: 'private' },
  { label: '自定义', value: 'custom' }
]

export const protocolOptions = [
  { label: 'OpenAI Chat Completions', value: 'openai_chat_completions' },
  { label: 'OpenAI Images Generations', value: 'openai_images_generations' },
  { label: 'OpenAI Responses', value: 'openai_responses' },
  { label: 'OpenAI Embeddings', value: 'openai_embeddings' },
  { label: 'Anthropic Messages', value: 'anthropic_messages' }
]

export const capabilityOptions = [
  { label: '文本对话', value: 'chat' },
  { label: '生图', value: 'image' },
  { label: '视频', value: 'video' },
  { label: 'Embedding', value: 'embedding' },
  { label: '音频', value: 'audio' },
  { label: '重排', value: 'rerank' }
]

export function listProviders() {
  return gatewayRequest.get('/admin/providers')
}

export function createProvider(data) {
  return gatewayRequest.post('/admin/providers', data)
}

export function updateProvider(providerId, data) {
  return gatewayRequest.patch(`/admin/providers/${providerId}`, data)
}

export function updateProviderStatus(providerId, status) {
  return gatewayRequest.patch(`/admin/providers/${providerId}/status`, { status })
}

export function listProviderEndpoints(providerId) {
  return gatewayRequest.get(`/admin/providers/${providerId}/endpoints`)
}

export function createProviderEndpoint(providerId, data) {
  return gatewayRequest.post(`/admin/providers/${providerId}/endpoints`, data)
}

export function updateProviderEndpoint(providerId, endpointId, data) {
  return gatewayRequest.patch(`/admin/providers/${providerId}/endpoints/${endpointId}`, data)
}

export function updateProviderEndpointStatus(providerId, endpointId, status) {
  return gatewayRequest.patch(`/admin/providers/${providerId}/endpoints/${endpointId}/status`, { status })
}

export function checkProviderEndpointHealth(providerId, endpointId) {
  return gatewayRequest.post(`/admin/providers/${providerId}/endpoints/${endpointId}/health-check`)
}

export function listProviderModelPrices(providerId) {
  return gatewayRequest.get(`/admin/providers/${providerId}/model-prices`)
}

export function createProviderModelPrice(providerId, data) {
  return gatewayRequest.post(`/admin/providers/${providerId}/model-prices`, data)
}

export function updateProviderModelPrice(providerId, priceId, data) {
  return gatewayRequest.patch(`/admin/providers/${providerId}/model-prices/${priceId}`, data)
}

export function updateProviderModelPriceStatus(providerId, priceId, status) {
  return gatewayRequest.patch(`/admin/providers/${providerId}/model-prices/${priceId}/status`, { status })
}

export function listModels() {
  return gatewayRequest.get('/admin/models')
}

export function createModel(data) {
  return gatewayRequest.post('/admin/models', data)
}

export function updateModel(modelId, data) {
  return gatewayRequest.patch(`/admin/models/${modelId}`, data)
}

export function updateModelStatus(modelId, status) {
  return gatewayRequest.patch(`/admin/models/${modelId}/status`, { status })
}

export function listModelPrices(modelId) {
  return gatewayRequest.get(`/admin/models/${modelId}/prices`)
}

export function createModelPrice(modelId, data) {
  return gatewayRequest.post(`/admin/models/${modelId}/prices`, data)
}

export function updateModelPrice(modelId, priceId, data) {
  return gatewayRequest.patch(`/admin/models/${modelId}/prices/${priceId}`, data)
}

export function updateModelPriceStatus(modelId, priceId, status) {
  return gatewayRequest.patch(`/admin/models/${modelId}/prices/${priceId}/status`, { status })
}

export function listModelDeployments(modelId) {
  return gatewayRequest.get(`/admin/models/${modelId}/deployments`)
}

export function createModelDeployment(modelId, data) {
  return gatewayRequest.post(`/admin/models/${modelId}/deployments`, data)
}

export function updateModelDeployment(modelId, deploymentId, data) {
  return gatewayRequest.patch(`/admin/models/${modelId}/deployments/${deploymentId}`, data)
}

export function updateModelDeploymentStatus(modelId, deploymentId, status) {
  return gatewayRequest.patch(`/admin/models/${modelId}/deployments/${deploymentId}/status`, { status })
}

export function listTenantModelGrants(tenantId) {
  return gatewayRequest.get(`/admin/tenants/${tenantId}/model-grants`)
}

export function grantModelToTenant(tenantId, data) {
  return gatewayRequest.post(`/admin/tenants/${tenantId}/model-grants`, data)
}

export function updateTenantModelGrantStatus(tenantId, modelId, status) {
  return gatewayRequest.patch(`/admin/tenants/${tenantId}/model-grants/${modelId}/status`, { status })
}

export function listTenantAPIKeys(tenantId) {
  return gatewayRequest.get(`/admin/tenants/${tenantId}/api-keys`)
}

export function createTenantAPIKey(tenantId, data) {
  return gatewayRequest.post(`/admin/tenants/${tenantId}/api-keys`, data)
}

export function updateTenantAPIKey(tenantId, apiKeyId, data) {
  return gatewayRequest.patch(`/admin/tenants/${tenantId}/api-keys/${apiKeyId}`, data)
}

export function updateTenantAPIKeyStatus(tenantId, apiKeyId, status) {
  return gatewayRequest.patch(`/admin/tenants/${tenantId}/api-keys/${apiKeyId}/status`, { status })
}

export function listUserModelGrants(tenantId, userId) {
  return gatewayRequest.get(`/admin/tenants/${tenantId}/users/${userId}/model-grants`)
}

export function grantModelToUser(tenantId, userId, data) {
  return gatewayRequest.post(`/admin/tenants/${tenantId}/users/${userId}/model-grants`, data)
}

export function updateUserModelGrantStatus(tenantId, userId, modelId, status) {
  return gatewayRequest.patch(`/admin/tenants/${tenantId}/users/${userId}/model-grants/${modelId}/status`, { status })
}

export function listUserAPIKeys(tenantId, userId) {
  return gatewayRequest.get(`/admin/tenants/${tenantId}/users/${userId}/api-keys`)
}

export function createUserAPIKey(tenantId, userId, data) {
  return gatewayRequest.post(`/admin/tenants/${tenantId}/users/${userId}/api-keys`, data)
}

export function updateUserAPIKey(tenantId, userId, apiKeyId, data) {
  return gatewayRequest.patch(`/admin/tenants/${tenantId}/users/${userId}/api-keys/${apiKeyId}`, data)
}

export function updateUserAPIKeyStatus(tenantId, userId, apiKeyId, status) {
  return gatewayRequest.patch(`/admin/tenants/${tenantId}/users/${userId}/api-keys/${apiKeyId}/status`, { status })
}

export function nowTimestamp() {
  return Date.now()
}

export function formatTimestamp(value) {
  if (!value) return ''
  return new Date(value).toLocaleString('zh-CN', { hour12: false })
}

export function formatCredits(value) {
  return (Number(value) || 0).toLocaleString('zh-CN')
}

export function listUsageLogs(params) {
  return gatewayRequest.get('/admin/usage-logs', { params })
}

export function listUsageSummary(params) {
  return gatewayRequest.get('/admin/usage-summary', { params })
}

export function listUsageUnitSummary(params) {
  return gatewayRequest.get('/admin/usage-unit-summary', { params })
}

export function getDashboardSummary(params) {
  return gatewayRequest.get('/admin/dashboard/summary', { params })
}

export function listDashboardTopModels(params) {
  return gatewayRequest.get('/admin/dashboard/top-models', { params })
}

export function listDashboardTopTenants(params) {
  return gatewayRequest.get('/admin/dashboard/top-tenants', { params })
}

export function listDashboardRecentErrors(params) {
  return gatewayRequest.get('/admin/dashboard/recent-errors', { params })
}

export function listRuntimeLimitPolicies(params) {
  return gatewayRequest.get('/admin/limit-policies', { params })
}

export function createRuntimeLimitPolicy(data) {
  return gatewayRequest.post('/admin/limit-policies', data)
}

export function updateRuntimeLimitPolicy(policyId, data) {
  return gatewayRequest.patch(`/admin/limit-policies/${policyId}`, data)
}

export function updateRuntimeLimitPolicyStatus(policyId, status) {
  return gatewayRequest.patch(`/admin/limit-policies/${policyId}/status`, { status })
}

export function listGatewayAuditLogs(params) {
  return gatewayRequest.get('/admin/audit-logs', { params })
}
