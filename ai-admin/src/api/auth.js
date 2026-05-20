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
  return request({
    url: '/urm/oauth2/revoke',
    method: 'post'
  })
}

export function getCurrentUser() {
  return request({
    url: '/urm/oauth2/userinfo',
    method: 'get'
  })
}

export function changePassword(oldPassword, newPassword) {
  return request({
    url: '/urm/oauth2/password',
    method: 'put',
    data: { oldPassword, newPassword }
  })
}
