import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
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
  const userType = ref(parseInt(localStorage.getItem('customer_userType') || '0'))
  const clientType = ref(localStorage.getItem('customer_clientType') || '')

  const roleName = computed(() => '终端用户')

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
    userType.value = Number(userInfo.userType || 0)
    clientType.value = userInfo.clientType || ''
    saveToLocalStorage()
    if (userType.value !== 4) {
      clearState()
      throw new Error('当前账号无权访问用户中心')
    }
    return userInfo
  }

  const logout = async () => {
    const ct = clientType.value
    try {
      if (accessToken.value) {
        await logoutApi()
      }
    } catch (error) {
      console.error('Logout failed:', error)
    } finally {
      clearState()
      stopAutoRefresh()
      if (SSO_LOGOUT_URL && ct) {
        const postLogoutUri = encodeURIComponent(window.location.origin + '/login')
        window.location.href = `${SSO_LOGOUT_URL}?client_type=${ct}&post_logout_redirect_uri=${postLogoutUri}`
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
    localStorage.setItem('customer_userType', userType.value.toString())
    localStorage.setItem('customer_clientType', clientType.value)
    localStorage.setItem('customer_expiresIn', expiresIn.value.toString())
  }

  const clearState = () => {
    accessToken.value = ''
    refreshToken.value = ''
    userId.value = ''
    username.value = ''
    tenantId.value = ''
    userType.value = 0
    clientType.value = ''
    expiresIn.value = 7200
    localStorage.removeItem('customer_accessToken')
    localStorage.removeItem('customer_refreshToken')
    localStorage.removeItem('customer_userId')
    localStorage.removeItem('customer_username')
    localStorage.removeItem('customer_tenantId')
    localStorage.removeItem('customer_userType')
    localStorage.removeItem('customer_clientType')
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
    userType,
    clientType,
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
