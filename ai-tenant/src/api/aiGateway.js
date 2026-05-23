import request from '@/utils/request'

// ==================== 辅助函数 ====================

/**
 * 格式化积分显示
 */
// 后端 _credits 字段已统一是 float，最多 4 位小数；整数计数无小数自然展示。
export const formatCredits = (value) => {
  if (value === null || value === undefined) return '-'
  const n = Number(value) || 0
  return n.toLocaleString(undefined, { minimumFractionDigits: 0, maximumFractionDigits: 4 })
}

// ==================== 常量选项 ====================

export const statusOptions = [
  { label: '启用', value: 'active' },
  { label: '停用', value: 'inactive' },
  { label: '禁用', value: 'disabled' }
]

export const capabilityOptions = [
  { label: '对话', value: 'chat' },
  { label: '图像', value: 'image' },
  { label: '视频', value: 'video' },
  { label: '向量', value: 'embedding' },
  { label: '语音合成', value: 'audio_tts' },
  { label: '语音识别', value: 'audio_stt' },
  { label: '重排序', value: 'rerank' }
]

// ==================== 已授权模型 ====================

/**
 * 获取租户已授权模型列表（自动过滤为当前租户）
 */
export const listTenantModelGrants = () => {
  return request.get('/api/v1/tenant-model-grants')
}

// ==================== 租户售价（租户对用户的定价） ====================

/**
 * 获取租户售价列表（自动过滤为当前租户）
 */
export const listTenantUserPrices = () => {
  return request.get('/api/v1/user-prices')
}

/**
 * 获取单个模型的租户售价
 */
export const getTenantUserPrice = (modelId) => {
  return request.get(`/api/v1/tenants/me/user-prices/${modelId}`)
}

/**
 * 设置租户售价
 */
export const upsertTenantUserPrice = (modelId, data) => {
  return request.put(`/api/v1/tenants/me/user-prices/${modelId}`, data)
}

/**
 * 删除租户售价
 */
export const deleteTenantUserPrice = (modelId) => {
  return request.delete(`/api/v1/tenants/me/user-prices/${modelId}`)
}

// ==================== 平台定价（参考用） ====================

/**
 * 获取平台公价（用于参考）
 */
export const getModelPrice = (modelId) => {
  return request.get(`/api/v1/models/${modelId}/price`)
}

// ==================== 租户 API Key ====================

/**
 * 获取租户 API Key 列表（自动过滤为当前租户）
 */
export const listTenantAPIKeys = () => {
  return request.get('/api/v1/tenant-api-keys')
}

/**
 * 创建租户 API Key
 */
export const createTenantAPIKey = (data) => {
  return request.post('/api/v1/tenants/me/api-keys', data)
}

/**
 * 更新租户 API Key
 */
export const updateTenantAPIKey = (apiKeyId, data) => {
  return request.patch(`/api/v1/tenants/me/api-keys/${apiKeyId}`, data)
}

/**
 * 更新租户 API Key 状态
 */
export const updateTenantAPIKeyStatus = (apiKeyId, status) => {
  return request.patch(`/api/v1/tenants/me/api-keys/${apiKeyId}/status`, { status })
}

/**
 * 轮换租户 API Key（生成新 Key，旧 Key 立即失效）
 */
export const rotateTenantAPIKey = (apiKeyId) => {
  return request.post(`/api/v1/tenants/me/api-keys/${apiKeyId}/rotate`)
}

/**
 * 删除租户 API Key
 */
export const deleteTenantAPIKey = (apiKeyId) => {
  return request.delete(`/api/v1/tenants/me/api-keys/${apiKeyId}`)
}

// ==================== 用户消耗统计 ====================

/**
 * 获取使用日志（自动过滤为当前租户）
 * params: { limit, offset, user_id, model_code, request_status, date_from, date_to }
 * 返回: { total, stats, records }
 */
export const listUsageLogs = (params = {}) => {
  return request.get('/api/v1/usage-logs', { params: { limit: 20, offset: 0, ...params } })
}

/**
 * 获取使用汇总
 */
export const listUsageSummary = (params = {}) => {
  return request.get('/api/v1/usage-summary', { params })
}

// ==================== Dashboard ====================

/**
 * 获取 Dashboard 概览（自动过滤为当前租户维度）
 */
export const getDashboardSummary = (params = {}) => {
  return request.get('/api/v1/dashboard/summary', { params })
}

/**
 * 获取 Top 模型
 */
export const getDashboardTopModels = (params = {}) => {
  return request.get('/api/v1/dashboard/top-models', { params })
}

export const listDashboardRecentErrors = (params = {}) => {
  return request.get('/api/v1/dashboard/recent-errors', { params })
}