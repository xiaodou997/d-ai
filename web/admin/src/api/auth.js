import request from '@/utils/request'

/**
 * 管理员登录（OAuth 2.0 统一登录）
 * @param {string} username - 用户名
 * @param {string} password - 密码
 * @returns {Promise<{accessToken: string, refreshToken: string, tokenType: string, expiresIn: number}>}
 */
export function login(username, password) {
  return request({
    url: '/urm/oauth2/token',
    method: 'post',
    headers: {
      'X-Client-Type': 'admin',
      'Content-Type': 'application/x-www-form-urlencoded'
    },
    data: `grant_type=password&username=${encodeURIComponent(username)}&password=${encodeURIComponent(password)}`
  })
}

/**
 * 刷新 Token（OAuth 2.0 统一刷新）
 * 使用 Refresh Token 获取新的 Access Token
 * @param {string} refreshToken - Refresh Token
 * @returns {Promise<{accessToken: string, refreshToken: string, tokenType: string, expiresIn: number}>}
 */
export function refreshToken(refreshToken) {
  return request({
    url: '/urm/oauth2/token',
    method: 'post',
    headers: {
      'Content-Type': 'application/x-www-form-urlencoded'
    },
    data: `grant_type=refresh_token&refresh_token=${encodeURIComponent(refreshToken)}`
  })
}

/**
 * 用户登出（OAuth 2.0 统一登出）
 * 将当前 Token 加入黑名单
 * @returns {Promise<{success: boolean}>}
 */
export function logout() {
  return request({
    url: '/urm/oauth2/revoke',
    method: 'post'
  })
}

/**
 * 强制登出所有设备
 * 将用户的所有 Token 加入黑名单
 * @param {string} userId - 用户 ID
 * @returns {Promise<{success: boolean, message: string}>}
 */
export function logoutAll(userId) {
  return request({
    url: `/urm/v1/auth/logout-all/${userId}`,
    method: 'post'
  })
}

/**
 * Token 内省
 * 验证 Token 有效性并获取用户信息
 * @param {string} token - JWT Token
 * @returns {Promise<{active: boolean, userId: string, username: string, tenantId: string, userType: number, tokenId: string, issuer: string, expiresAt: number, reason: string}>}
 */
export function introspect(token) {
  return request({
    url: '/urm/v1/auth/introspect',
    method: 'post',
    data: {
      token
    }
  })
}

/**
 * 获取当前用户信息
 * @returns {Promise<{userId: string, username: string, tenantId: string, userType: number, userTypeDisplay: string, email: string, status: number, statusDisplay: string, lastLoginTime: number}>}
 */
export function getCurrentUser() {
  return request({
    url: '/urm/v1/auth/me',
    method: 'get'
  })
}

/**
 * 修改密码（OAuth 2.0 统一接口）
 * @param {string} oldPassword - 旧密码
 * @param {string} newPassword - 新密码（至少6位）
 */
export function changePassword(oldPassword, newPassword) {
  return request({
    url: '/urm/oauth2/password',
    method: 'put',
    data: { oldPassword, newPassword }
  })
}

/**
 * 获取 JWT 密钥列表（超级管理员）
 * @returns {Promise<{keys: Array, total: number}>}
 */
export function listJwtKeys() {
  return request({
    url: '/urm/v1/admin/system/jwt-keys',
    method: 'get'
  })
}

/**
 * 轮换 JWT 密钥（超级管理员）
 * 生成新密钥，旧密钥进入 24 小时宽限期后自动退役
 * @returns {Promise<{message: string}>}
 */
export function rotateJwtKey() {
  return request({
    url: '/urm/v1/admin/system/rotate-jwt-key',
    method: 'post'
  })
}
