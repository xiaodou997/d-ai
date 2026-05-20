import request from '@/utils/request'

const ACCOUNT_PATH = '/urm/v1/account'

export function getAccountBalance(params) {
  return request({
    url: `${ACCOUNT_PATH}/balance`,
    method: 'get',
    params
  })
}

export function getAccountTransactions(params) {
  return request({
    url: `${ACCOUNT_PATH}/transactions`,
    method: 'get',
    params
  })
}

export function getRechargeRecords(params) {
  return request({
    url: `${ACCOUNT_PATH}/recharge-records`,
    method: 'get',
    params
  })
}

export function recharge(data) {
  return request({
    url: '/urm/v1/recharges',
    method: 'post',
    data
  })
}

export function refund(data) {
  return request({
    url: '/urm/v1/refunds',
    method: 'post',
    data
  })
}

export function reverseRecharge(orderId, data) {
  return request({
    url: `/urm/v1/recharges/${orderId}/reverse`,
    method: 'post',
    data
  })
}

export function getAuthAuditLogs(params) {
  return request({
    url: '/urm/v1/auth-audit-logs',
    method: 'get',
    params
  })
}
