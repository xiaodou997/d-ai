import axios from 'axios'
import { ElMessage } from 'element-plus'
import { useAuthStore } from '@/stores/auth'
import router from '@/router'

const gatewayRequest = axios.create({
  baseURL: import.meta.env.VITE_AI_GATEWAY_BASE_URL || '',
  timeout: 30000,
  headers: {
    'Content-Type': 'application/json'
  }
})

// 刷新锁，防止并发请求时多次刷新 Token
let isRefreshing = false
// 刷新队列
let refreshSubscribers = []

const redirectToLogin = () => {
  if (router.currentRoute.value.path !== '/login') {
    router.replace('/login')
  }
}

// 添加到刷新队列
const subscribeTokenRefresh = (resolve, reject) => {
  refreshSubscribers.push({ resolve, reject })
}

// 执行刷新队列
const onRefreshed = (accessToken) => {
  refreshSubscribers.forEach((callback) => {
    callback.resolve(accessToken)
  })
  refreshSubscribers = []
}

// 刷新队列失败
const onRefreshFailed = (error) => {
  refreshSubscribers.forEach((callback) => {
    callback.reject(error)
  })
  refreshSubscribers = []
}

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
  async (error) => {
    // 处理 HTTP 错误
    if (error.response) {
      const { status, data } = error.response

      if (status === 401) {
        const authStore = useAuthStore()
        const refreshToken = localStorage.getItem('admin_refreshToken')

        // 没有 refresh token，直接跳转登录
        if (!refreshToken) {
          ElMessage.error('登录已过期，请重新登录')
          authStore.clearState()
          redirectToLogin()
          return Promise.reject(error)
        }

        // 如果正在刷新，将请求加入队列
        if (isRefreshing) {
          return new Promise((resolve, reject) => {
            subscribeTokenRefresh((newToken) => {
              const config = error.config
              config.headers['Authorization'] = `Bearer ${newToken}`
              resolve(gatewayRequest(config))
            }, reject)
          })
        }

        // 开始刷新 token
        isRefreshing = true
        try {
          const newToken = await authStore.refreshAccessToken()
          isRefreshing = false
          onRefreshed(newToken.accessToken)

          // 重试之前的请求
          const config = error.config
          config.headers['Authorization'] = `Bearer ${newToken.accessToken}`
          return gatewayRequest(config)
        } catch (refreshError) {
          isRefreshing = false
          onRefreshFailed(refreshError)

          // 刷新失败，清理状态并跳转
          ElMessage.error('登录已过期，请重新登录')
          authStore.clearState()
          authStore.stopAutoRefresh()
          redirectToLogin()

          return Promise.reject(refreshError)
        }
      }

      // 其他错误
      const message = data?.error || data?.message || 'AI Gateway 请求失败'
      ElMessage.error(message)
    } else if (error.request) {
      ElMessage.error('网络错误，请检查网络连接')
    } else {
      ElMessage.error('请求配置错误')
    }

    return Promise.reject(error)
  }
)

export const statusOptions = [
  { label: '启用', value: 'active' },
  { label: '停用', value: 'inactive' },
  { label: '禁用', value: 'disabled' }
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
  { label: '语音合成 TTS', value: 'audio_tts' },
  { label: '语音识别 STT', value: 'audio_stt' },
  { label: '重排', value: 'rerank' }
]

// ============================================================================
// Provider APIs
// ============================================================================

export function listProviders() {
  return gatewayRequest.get('/api/v1/providers')
}

export function createProvider(data) {
  return gatewayRequest.post('/api/v1/providers', data)
}

export function updateProvider(providerId, data) {
  return gatewayRequest.patch(`/api/v1/providers/${providerId}`, data)
}

export function updateProviderStatus(providerId, status) {
  return gatewayRequest.patch(`/api/v1/providers/${providerId}/status`, { status })
}

export function listProviderEndpoints(providerId) {
  return gatewayRequest.get(`/api/v1/providers/${providerId}/endpoints`)
}

export function createProviderEndpoint(providerId, data) {
  return gatewayRequest.post(`/api/v1/providers/${providerId}/endpoints`, data)
}

export function updateProviderEndpoint(providerId, endpointId, data) {
  return gatewayRequest.patch(`/api/v1/providers/${providerId}/endpoints/${endpointId}`, data)
}

export function updateProviderEndpointStatus(providerId, endpointId, status) {
  return gatewayRequest.patch(`/api/v1/providers/${providerId}/endpoints/${endpointId}/status`, { status })
}

// ============================================================================
// Upstream Deployment APIs (替代旧的 Model Deployment)
// ============================================================================

export function listUpstreamDeployments(params) {
  return gatewayRequest.get('/api/v1/upstream-deployments', { params })
}

export function createUpstreamDeployment(data) {
  return gatewayRequest.post('/api/v1/upstream-deployments', data)
}

export function getUpstreamDeployment(deploymentId) {
  return gatewayRequest.get(`/api/v1/upstream-deployments/${deploymentId}`)
}

export function updateUpstreamDeployment(deploymentId, data) {
  return gatewayRequest.patch(`/api/v1/upstream-deployments/${deploymentId}`, data)
}

export function updateUpstreamDeploymentStatus(deploymentId, status) {
  return gatewayRequest.patch(`/api/v1/upstream-deployments/${deploymentId}/status`, { status })
}

export function checkUpstreamDeploymentHealth(deploymentId) {
  return gatewayRequest.post(`/api/v1/upstream-deployments/${deploymentId}/health-check`)
}

// ============================================================================
// Upstream Deployment Cost Price APIs
// ============================================================================

export function listUpstreamDeploymentCostPrices(deploymentId) {
  return gatewayRequest.get(`/api/v1/upstream-deployments/${deploymentId}/cost-prices`)
}

export function createUpstreamDeploymentCostPrice(deploymentId, data) {
  return gatewayRequest.post(`/api/v1/upstream-deployments/${deploymentId}/cost-prices`, data)
}

export function updateUpstreamDeploymentCostPrice(deploymentId, priceId, data) {
  return gatewayRequest.patch(`/api/v1/upstream-deployments/${deploymentId}/cost-prices/${priceId}`, data)
}

export function updateUpstreamDeploymentCostPriceStatus(deploymentId, priceId, status) {
  return gatewayRequest.patch(`/api/v1/upstream-deployments/${deploymentId}/cost-prices/${priceId}/status`, { status })
}

// ============================================================================
// Model APIs
// ============================================================================

export function listModels() {
  return gatewayRequest.get('/api/v1/models')
}

export function createModel(data) {
  return gatewayRequest.post('/api/v1/models', data)
}

export function updateModel(modelId, data) {
  return gatewayRequest.patch(`/api/v1/models/${modelId}`, data)
}

export function updateModelStatus(modelId, status) {
  return gatewayRequest.patch(`/api/v1/models/${modelId}/status`, { status })
}

// ============================================================================
// Model Price APIs (1:1 with model, upsert pattern)
// ============================================================================

export function getModelPrice(modelId) {
  return gatewayRequest.get(`/api/v1/models/${modelId}/price`)
}

export function upsertModelPrice(modelId, data) {
  return gatewayRequest.put(`/api/v1/models/${modelId}/price`, data)
}

// ============================================================================
// Tenant Model Price Override APIs
// ============================================================================

export function listTenantModelPriceOverrides(tenantId) {
  return gatewayRequest.get(`/api/v1/tenants/${tenantId}/model-price-overrides`)
}

export function getTenantModelPriceOverride(tenantId, modelId) {
  return gatewayRequest.get(`/api/v1/tenants/${tenantId}/model-price-overrides/${modelId}`)
}

export function upsertTenantModelPriceOverride(tenantId, modelId, data) {
  return gatewayRequest.put(`/api/v1/tenants/${tenantId}/model-price-overrides/${modelId}`, data)
}

export function deleteTenantModelPriceOverride(tenantId, modelId) {
  return gatewayRequest.delete(`/api/v1/tenants/${tenantId}/model-price-overrides/${modelId}`)
}

// ============================================================================
// Model Route APIs (新增)
// ============================================================================

export function listModelRoutes(modelId) {
  return gatewayRequest.get(`/api/v1/models/${modelId}/routes`)
}

export function createModelRoute(modelId, data) {
  return gatewayRequest.post(`/api/v1/models/${modelId}/routes`, data)
}

export function getModelRoute(modelId, routeId) {
  return gatewayRequest.get(`/api/v1/models/${modelId}/routes/${routeId}`)
}

export function updateModelRoute(modelId, routeId, data) {
  return gatewayRequest.patch(`/api/v1/models/${modelId}/routes/${routeId}`, data)
}

export function updateModelRouteStatus(modelId, routeId, status) {
  return gatewayRequest.patch(`/api/v1/models/${modelId}/routes/${routeId}/status`, { status })
}

export function deleteModelRoute(modelId, routeId) {
  return gatewayRequest.delete(`/api/v1/models/${modelId}/routes/${routeId}`)
}

// ============================================================================
// Tenant Model Grant APIs (只保留 tenant grant)
// ============================================================================

export function listTenantModelGrants(tenantId) {
  return gatewayRequest.get(`/api/v1/tenants/${tenantId}/model-grants`)
}

export function grantModelToTenant(tenantId, data) {
  return gatewayRequest.post(`/api/v1/tenants/${tenantId}/model-grants`, data)
}

export function updateTenantModelGrantStatus(tenantId, modelId, status) {
  return gatewayRequest.patch(`/api/v1/tenants/${tenantId}/model-grants/${modelId}/status`, { status })
}

// ============================================================================
// Tenant API Key APIs
// ============================================================================

export function listTenantAPIKeys(tenantId) {
  return gatewayRequest.get(`/api/v1/tenants/${tenantId}/api-keys`)
}

export function createTenantAPIKey(tenantId, data) {
  return gatewayRequest.post(`/api/v1/tenants/${tenantId}/api-keys`, data)
}

export function updateTenantAPIKey(tenantId, apiKeyId, data) {
  return gatewayRequest.patch(`/api/v1/tenants/${tenantId}/api-keys/${apiKeyId}`, data)
}

export function updateTenantAPIKeyStatus(tenantId, apiKeyId, status) {
  return gatewayRequest.patch(`/api/v1/tenants/${tenantId}/api-keys/${apiKeyId}/status`, { status })
}

// ============================================================================
// User API Key APIs (删除了 User Model Grant)
// ============================================================================

export function listUserAPIKeys(tenantId, userId) {
  return gatewayRequest.get(`/api/v1/tenants/${tenantId}/users/${userId}/api-keys`)
}

export function createUserAPIKey(tenantId, userId, data) {
  return gatewayRequest.post(`/api/v1/tenants/${tenantId}/users/${userId}/api-keys`, data)
}

export function updateUserAPIKey(tenantId, userId, apiKeyId, data) {
  return gatewayRequest.patch(`/api/v1/tenants/${tenantId}/users/${userId}/api-keys/${apiKeyId}`, data)
}

export function updateUserAPIKeyStatus(tenantId, userId, apiKeyId, status) {
  return gatewayRequest.patch(`/api/v1/tenants/${tenantId}/users/${userId}/api-keys/${apiKeyId}/status`, { status })
}

// ============================================================================
// Utility Functions
// ============================================================================

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

// ============================================================================
// Usage & Dashboard APIs
// ============================================================================

export function listUsageLogs(params) {
  return gatewayRequest.get('/api/v1/usage-logs', { params })
}

export function listUsageSummary(params) {
  return gatewayRequest.get('/api/v1/usage-summary', { params })
}

export function listUsageUnitSummary(params) {
  return gatewayRequest.get('/api/v1/usage-unit-summary', { params })
}

export function getDashboardSummary(params) {
  return gatewayRequest.get('/api/v1/dashboard/summary', { params })
}

export function listDashboardTopModels(params) {
  return gatewayRequest.get('/api/v1/dashboard/top-models', { params })
}

export function listDashboardTopTenants(params) {
  return gatewayRequest.get('/api/v1/dashboard/top-tenants', { params })
}

export function listDashboardRecentErrors(params) {
  return gatewayRequest.get('/api/v1/dashboard/recent-errors', { params })
}

// ============================================================================
// Runtime Limit Policy APIs
// ============================================================================

export function listRuntimeLimitPolicies(params) {
  return gatewayRequest.get('/api/v1/limit-policies', { params })
}

export function createRuntimeLimitPolicy(data) {
  return gatewayRequest.post('/api/v1/limit-policies', data)
}

export function updateRuntimeLimitPolicy(policyId, data) {
  return gatewayRequest.patch(`/api/v1/limit-policies/${policyId}`, data)
}

export function updateRuntimeLimitPolicyStatus(policyId, status) {
  return gatewayRequest.patch(`/api/v1/limit-policies/${policyId}/status`, { status })
}

// ============================================================================
// Audit Log APIs
// ============================================================================

export function listGatewayAuditLogs(params) {
  return gatewayRequest.get('/api/v1/audit-logs', { params })
}
