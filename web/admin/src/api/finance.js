import request from '@/utils/request'

/**
 * 查询充值记录（分页）
 * @param {Object} params - { rechargeType: 0全部/1租户/2用户, tenantName, page, size }
 */
export function getRechargeRecords(params) {
  return request({
    url: '/urm/v1/account/recharge-records',
    method: 'get',
    params
  })
}

/**
 * 查询交易流水（分页）
 * @param {Object} params - { tenantName, username, appName, page, size }
 */
export function getTransactions(params) {
  return request({
    url: '/urm/v1/account/transactions',
    method: 'get',
    params
  })
}

/**
 * 查询租户补发记录（分页）
 */
export function getTenantGrantLogs(params) {
  return request({
    url: '/urm/v1/account/resource-grants',
    method: 'get',
    params
  })
}

/**
 * 查询用户补发记录（分页）
 */
export function getUserGrantLogs(params) {
  return request({
    url: '/urm/v1/account/resource-grants',
    method: 'get',
    params
  })
}
