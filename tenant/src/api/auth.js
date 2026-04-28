import request from '@/utils/request'

/**
 * 租户登录（OAuth 2.0 统一登录）
 * @param {string} username - 用户名
 * @param {string} password - 密码
 */
export function login(username, password) {
  return request({
    url: '/urm/oauth2/token',
    method: 'post',
    headers: {
      'X-Client-Type': 'tenant',
      'Content-Type': 'application/x-www-form-urlencoded'
    },
    data: `grant_type=password&username=${encodeURIComponent(username)}&password=${encodeURIComponent(password)}`
  })
}

/**
 * 刷新 Token（OAuth 2.0 统一刷新）
 * @param {string} refreshToken - Refresh Token
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
 * 退出登录（OAuth 2.0 统一登出）
 */
export function logout() {
  return request({
    url: '/urm/oauth2/revoke',
    method: 'post'
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
