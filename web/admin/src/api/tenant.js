import request from '@/utils/request'

/**
 * 查询租户列表（分页）
 * @param {Object} params - 查询参数
 */
export function queryTenants(params) {
  return request({
    url: '/urm/v1/admin/query/tenants',
    method: 'get',
    params
  })
}

/**
 * 创建租户
 * @param {Object} data - 租户信息
 */
export function createTenant(data) {
  return request({
    url: '/urm/v1/admin/query/tenants',
    method: 'post',
    data
  })
}

/**
 * 更新租户
 * @param {string} id - 租户ID
 * @param {Object} data - 更新内容
 */
export function updateTenant(id, data) {
  return request({
    url: `/urm/v1/admin/query/tenants/${id}`,
    method: 'put',
    data
  })
}

/**
 * 删除租户
 * @param {string} id - 租户ID
 */
export function deleteTenant(id) {
  return request({
    url: `/urm/v1/admin/query/tenants/${id}`,
    method: 'delete'
  })
}

/**
 * 停用租户（级联停用组织用户、终端用户、冻结账户）
 * @param {string} id - 租户ID
 */
export function disableTenant(id) {
  return request({
    url: `/urm/v1/admin/query/tenants/${id}/disable`,
    method: 'post'
  })
}

/**
 * 启用租户（级联恢复组织用户、终端用户、解冻账户）
 * @param {string} id - 租户ID
 */
export function enableTenant(id) {
  return request({
    url: `/urm/v1/admin/query/tenants/${id}/enable`,
    method: 'post'
  })
}

/**
 * 查询用户列表（分页）
 * @param {Object} params - 查询参数
 */
export function queryUsers(params) {
  return request({
    url: '/urm/v1/users',
    method: 'get',
    params
  })
}

/**
 * 查询租户组织用户列表
 * @param {Object} params - { tenantId, page, size }
 */
export function listTenantUsers(params) {
  return request({
    url: '/urm/v1/admin/query/tenant-users',
    method: 'get',
    params
  })
}

/**
 * 创建租户组织用户
 * @param {Object} data - { tenantId, username, email }
 */
export function createTenantUser(data) {
  return request({
    url: '/urm/v1/admin/query/tenant-users',
    method: 'post',
    data
  })
}

/**
 * 停用租户组织用户
 * @param {string} id - 用户ID
 */
export function disableTenantUser(id) {
  return request({
    url: `/urm/v1/admin/query/tenant-users/${id}/disable`,
    method: 'post'
  })
}

/**
 * 启用租户组织用户
 * @param {string} id - 用户ID
 */
export function enableTenantUser(id) {
  return request({
    url: `/urm/v1/admin/query/tenant-users/${id}/enable`,
    method: 'post'
  })
}

/**
 * 重置租户组织用户密码为 123456
 * @param {string} id - 用户ID
 */
export function resetTenantUserPassword(id) {
  return request({
    url: `/urm/v1/admin/query/tenant-users/${id}/reset-password`,
    method: 'post'
  })
}
