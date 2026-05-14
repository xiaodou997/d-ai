import request from '@/utils/request'

export function getAccountDetail(params) {
  return request({
    url: '/urm/v1/account/balance',
    method: 'get',
    params: { ...params, detail: true }
  })
}

export function queryTransactions(params) {
  return request({
    url: '/urm/v1/account/transactions',
    method: 'get',
    params
  })
}

export function queryTenants(params) {
  return request({
    url: '/urm/v1/tenants',
    method: 'get',
    params
  })
}

export function createTenant(data) {
  return request({
    url: '/urm/v1/tenants',
    method: 'post',
    data
  })
}

export function updateTenant(id, data) {
  return request({
    url: `/urm/v1/tenants/${id}`,
    method: 'put',
    data
  })
}

export function deleteTenant(id) {
  return request({
    url: `/urm/v1/tenants/${id}`,
    method: 'delete'
  })
}

export function updateTenantStatus(id, status) {
  return request({
    url: `/urm/v1/tenants/${id}/status`,
    method: 'patch',
    data: { status }
  })
}

export function queryUsers(params) {
  return request({
    url: '/urm/v1/users',
    method: 'get',
    params
  })
}

export function getAccountInfo(params) {
  return request({
    url: '/urm/v1/account/balance',
    method: 'get',
    params
  })
}

export function getRechargeRecords(params) {
  return request({
    url: '/urm/v1/account/recharge-records',
    method: 'get',
    params
  })
}

export function getGrantLogs(params) {
  return request({
    url: '/urm/v1/account/resource-grants',
    method: 'get',
    params
  })
}

export function listTenantUsers(params) {
  return request({
    url: '/urm/v1/tenant-users',
    method: 'get',
    params
  })
}

export function createTenantUser(data) {
  return request({
    url: '/urm/v1/tenant-users',
    method: 'post',
    data
  })
}

export function updateTenantUserStatus(id, status) {
  return request({
    url: `/urm/v1/tenant-users/${id}/status`,
    method: 'patch',
    data: { status }
  })
}

export function updateEndUserStatus(id, status) {
  return request({
    url: `/urm/v1/users/${id}/status`,
    method: 'patch',
    data: { status }
  })
}

export function resetTenantUserPassword(id) {
  return request({
    url: `/urm/v1/tenant-users/${id}/reset-password`,
    method: 'post'
  })
}

export function listTenantApps(tenantId) {
  return request({
    url: `/urm/v1/tenants/${tenantId}/client-services`,
    method: 'get'
  })
}
