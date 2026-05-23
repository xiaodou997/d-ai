import request from '@/utils/request'

// ==================== 辅助函数 ====================

/**
 * 格式化积分显示
 */
// 兼容整数计数和小数积分（后端 _credits float），浮点保留最多 4 位小数。
export const formatCredits = (value) => {
  if (value === null || value === undefined) return '-'
  const n = Number(value) || 0
  if (Number.isInteger(n)) return n.toLocaleString()
  return n.toLocaleString(undefined, { minimumFractionDigits: 0, maximumFractionDigits: 4 })
}

// ==================== 常量选项 ====================

export const statusOptions = [
  { label: '启用', value: 'active' },
  { label: '停用', value: 'inactive' },
  { label: '禁用', value: 'disabled' }
]

// ==================== 用户 API Key ====================

/**
 * 获取用户 API Key 列表（自动过滤为当前用户）
 */
export const listUserAPIKeys = () => {
  return request.get('/api/v1/user-api-keys')
}

/**
 * 创建用户 API Key
 */
export const createUserAPIKey = (data) => {
  return request.post('/api/v1/users/me/api-keys', data)
}

/**
 * 更新用户 API Key
 */
export const updateUserAPIKey = (apiKeyId, data) => {
  return request.patch(`/api/v1/users/me/api-keys/${apiKeyId}`, data)
}

/**
 * 更新用户 API Key 状态
 */
export const updateUserAPIKeyStatus = (apiKeyId, status) => {
  return request.patch(`/api/v1/users/me/api-keys/${apiKeyId}/status`, { status })
}

/**
 * 轮换用户 API Key（生成新 Key，旧 Key 立即失效）
 */
export const rotateUserAPIKey = (apiKeyId) => {
  return request.post(`/api/v1/users/me/api-keys/${apiKeyId}/rotate`)
}

/**
 * 删除用户 API Key
 */
export const deleteUserAPIKey = (apiKeyId) => {
  return request.delete(`/api/v1/users/me/api-keys/${apiKeyId}`)
}

// ==================== 用户消耗统计 ====================

/**
 * 获取个人使用日志（自动过滤为当前用户）
 */
export const listMyUsageLogs = (params = {}) => {
  return request.get('/api/v1/user-usage-logs', { params: { limit: 100, ...params } })
}

/**
 * 获取个人使用汇总
 */
export const getMyUsageSummary = (params = {}) => {
  return request.get('/api/v1/user-usage-summary', { params })
}

// ==================== 用户授权模型 ====================

/**
 * 获取用户授权模型列表（自动过滤为当前用户可用的模型）
 */
export const listUserModelGrants = () => {
  return request.get('/api/v1/user-model-grants')
}