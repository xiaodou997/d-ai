import { defineStore } from 'pinia'
import { ref } from 'vue'
import {
  login as loginApi,
  refreshToken as refreshApi,
  logout as logoutApi
} from '@/api/auth'
import request from '@/utils/request'
import router from '@/router'

export const useAuthStore = defineStore('customerAuth', () => {
  const accessToken = ref(localStorage.getItem('customer_accessToken') || '')
  const refreshToken = ref(localStorage.getItem('customer_refreshToken') || '')
  const expiresIn = ref(
    parseInt(localStorage.getItem('customer_expiresIn') || '7200')
  )
  const username = ref(localStorage.getItem('customer_username') || '')
  const userId = ref(localStorage.getItem('customer_userId') || '')
  const tenantId = ref(localStorage.getItem('customer_tenantId') || '')

  let refreshTimer = null

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

    saveToLocalStorage()
    startAutoRefresh()
    router.push('/dashboard')
    return { ...response, userId: userInfo.sub, username: userInfo.username }
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
      router.push('/login')
    }
  }

  const refreshAccessToken = async () => {
    if (!refreshToken.value) {
      clearState()
      router.push('/login')
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
      router.push('/login')
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
    login,
    logout,
    refreshAccessToken,
    isAuthenticated,
    startAutoRefresh,
    stopAutoRefresh,
    clearState,
    init
  }
})
