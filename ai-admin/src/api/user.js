import request from '@/utils/request'

const BASE_PATH = '/urm/v1/users'

/**
 * 查询终端用户列表（分页）
 * 系统管理员 + 租户用户均可操作，由 handler 内部校验归属
 * @param {Object} params - { tenantId, userId, username, status, page, size }
 * @returns {Promise<{records: Array, total: number, page: number, size: number}>}
 */
export function listEndUsers(params) {
  return request({
    url: BASE_PATH,
    method: 'get',
    params
  })
}

/**
 * 更新终端用户状态（启用/停用）
 * @param {string} id - 用户ID
 * @param {string} status - 状态：active / disabled
 */
export function updateEndUserStatus(id, status) {
  return request({
    url: `${BASE_PATH}/${id}/status`,
    method: 'patch',
    data: { status }
  })
}

/**
 * 重置终端用户密码
 * @param {string} id - 用户ID
 */
export function resetEndUserPassword(id) {
  return request({
    url: `${BASE_PATH}/${id}/reset-password`,
    method: 'post'
  })
}
