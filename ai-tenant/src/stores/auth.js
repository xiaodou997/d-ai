import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import {
  refreshAccessToken as refreshAccessTokenApi,
  logout as logoutApi,
  getCurrentUser
} from '@/api/auth'
import router from '@/router'

const SSO_LOGOUT_URL = (import.meta.env.VITE_SSO_AUTHORIZE_URL || '').replace('/oauth2/authorize', '/oauth2/logout')

export const useAuthStore = defineStore('tenantAuth', () => {
  const accessToken = ref(localStorage.getItem('tenant_accessToken') || '')
  const refreshToken = ref(localStorage.getItem('tenant_refreshToken') || '')
  const userId = ref(localStorage.getItem('tenant_userId') || '')
  const username = ref(localStorage.getItem('tenant_username') || '')
  const tenantId = ref(localStorage.getItem('tenant_tenantId') || '')
  const tenantName = ref(localStorage.getItem('tenant_tenantName') || '')
  const userType = ref(parseInt(localStorage.getItem('tenant_userType') || '0'))
  const expiresIn = ref(parseInt(localStorage.getItem('tenant_expiresIn') || '7200'))

  const roleName = computed(() => '租户管理员')

  let refreshTimer = null

  const redirectToLogin = async () => {
    if (router.currentRoute.value.path !== '/login') {
      await router.replace('/login')
    }
  }

  const setAuth = (response) => {
    accessToken.value = response.accessToken || ''
    refreshToken.value = response.refreshToken || ''
    expiresIn.value = response.expiresIn || 7200
    saveToLocalStorage()
    startAutoRefresh()
  }

  const fetchUserInfo = async () => {
    const userInfo = await getCurrentUser()
    userId.value = userInfo.sub || userInfo.userId || ''
    username.value = userInfo.username || ''
    tenantId.value = userInfo.tenantId || ''
    tenantName.value = userInfo.tenantName || ''
    userType.value = Number(userInfo.userType || 0)
    saveToLocalStorage()
    if (userType.value !== 3) {
      clearState()
      throw new Error('当前账号无权访问租户管理中心')
    }
    return userInfo
  }

  const logout = async () => {
    try {
      if (accessToken.value) {
        await logoutApi()
      }
    } catch (error) {
      console.error('Logout failed:', error)
    } finally {
      clearState()
      stopAutoRefresh()
      if (SSO_LOGOUT_URL) {
        const postLogoutUri = encodeURIComponent(window.location.origin + '/login')
        window.location.href = `${SSO_LOGOUT_URL}?post_logout_redirect_uri=${postLogoutUri}`
      } else {
        await redirectToLogin()
      }
    }
  }

  const refreshAccessToken = async () => {
    if (!refreshToken.value) {
      clearState()
      await redirectToLogin()
      return
    }

    try {
      const response = await refreshAccessTokenApi(refreshToken.value)
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

  const saveToLocalStorage = () => {
    localStorage.setItem('tenant_accessToken', accessToken.value)
    localStorage.setItem('tenant_refreshToken', refreshToken.value)
    localStorage.setItem('tenant_userId', userId.value)
    localStorage.setItem('tenant_username', username.value)
    localStorage.setItem('tenant_tenantId', tenantId.value)
    localStorage.setItem('tenant_tenantName', tenantName.value)
    localStorage.setItem('tenant_userType', userType.value.toString())
    localStorage.setItem('tenant_expiresIn', expiresIn.value.toString())
  }

  const clearState = () => {
    accessToken.value = ''
    refreshToken.value = ''
    userId.value = ''
    username.value = ''
    tenantId.value = ''
    tenantName.value = ''
    userType.value = 0
    expiresIn.value = 7200
    localStorage.removeItem('tenant_accessToken')
    localStorage.removeItem('tenant_refreshToken')
    localStorage.removeItem('tenant_userId')
    localStorage.removeItem('tenant_username')
    localStorage.removeItem('tenant_tenantId')
    localStorage.removeItem('tenant_tenantName')
    localStorage.removeItem('tenant_userType')
    localStorage.removeItem('tenant_expiresIn')
  }

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
    userType,
    expiresIn,
    roleName,
    logout,
    refreshAccessToken,
    fetchUserInfo,
    setAuth,
    isAuthenticated,
    startAutoRefresh,
    stopAutoRefresh,
    clearState,
    init
  }
})
