import request from '@/utils/request'

export const getStats = () => {
  return request.get('/urm/v1/account/stats')
}

export const getUsers = (params = {}) => {
  return request.get('/urm/v1/users', {
    params: { page: 1, size: 20, ...params }
  })
}

export const getAccountBalance = (detail = true) => {
  return request.get(`/urm/v1/account/balance?detail=${detail}`)
}

export const getTransactions = (params = {}) => {
  return request.get('/urm/v1/account/transactions', {
    params: { page: 1, size: 20, ...params }
  })
}

export const getUserRechargeRecords = (params = {}) => {
  return request.get('/urm/v1/account/recharge-records', {
    params: { page: 1, size: 20, rechargeType: 2, ...params }
  })
}

export const getTenantMe = () => {
  return request.get('/urm/v1/tenants/me')
}

export const getAnalyticsOverview = (params = {}) => {
  return request.get('/urm/v1/tenants/analytics/overview', { params })
}

export const getAppConsumption = (params = {}) => {
  return request.get('/urm/v1/tenants/analytics/app-consumption', { params })
}
