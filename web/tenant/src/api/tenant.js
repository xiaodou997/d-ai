import request from '@/utils/request'

// ==================== 统计 ====================

/**
 * 获取租户统计数据
 * 返回：{ endUserCount, inviteCodeCount, totalDeduction }
 */
export const getStats = () => {
  return request.get('/urm/v1/account/stats')
}

// ==================== 用户管理 ====================

/**
 * 获取终端用户列表
 * @param {object} params - { keyword, page, size }
 */
export const getUsers = (params = {}) => {
  return request.get('/urm/v1/users', { params: { page: 1, size: 20, ...params } })
}

/** 停用终端用户 */
export const disableUser = (id) =>
  request.post(`/urm/v1/users/${id}/disable`)

/** 启用终端用户 */
export const enableUser = (id) =>
  request.post(`/urm/v1/users/${id}/enable`)

/** 重置终端用户密码为 123456 */
export const resetUserPassword = (id) =>
  request.post(`/urm/v1/users/${id}/reset-password`)

/**
 * 给终端用户充值积分
 * @param {object} data - { userId, paidAmount, creditAmount, reason?, expireTime? }
 */
export const rechargeUser = (data) =>
  request.post('/urm/v1/tenant/operations/recharge-user', data)

// ==================== 邀请码 ====================

/**
 * 获取邀请码列表
 * @param {object} params - { page, size }
 */
export const getInviteCodes = (params = {}) => {
  return request.get('/urm/v1/tenant/invitation-codes', { params: { page: 1, size: 20, ...params } })
}

/**
 * 创建邀请码
 * @param {object} data - { description, max_uses, expire_time }
 */
export const createInviteCode = (data) => {
  return request.post('/urm/v1/tenant/invitation-codes', data)
}

/**
 * 更新邀请码
 * @param {string|number} id
 * @param {object} data - { status, description }
 */
export const updateInviteCode = (id, data) => {
  return request.put(`/urm/v1/tenant/invitation-codes/${id}`, data)
}

/**
 * 删除邀请码
 * @param {string|number} id
 */
export const deleteInviteCode = (id) => {
  return request.delete(`/urm/v1/tenant/invitation-codes/${id}`)
}

// ==================== 统一账户接口 ====================

/**
 * 获取我的账户余额（总积分、冻结、可用、积分包列表）
 * @param {boolean} detail - 是否返回积分包详情
 */
export const getAccountBalance = (detail = true) => {
  return request.get(`/urm/v1/account/balance?detail=${detail}`)
}

/**
 * 获取我的积分流水
 * @param {object} params - { page, size }
 */
export const getTransactions = (params = {}) => {
  return request.get('/urm/v1/account/transactions', { params: { page: 1, size: 20, ...params } })
}

/**
 * 获取我的充值记录
 * @param {object} params - { page, size }
 */
export const getRechargeRecords = (params = {}) => {
  return request.get('/urm/v1/account/recharge-records', { params: { page: 1, size: 20, ...params } })
}

// ==================== 用户财务中心 ====================

/**
 * 获取本租户所有用户的充值记录
 * @param {object} params - { page, size }
 */
export const getUserRechargeRecords = (params = {}) => {
  return request.get('/urm/v1/account/recharge-records', { params: { page: 1, size: 20, rechargeType: 2, ...params } })
}

// ==================== 数据分析 ====================

/**
 * 获取扩展的租户统计数据
 * 返回：{ endUserCount, inviteCodeCount, totalDeduction, userTotalCredits, activeUserCount }
 */
export const getAnalyticsOverview = () => {
  return request.get('/urm/v1/tenant/analytics/overview')
}

/**
 * 获取按 APP 消耗分布（用于饼状图）
 * @param {number} days - 时间范围天数，默认 30
 * 返回：[{ appKey, appName, credits, percentage }]
 */
export const getAppConsumption = (days = 30) => {
  return request.get(`/urm/v1/tenant/analytics/app-consumption?days=${days}`)
}
