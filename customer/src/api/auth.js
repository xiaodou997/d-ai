import request from '@/utils/request'

const CLIENT_ID = import.meta.env.VITE_SSO_CLIENT_ID || ''

export function exchangeCode(code, redirectUri) {
  return request({
    url: '/urm/oauth2/exchange',
    method: 'post',
    headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
    data: new URLSearchParams({ code, redirect_uri: redirectUri })
  })
}

export function getCurrentUser() {
  return request.get('/urm/oauth2/userinfo')
}

export function refreshAccessToken(refreshToken) {
  return request({
    url: '/urm/oauth2/token',
    method: 'post',
    headers: {
      'X-Client-Id': CLIENT_ID,
      'Content-Type': 'application/x-www-form-urlencoded'
    },
    data: new URLSearchParams({
      grant_type: 'refresh_token',
      refresh_token: refreshToken
    })
  })
}

export function logout() {
  return request.post('/urm/oauth2/revoke')
}

export function register(data) {
  return request.post('/urm/v1/user/register', data)
}

export function changePassword(oldPassword, newPassword) {
  return request.put('/urm/oauth2/password', { oldPassword, newPassword })
}
