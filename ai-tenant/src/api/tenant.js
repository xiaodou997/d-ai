import request from '@/utils/request'

export const getStats = () => {
  return request.get('/urm/v1/account/stats')
}

export const getUsers = (params = {}) => {
  return request.get('/urm/v1/users', {
    params: { page: 1, size: 20, ...params }
  })
}

export const updateUserStatus = (id, status) =>
  request.patch(`/urm/v1/users/${id}/status`, { status })

export const resetUserPassword = (id) =>
  request.post(`/urm/v1/users/${id}/reset-password`)

export const rechargeUser = (data) =>
  request.post('/urm/v1/recharges', { ...data, packageType: 2 })

export const reverseRecharge = (orderId, data) =>
  request.post(`/urm/v1/recharges/${orderId}/reverse`, data)

export const getInviteCodes = (params = {}) => {
  return request.get('/urm/v1/invitations', {
    params: { page: 1, size: 20, ...params }
  })
}

export const createInviteCode = (data) => {
  return request.post('/urm/v1/invitations', data)
}

export const updateInviteCode = (id, data) => {
  return request.put(`/urm/v1/invitations/${id}`, data)
}

export const deleteInviteCode = (id) => {
  return request.delete(`/urm/v1/invitations/${id}`)
}

export const getAccountBalance = (detail = true) => {
  return request.get(`/urm/v1/account/balance?detail=${detail}`)
}

export const getTransactions = (params = {}) => {
  return request.get('/urm/v1/account/transactions', {
    params: { page: 1, size: 20, ...params }
  })
}

export const getRechargeRecords = (params = {}) => {
  return request.get('/urm/v1/account/recharge-records', {
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
