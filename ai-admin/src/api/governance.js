import request from '@/utils/request'

const BASE_PATH = '/urm/v1/governance'

// ============================================
// IP 白名单
// ============================================

/**
 * 查询 IP 白名单列表
 */
export function listIPWhitelist() {
  return request({ url: `${BASE_PATH}/ip-whitelist`, method: 'get' })
}

/**
 * 创建 IP 白名单
 * @param {Object} data - { ip, description }
 */
export function createIPWhitelist(data) {
  return request({ url: `${BASE_PATH}/ip-whitelist`, method: 'post', data })
}

/**
 * 更新 IP 白名单
 * @param {string|number} id - 白名单记录ID
 * @param {Object} data - { ip, description }
 */
export function updateIPWhitelist(id, data) {
  return request({ url: `${BASE_PATH}/ip-whitelist/${id}`, method: 'put', data })
}

/**
 * 删除 IP 白名单
 * @param {string|number} id - 白名单记录ID
 */
export function deleteIPWhitelist(id) {
  return request({ url: `${BASE_PATH}/ip-whitelist/${id}`, method: 'delete' })
}

// ============================================
// 客户端（Client）管理
// ============================================

/**
 * 查询客户端列表
 * @returns {Promise<Array>}
 */
export function listClients() {
  return request({
    url: `${BASE_PATH}/clients`,
    method: 'get'
  })
}

/**
 * 获取单个客户端详情
 * @param {string} clientId - 客户端 ID
 * @returns {Promise<Object>}
 */
export function getClient(clientId) {
  return request({
    url: `${BASE_PATH}/clients/${clientId}`,
    method: 'get'
  })
}

/**
 * 创建客户端
 * @param {Object} data - { clientId, displayName, description }
 * @returns {Promise<Object>}
 */
export function createClient(data) {
  return request({
    url: `${BASE_PATH}/clients`,
    method: 'post',
    data
  })
}

/**
 * 更新客户端
 * @param {string} clientId - 客户端 ID
 * @param {Object} data - { displayName, description }
 * @returns {Promise<Object>}
 */
export function updateClient(clientId, data) {
  return request({
    url: `${BASE_PATH}/clients/${clientId}`,
    method: 'put',
    data
  })
}

/**
 * 删除客户端
 * @param {string} clientId - 客户端 ID
 * @returns {Promise<Object>}
 */
export function deleteClient(clientId) {
  return request({
    url: `${BASE_PATH}/clients/${clientId}`,
    method: 'delete'
  })
}

/**
 * 刷新客户端密钥（管理后台调用）
 * @param {string} clientId - 客户端 ID
 * @returns {Promise<{clientId: string, clientSecret: string}>}
 */
export function refreshClientSecret(clientId) {
  return request({
    url: `${BASE_PATH}/clients/${clientId}/refresh-secret`,
    method: 'post'
  })
}

// ============================================
// Scope 管理
// ============================================

/**
 * 查询 Scope 列表
 * @param {string} [ownerClientId] - 按所属客户端筛选（可选）
 * @returns {Promise<Array>}
 */
export function listScopes(ownerClientId) {
  const params = ownerClientId ? { ownerClientId } : {}
  return request({
    url: `${BASE_PATH}/scopes`,
    method: 'get',
    params
  })
}

/**
 * 删除单个 Scope
 * @param {string} clientId - 客户端 ID
 * @param {string} scopeName - Scope 名称
 */
export function deleteScope(clientId, scopeName) {
  return request({
    url: `${BASE_PATH}/scopes/${clientId}/${scopeName}`,
    method: 'delete'
  })
}

/**
 * 删除客户端下所有 Scope
 * @param {string} clientId - 客户端 ID
 */
export function deleteAllScopes(clientId) {
  return request({
    url: `${BASE_PATH}/scopes/${clientId}`,
    method: 'delete'
  })
}

// ============================================
// Scope 授权（Grant）
// ============================================

/**
 * 查询 Scope 授权列表
 * @returns {Promise<Array>}
 */
export function listScopeGrants() {
  return request({
    url: `${BASE_PATH}/scope-grants`,
    method: 'get'
  })
}

/**
 * 创建 Scope 授权
 * @param {Object} data - { sourceClientId, ownerClientId, scopeName }
 */
export function createScopeGrant(data) {
  return request({
    url: `${BASE_PATH}/scope-grants`,
    method: 'post',
    data
  })
}

/**
 * 删除 Scope 授权
 * @param {string} sourceClientId - 源客户端 ID
 * @param {string} ownerClientId - 所属客户端 ID
 * @param {string} scopeName - Scope 名称
 */
export function deleteScopeGrant(sourceClientId, ownerClientId, scopeName) {
  return request({
    url: `${BASE_PATH}/scope-grants/${sourceClientId}/${ownerClientId}/${scopeName}`,
    method: 'delete'
  })
}
