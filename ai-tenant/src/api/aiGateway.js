import request from '@/utils/request'

// ==================== 辅助函数 ====================

/**
 * 格式化积分显示
 */
// formatCredits 用于实际消耗/统计结果，保留最多 4 位小数。
// 售价/配额这类配置值必须走 formatWholeCredits。
export const formatCredits = (value) => {
  if (value === null || value === undefined) return '-'
  const n = Number(value) || 0
  return n.toLocaleString(undefined, { minimumFractionDigits: 0, maximumFractionDigits: 4 })
}

// 售价/配额配置的展示单位是整数积分；小数只用于实际消耗/统计。
export const formatWholeCredits = (value) => {
  if (value === null || value === undefined) return '-'
  const n = Number(value) || 0
  return Math.round(n).toLocaleString(undefined, { maximumFractionDigits: 0 })
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

// ==================== Price Book 自助定价（租户自助） ====================

// 平台给本租户的售价绑定（只读）
export const getMySellBinding = () => {
  return request.get('/api/v1/tenants/me/sell-binding')
}

// 本租户给其用户的售价绑定（级联倍率 + 缓存开关）
export const getMyUserSellBinding = () => {
  return request.get('/api/v1/tenants/me/user-sell-binding')
}

export const upsertMyUserSellBinding = (data) => {
  return request.put('/api/v1/tenants/me/user-sell-binding', data)
}

export const deleteMyUserSellBinding = () => {
  return request.delete('/api/v1/tenants/me/user-sell-binding')
}

// 生效积分单价：scope=tenant（我的成本）| user（我卖给用户）
export const getMyEffectivePrices = (scope = 'tenant') => {
  return request.get('/api/v1/tenants/me/effective-prices', { params: { scope } })
}
