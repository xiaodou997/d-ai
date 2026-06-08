import request from '@/utils/request'

/**
 * 获取我的余额（含积分包明细）
 * @returns {Promise<Object>}
 */
export function getBalance() {
  return request({
    url: '/urm/v1/account/balance',
    method: 'get',
    params: { detail: true }
  })
}

/**
 * 获取我的积分流水（分页）
 * @param {Object} params - { page, size }
 * @returns {Promise<{records: Array, total: number}>}
 */
export function getTransactions(params) {
  return request({
    url: '/urm/v1/account/transactions',
    method: 'get',
    params
  })
}

/**
 * 获取我的充值记录（分页）
 * @param {Object} params - { page, size }
 * @returns {Promise<{records: Array, total: number}>}
 */
export function getRechargeRecords(params) {
  return request({
    url: '/urm/v1/account/recharge-records',
    method: 'get',
    params
  })
}
