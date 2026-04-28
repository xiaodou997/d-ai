import request from '@/utils/request'

/**
 * 用户登录（OAuth 2.0 统一登录）
 * @param {string} username
 * @param {string} password
 */
export function login(username, password) {
  return request({
    url: '/urm/oauth2/token',
    method: 'post',
    headers: {
      'X-Client-Type': 'customer',
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
 * 用户注册（公开接口）
 * @param {object} data
 * @param {string} data.username - 用户名（全局唯一）
 * @param {string} data.password - 密码
 * @param {string} data.inviteCode - 邀请码
 */
export function register(data) {
  return request.post('/urm/v1/user/register', data)
}
