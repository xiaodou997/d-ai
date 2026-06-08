import request from '@/utils/request'

// ============================================
// 租户管理
// ============================================

/**
 * 查询租户列表（分页）
 * @param {Object} params - { keyword, status, page, size }
 * @returns {Promise<{records: Array, total: number, page: number, size: number}>}
 */
export function queryTenants(params) {
  return request({
    url: '/urm/v1/tenants',
    method: 'get',
    params
  })
}

/**
 * 获取租户详情
 * @param {string} id - 租户ID
 */
export function getTenant(id) {
  return request({
    url: `/urm/v1/tenants/${id}`,
    method: 'get'
  })
}

/**
 * 创建租户
 * @param {Object} data - 租户信息
 */
export function createTenant(data) {
  return request({
    url: '/urm/v1/tenants',
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
    url: `/urm/v1/tenants/${id}`,
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
    url: `/urm/v1/tenants/${id}`,
    method: 'delete'
  })
}

/**
 * 更新租户状态（启用/停用）
 * 级联操作：停用时级联停用组织用户、终端用户、冻结积分包；启用时级联恢复
 * @param {string} id - 租户ID
 * @param {string} status - 状态：active / disabled
 */
export function updateTenantStatus(id, status) {
  return request({
    url: `/urm/v1/tenants/${id}/status`,
    method: 'patch',
    data: { status }
  })
}

/**
 * 查询租户已授权的客户端服务列表
 * @param {string} tenantId - 租户ID
 */
export function listTenantClientServices(tenantId) {
  return request({
    url: `/urm/v1/tenants/${tenantId}/client-services`,
    method: 'get'
  })
}

// ============================================
// 租户组织用户（tenant-users）
// ============================================

/**
 * 查询租户组织用户列表
 * @param {Object} params - { tenantId, page, size }
 */
export function listTenantUsers(params) {
  return request({
    url: '/urm/v1/tenant-users',
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
    url: '/urm/v1/tenant-users',
    method: 'post',
    data
  })
}

/**
 * 更新租户组织用户状态（启用/停用）
 * @param {string} id - 用户ID
 * @param {string} status - 状态：active / disabled
 */
export function updateTenantUserStatus(id, status) {
  return request({
    url: `/urm/v1/tenant-users/${id}/status`,
    method: 'patch',
    data: { status }
  })
}

/**
 * 重置租户组织用户密码
 * @param {string} id - 用户ID
 */
export function resetTenantUserPassword(id) {
  return request({
    url: `/urm/v1/tenant-users/${id}/reset-password`,
    method: 'post'
  })
}
