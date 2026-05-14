import { defineStore } from 'pinia'
import { ref } from 'vue'
import {
  refreshAccessToken as refreshAccessTokenApi,
  logout as logoutApi,
  getCurrentUser
} from '@/api/auth'
import router from '@/router'

const SSO_LOGOUT_URL = (import.meta.env.VITE_SSO_AUTHORIZE_URL || '').replace('/oauth2/authorize', '/oauth2/logout')

export const useAuthStore = defineStore('customerAuth', () => {
  const accessToken = ref(localStorage.getItem('customer_accessToken') || '')
  const refreshToken = ref(localStorage.getItem('customer_refreshToken') || '')
  const expiresIn = ref(parseInt(localStorage.getItem('customer_expiresIn') || '7200'))
  const username = ref(localStorage.getItem('customer_username') || '')
  const userId = ref(localStorage.getItem('customer_userId') || '')
  const tenantId = ref(localStorage.getItem('customer_tenantId') || '')

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
    saveToLocalStorage()
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
    localStorage.setItem('customer_accessToken', accessToken.value)
    localStorage.setItem('customer_refreshToken', refreshToken.value)
    localStorage.setItem('customer_userId', userId.value)
    localStorage.setItem('customer_username', username.value)
    localStorage.setItem('customer_tenantId', tenantId.value)
    localStorage.setItem('customer_expiresIn', expiresIn.value.toString())
  }

  const clearState = () => {
    accessToken.value = ''
    refreshToken.value = ''
    userId.value = ''
    username.value = ''
    tenantId.value = ''
    expiresIn.value = 7200
    localStorage.removeItem('customer_accessToken')
    localStorage.removeItem('customer_refreshToken')
    localStorage.removeItem('customer_userId')
    localStorage.removeItem('customer_username')
    localStorage.removeItem('customer_tenantId')
    localStorage.removeItem('customer_expiresIn')
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
    expiresIn,
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
