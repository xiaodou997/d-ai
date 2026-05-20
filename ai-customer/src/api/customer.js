import request from '@/utils/request'

export const getBalance = () => {
  return request.get('/urm/v1/account/balance', { params: { detail: true } })
}

export const getTransactions = (params) => {
  return request.get('/urm/v1/account/transactions', { params })
}

export const getRechargeRecords = (params) => {
  return request.get('/urm/v1/account/recharge-records', { params })
}
