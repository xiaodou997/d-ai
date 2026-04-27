import request from '@/utils/request'

/**
 * 手动充值
 * @param {Object} data - 充值请求数据
 * @param {number} data.accountType - 账户类型 (1-租户, 2-用户)
 * @param {string} data.accountId - 账户ID
 * @param {number} data.paidAmount - 实付金额（分，即 CNY × 100）
 * @param {number} data.creditAmount - 到账积分
 * @param {string} data.assetCode - 资产代码，默认CREDIT_POINT
 * @param {string} data.reason - 充值原因
 * @param {string} data.operatorId - 操作员ID
 * @returns {Promise<{paidAmount: number, creditAmount: number, beforeCredits: number, afterCredits: number, operationTime: string, auditLogId: number}>}
 */
export function recharge(data) {
  return request({
    url: '/urm/v1/admin/operations/recharge',
    method: 'post',
    data
  })
}

/**
 * 手动退款
 * @param {Object} data - 退款请求数据
 * @param {string} data.transactionId - 原交易ID
 * @param {number} data.refundAmount - 退款金额（分）
 * @param {string} data.reason - 退款原因
 * @param {string} data.operatorId - 操作员ID
 * @returns {Promise<{originalTransactionId: string, refundAmount: number, newBalance: number, operationTime: string, auditLogId: number}>}
 */
export function refund(data) {
  return request({
    url: '/urm/v1/admin/operations/refund',
    method: 'post',
    data
  })
}

/**
 * 冻结账户
 * @param {Object} data - 冻结请求数据
 * @param {number} data.accountType - 账户类型 (1-租户, 2-用户)
 * @param {string} data.accountId - 账户ID
 * @param {string} data.reason - 冻结原因
 * @param {string} data.operatorId - 操作员ID
 * @returns {Promise<{accountType: number, accountId: string, newStatus: number, operationTime: string, auditLogId: number}>}
 */
export function freezeAccount(data) {
  return request({
    url: '/urm/v1/admin/operations/freeze-account',
    method: 'post',
    data
  })
}

/**
 * 解冻账户
 * @param {Object} data - 解冻请求数据
 * @param {number} data.accountType - 账户类型 (1-租户, 2-用户)
 * @param {string} data.accountId - 账户ID
 * @param {string} data.operatorId - 操作员ID
 * @returns {Promise<{accountType: number, accountId: string, newStatus: number, operationTime: string, auditLogId: number}>}
 */
export function unfreezeAccount(data) {
  return request({
    url: '/urm/v1/admin/operations/unfreeze-account',
    method: 'post',
    data
  })
}

/**
 * 查询账户信息（用于充值/冻结前确认）
 * @param {number} accountType - 账户类型
 * @param {string} accountId - 账户ID
 * @returns {Promise<{accountId: string, credits: number, status: number}>}
 */
export function getAccountInfo(accountType, accountId) {
  return request({
    url: '/urm/v1/account/balance',
    method: 'get',
    params: {
      accountType,
      accountId
    }
  })
}

/**
 * 查询操作审计日志（分页）
 * @param {Object} params - 查询参数
 * @param {string} params.operatorId - 操作员ID（可选）
 * @param {string} params.operationType - 操作类型（可选）：RECHARGE/REFUND/FREEZE/UNFREEZE
 * @param {string} params.targetAccountId - 目标账户ID（可选）
 * @param {string} params.startTime - 开始时间（可选）
 * @param {string} params.endTime - 结束时间（可选）
 * @param {number} params.page - 页码
 * @param {number} params.size - 每页大小
 * @returns {Promise<{records: Array, total: number}>}
 */
export function getOperationLogs(params) {
  return request({
    url: '/urm/v1/admin/operations/logs',
    method: 'get',
    params
  })
}
