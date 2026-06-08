import request from '@/utils/request'

const ACCOUNT_PATH = '/urm/v1/account'

// ============================================
// 账户查询
// ============================================

/**
 * 查询账户余额
 * 任何已认证用户均可查询自己有权限的账户
 * @param {Object} params - { accountType, accountId }
 * @param {boolean} [params.detail=false] - 是否返回详细信息（积分包明细等）
 * @returns {Promise<Object>}
 */
export function getAccountBalance(params) {
  return request({
    url: `${ACCOUNT_PATH}/balance`,
    method: 'get',
    params
  })
}

/**
 * 查询账户交易流水（分页）
 * @param {Object} params - { accountType, accountId, page, size }
 * @returns {Promise<{records: Array, total: number}>}
 */
export function getAccountTransactions(params) {
  return request({
    url: `${ACCOUNT_PATH}/transactions`,
    method: 'get',
    params
  })
}

/**
 * 查询充值记录（分页）
 * @param {Object} params - { rechargeType: 0全部/1租户/2用户, tenantName, page, size }
 * @returns {Promise<{records: Array, total: number}>}
 */
export function getRechargeRecords(params) {
  return request({
    url: `${ACCOUNT_PATH}/recharge-records`,
    method: 'get',
    params
  })
}

/**
 * 查询账户统计信息
 * @param {Object} params - { accountType, accountId }
 * @returns {Promise<Object>}
 */
export function getAccountStats(params) {
  return request({
    url: `${ACCOUNT_PATH}/stats`,
    method: 'get',
    params
  })
}

// ============================================
// 充值 / 退款
// ============================================

/**
 * 统一充值接口
 * 管理员：packageType=1 为租户充值，packageType=2 为用户充值
 * 租户用户：packageType 固定为 2（用户充值），customerId 必填
 * @param {Object} data - 充值请求数据
 * @param {number} data.packageType - 充值类型 (1-租户充值, 2-用户充值)
 * @param {string} data.customerId - 用户ID（用户充值时必填）
 * @param {string} data.tenantId - 租户ID（租户充值时管理员指定）
 * @param {number} data.paidAmount - 实付金额（分，即 CNY × 100）
 * @param {number} data.creditAmount - 到账积分
 * @param {string} data.reason - 充值原因
 * @param {number} data.expireTime - 过期时间（毫秒时间戳，可选）
 * @returns {Promise<Object>}
 */
export function recharge(data) {
  return request({
    url: '/urm/v1/recharges',
    method: 'post',
    data
  })
}

/**
 * 手动退款（单条）
 */
export function refund(data) {
  return request({ url: '/urm/v1/refunds', method: 'post', data })
}

/**
 * 手动确认已释放事件（released → succeeded）
 * @param {string} eventId
 * @param {Object} data - { actualTenantCredits, actualUserCredits, note }
 */
export function manualConfirmEvent(eventId, data) {
  return request({ url: `/urm/v1/billing/events/${eventId}/confirm`, method: 'post', data })
}

/**
 * 免除收费（released → cancelled）
 * @param {string} eventId
 * @param {Object} data - { note }
 */
export function adminDismissEvent(eventId, data) {
  return request({ url: `/urm/v1/billing/events/${eventId}/dismiss`, method: 'post', data })
}

/**
 * 批量手动确认（使用原冻结额）
 * @param {Object} data - { eventIds: string[], note: string }
 */
export function batchConfirmEvents(data) {
  return request({ url: '/urm/v1/billing/events/batch-confirm', method: 'post', data })
}

/**
 * 批量退款
 * @param {Object} data - { eventIds: string[], reason: string }
 */
export function batchRefundEvents(data) {
  return request({ url: '/urm/v1/billing/events/batch-refund', method: 'post', data })
}

/**
 * 充值撤销
 * @param {string} orderId - 充值单号
 * @param {Object} data - { reason }
 * @returns {Promise<Object>}
 */
export function reverseRecharge(orderId, data) {
  return request({
    url: `/urm/v1/recharges/${orderId}/reverse`,
    method: 'post',
    data
  })
}

// ============================================
// 操作审计
// ============================================

/**
 * 查询操作审计日志（分页，仅超级管理员）
 * @param {Object} params - { eventType, principalType, decision, clientId, userId, page, size }
 * @returns {Promise<{records: Array, total: number, page: number, size: number}>}
 */
export function getAuthAuditLogs(params) {
  return request({
    url: '/urm/v1/auth-audit-logs',
    method: 'get',
    params
  })
}
