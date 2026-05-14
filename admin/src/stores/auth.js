import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import {
  refreshAccessToken as refreshAccessTokenApi,
  logout as logoutApi,
  getCurrentUser
} from '@/api/auth'
import router from '@/router'

const SSO_LOGOUT_URL = (import.meta.env.VITE_SSO_AUTHORIZE_URL || '').replace('/oauth2/authorize', '/oauth2/logout')

export const useAuthStore = defineStore('auth', () => {
  const accessToken = ref(localStorage.getItem('admin_accessToken') || '')
  const refreshToken = ref(localStorage.getItem('admin_refreshToken') || '')
  const userId = ref(localStorage.getItem('admin_userId') || '')
  const tenantId = ref(localStorage.getItem('admin_tenantId') || '')
  const username = ref(localStorage.getItem('admin_username') || '')
  const userType = ref(parseInt(localStorage.getItem('admin_userType') || '0'))
  const expiresIn = ref(parseInt(localStorage.getItem('admin_expiresIn') || '7200'))

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
    tenantId.value = userInfo.tenantId || ''
    username.value = userInfo.username || ''
    userType.value = Number(userInfo.userType || 0)
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
      stopAutoRefresh()
      throw new Error('No refresh token')
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
      stopAutoRefresh()
      await redirectToLogin()
      throw error
    }
  }

  const isAuthenticated = () => !!accessToken.value
  const isPlatformAdmin = computed(() => userType.value === 1)
  const isTenantAdmin = computed(() => userType.value === 2)
  const isEndUser = computed(() => userType.value === 3)

  const roleName = computed(() => {
    if (isPlatformAdmin.value) return '平台管理员'
    if (isTenantAdmin.value) return '租户管理员'
    if (isEndUser.value) return '终端用户'
    return '未识别角色'
  })

  const defaultRoute = computed(() => (isPlatformAdmin.value ? '/dashboard' : '/ai-gateway/access'))

  const isTokenExpiring = () => expiresIn.value < 300

  const saveToLocalStorage = () => {
    localStorage.setItem('admin_accessToken', accessToken.value)
    localStorage.setItem('admin_refreshToken', refreshToken.value)
    localStorage.setItem('admin_userId', userId.value)
    localStorage.setItem('admin_tenantId', tenantId.value)
    localStorage.setItem('admin_username', username.value)
    localStorage.setItem('admin_userType', userType.value.toString())
    localStorage.setItem('admin_expiresIn', expiresIn.value.toString())
  }

  const clearState = () => {
    accessToken.value = ''
    refreshToken.value = ''
    userId.value = ''
    tenantId.value = ''
    username.value = ''
    userType.value = 0
    expiresIn.value = 7200

    localStorage.removeItem('admin_accessToken')
    localStorage.removeItem('admin_refreshToken')
    localStorage.removeItem('admin_userId')
    localStorage.removeItem('admin_tenantId')
    localStorage.removeItem('admin_username')
    localStorage.removeItem('admin_userType')
    localStorage.removeItem('admin_expiresIn')
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
    tenantId,
    username,
    userType,
    expiresIn,
    isPlatformAdmin,
    isTenantAdmin,
    isEndUser,
    roleName,
    defaultRoute,
    logout,
    refreshAccessToken,
    fetchUserInfo,
    setAuth,
    isAuthenticated,
    isTokenExpiring,
    startAutoRefresh,
    stopAutoRefresh,
    clearState,
    saveToLocalStorage,
    init
  }
})
