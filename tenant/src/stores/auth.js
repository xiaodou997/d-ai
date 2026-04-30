import { defineStore } from 'pinia'
import { ref } from 'vue'
import {
  login as loginApi,
  refreshToken as refreshApi,
  logout as logoutApi
} from '@/api/auth'
import request from '@/utils/request'
import router from '@/router'

export const useAuthStore = defineStore('tenantAuth', () => {
  // State
  const accessToken = ref(localStorage.getItem('tenant_accessToken') || '')
  const refreshToken = ref(localStorage.getItem('tenant_refreshToken') || '')
  const userId = ref(localStorage.getItem('tenant_userId') || '')
  const username = ref(localStorage.getItem('tenant_username') || '')
  const tenantId = ref(localStorage.getItem('tenant_tenantId') || '')
  const tenantName = ref(localStorage.getItem('tenant_tenantName') || '')
  const expiresIn = ref(
    parseInt(localStorage.getItem('tenant_expiresIn') || '7200')
  )

  // 刷新定时器
  let refreshTimer = null

  const redirectToLogin = async () => {
    if (router.currentRoute.value.path !== '/login') {
      await router.replace('/login')
    }
  }

  // Actions
  const login = async (usernameVal, password) => {
    const response = await loginApi(usernameVal, password)
    accessToken.value = response.accessToken || ''
    refreshToken.value = response.refreshToken || ''
    expiresIn.value = response.expiresIn || 7200

    // 获取用户信息
    const userInfo = await request.get('/urm/oauth2/userinfo')
    userId.value = userInfo.sub
    username.value = userInfo.username
    tenantId.value = userInfo.tenantId || ''
    tenantName.value = userInfo.tenantName || ''

    saveToLocalStorage()
    startAutoRefresh()
    router.push('/dashboard')
    return { ...response, userId: userInfo.sub, username: userInfo.username }
  }

  const loginWithSSO = async (code, redirectUri) => {
    const tokenData = await request.get('/api/auth/callback', { params: { code, redirect_uri: redirectUri } })
    accessToken.value = tokenData.accessToken || tokenData.access_token || ''
    refreshToken.value = tokenData.refreshToken || tokenData.refresh_token || ''
    expiresIn.value = tokenData.expiresIn || tokenData.expires_in || 7200

    const userInfo = await request.get('/urm/oauth2/userinfo')
    userId.value = userInfo.sub
    username.value = userInfo.username
    tenantId.value = userInfo.tenantId || ''
    tenantName.value = userInfo.tenantName || ''

    saveToLocalStorage()
    startAutoRefresh()
    return userInfo
  }

  const logout = async () => {
    try {
      if (accessToken.value) {
        await logoutApi().catch(() => {})
      }
    } catch (error) {
      console.error('Logout failed:', error)
    } finally {
      clearState()
      stopAutoRefresh()
      await redirectToLogin()
    }
  }

  const refreshAccessToken = async () => {
    if (!refreshToken.value) {
      clearState()
      await redirectToLogin()
      return
    }
    try {
      const response = await refreshApi(refreshToken.value)
      accessToken.value = response.accessToken
      if (response.refreshToken) {
        refreshToken.value = response.refreshToken
      }
      expiresIn.value = response.expiresIn || 7200
      saveToLocalStorage()
      startAutoRefresh()
      return response
    } catch (error) {
      console.error('Token refresh failed:', error)
      clearState()
      await redirectToLogin()
      throw error
    }
  }

  const isAuthenticated = () => !!accessToken.value

  // 保存状态到 localStorage
  const saveToLocalStorage = () => {
    localStorage.setItem('tenant_accessToken', accessToken.value)
    localStorage.setItem('tenant_refreshToken', refreshToken.value)
    localStorage.setItem('tenant_userId', userId.value)
    localStorage.setItem('tenant_username', username.value)
    localStorage.setItem('tenant_tenantId', tenantId.value)
    localStorage.setItem('tenant_tenantName', tenantName.value)
    localStorage.setItem('tenant_expiresIn', expiresIn.value.toString())
  }

  // 清除状态
  const clearState = () => {
    accessToken.value = ''
    refreshToken.value = ''
    userId.value = ''
    username.value = ''
    tenantId.value = ''
    tenantName.value = ''
    expiresIn.value = 7200
    localStorage.removeItem('tenant_accessToken')
    localStorage.removeItem('tenant_refreshToken')
    localStorage.removeItem('tenant_userId')
    localStorage.removeItem('tenant_username')
    localStorage.removeItem('tenant_tenantId')
    localStorage.removeItem('tenant_tenantName')
    localStorage.removeItem('tenant_expiresIn')
  }

  // 启动自动刷新（Token 过期前 5 分钟触发）
  const startAutoRefresh = () => {
    stopAutoRefresh()
    const refreshTime = (expiresIn.value - 300) * 1000
    if (refreshTime > 0) {
      refreshTimer = setTimeout(() => {
        refreshAccessToken()
      }, refreshTime)
    }
  }

  const stopAutoRefresh = () => {
    if (refreshTimer) {
      clearTimeout(refreshTimer)
      refreshTimer = null
    }
  }

  // 初始化：从 localStorage 恢复状态并启动自动刷新
  const init = () => {
    if (accessToken.value && refreshToken.value) {
      startAutoRefresh()
    }
  }

  return {
    accessToken,
    refreshToken,
    userId,
    username,
    tenantId,
    tenantName,
    expiresIn,
    login,
    loginWithSSO,
    logout,
    refreshAccessToken,
    isAuthenticated,
    startAutoRefresh,
    stopAutoRefresh,
    clearState,
    init
  }
})
