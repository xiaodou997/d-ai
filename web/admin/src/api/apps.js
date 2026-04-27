import request from '@/utils/request'

/**
 * 查询业务系统列表
 * @param {Object} params - 查询参数
 * @param {number} params.page - 页码
 * @param {number} params.size - 每页大小
 * @param {string} params.keyword - 搜索关键词
 * @returns {Promise<{records: Array, total: number, page: number, size: number}>}
 */
export function getApps(params) {
  return request({
    url: '/urm/v1/admin/apps',
    method: 'get',
    params
  })
}

/**
 * 创建业务系统
 * @param {Object} data - 创建数据
 * @param {string} data.appName - 系统名称
 * @param {string} data.description - 描述
 * @param {number} data.status - 状态（1=启用，0=禁用）
 * @returns {Promise<{id: number, appKey: string, appSecret: string, appName: string, description: string, status: number, createdAt: string}>}
 */
export function createApp(data) {
  return request({
    url: '/urm/v1/admin/apps',
    method: 'post',
    data
  })
}

/**
 * 更新业务系统
 * @param {number} id - 业务系统 ID
 * @param {Object} data - 更新数据
 * @param {string} data.appName - 系统名称
 * @param {string} data.description - 描述
 * @param {number} data.status - 状态
 * @returns {Promise<{id: number, appKey: string, appName: string, description: string, status: number, updatedAt: string}>}
 */
export function updateApp(id, data) {
  return request({
    url: `/urm/v1/admin/apps/${id}`,
    method: 'put',
    data
  })
}

/**
 * 删除业务系统
 * @param {number} id - 业务系统 ID
 * @returns {Promise<{success: boolean}>}
 */
export function deleteApp(id) {
  return request({
    url: `/urm/v1/admin/apps/${id}`,
    method: 'delete'
  })
}

/**
 * 重置 Secret
 * @param {number} id - 业务系统 ID
 * @returns {Promise<{id: number, appKey: string, appSecret: string, resetAt: string}>}
 */
export function resetSecret(id) {
  return request({
    url: `/urm/v1/admin/apps/${id}/reset-secret`,
    method: 'post'
  })
}
