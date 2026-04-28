import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import {
  login as loginApi,
  refreshToken as refreshApi,
  logout as logoutApi
} from '@/api/auth'
import request from '@/utils/request'
import router from '@/router'

export const useAuthStore = defineStore('auth', () => {
  // State
  const accessToken = ref(localStorage.getItem('admin_accessToken') || '')
  const refreshToken = ref(localStorage.getItem('admin_refreshToken') || '')
  const userId = ref(localStorage.getItem('admin_userId') || '')
  const tenantId = ref(localStorage.getItem('admin_tenantId') || '')
  const username = ref(localStorage.getItem('admin_username') || '')
  const userType = ref(parseInt(localStorage.getItem('admin_userType') || '0'))
  const expiresIn = ref(
    parseInt(localStorage.getItem('admin_expiresIn') || '7200')
  )

  // 刷新定时器
  let refreshTimer = null

  // Actions
  const login = async (user, password) => {
    try {
      const response = await loginApi(user, password)

      // 更新 Token
      accessToken.value = response.accessToken
      refreshToken.value = response.refreshToken
      expiresIn.value = response.expiresIn

      // 获取用户信息
      const userInfo = await request.get('/urm/oauth2/userinfo')
      userId.value = userInfo.sub
      tenantId.value = userInfo.tenantId || ''
      username.value = userInfo.username
      userType.value = userInfo.userType

      // 持久化到 localStorage
      saveToLocalStorage()

      // 启动自动刷新
      startAutoRefresh()

      router.push(defaultRoute.value)

      return {
        ...response,
        userId: userInfo.sub,
        username: userInfo.username,
        userType: userInfo.userType
      }
    } catch (error) {
      console.error('Login failed:', error)
      throw error
    }
  }

  const logout = async () => {
    try {
      // 调用登出 API（如果 token 有效）
      if (accessToken.value) {
        await logoutApi().catch(() => {
          // 忽略登出接口错误（如 token 已过期）
        })
      }
    } catch (error) {
      console.error('Logout failed:', error)
    } finally {
      // 清除状态和跳转由 request.js 拦截器统一处理
      // 这里只清除本地状态和停止定时器
      clearState()
      stopAutoRefresh()
    }
  }

  const refreshAccessToken = async () => {
    if (!refreshToken.value) {
      console.warn('No refresh token available')
      clearState()
      stopAutoRefresh()
      // 不在这里跳转，由拦截器处理
      throw new Error('No refresh token')
    }

    try {
      const response = await refreshApi(refreshToken.value)

      // 更新 Token
      accessToken.value = response.accessToken
      // 如果返回了新的 Refresh Token，也更新
      if (response.refreshToken) {
        refreshToken.value = response.refreshToken
      }
      expiresIn.value = response.expiresIn

      // 持久化
      saveToLocalStorage()

      // 重新启动自动刷新
      startAutoRefresh()

      console.log('Token refreshed successfully')
      return response
    } catch (error) {
      console.error('Token refresh failed:', error)
      // Refresh Token 过期或无效，清除状态
      clearState()
      stopAutoRefresh()
      // 不在这里跳转，由拦截器处理
      throw error
    }
  }

  const isAuthenticated = () => {
    return !!accessToken.value
  }

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

  const isTokenExpiring = () => {
    // 如果 Token 剩余时间少于 5 分钟，认为即将过期
    return expiresIn.value < 300
  }

  // 保存状态到 localStorage
  const saveToLocalStorage = () => {
    localStorage.setItem('admin_accessToken', accessToken.value)
    localStorage.setItem('admin_refreshToken', refreshToken.value)
    localStorage.setItem('admin_userId', userId.value)
    localStorage.setItem('admin_tenantId', tenantId.value)
    localStorage.setItem('admin_username', username.value)
    localStorage.setItem('admin_userType', userType.value.toString())
    localStorage.setItem('admin_expiresIn', expiresIn.value.toString())
  }

  // 清除状态
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

  // 启动自动刷新
  const startAutoRefresh = () => {
    // 清除旧的定时器
    stopAutoRefresh()

    // 在 Token 过期前 5 分钟自动刷新
    const refreshTime = (expiresIn.value - 300) * 1000

    if (refreshTime > 0) {
      refreshTimer = setTimeout(() => {
        console.log('Auto-refreshing token...')
        refreshAccessToken()
      }, refreshTime)

      console.log(`Auto-refresh scheduled in ${refreshTime / 1000} seconds`)
    }
  }

  // 停止自动刷新
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
    // State
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

    // Actions
    login,
    logout,
    refreshAccessToken,
    isAuthenticated,
    isTokenExpiring,
    startAutoRefresh,
    stopAutoRefresh,
    clearState,
    saveToLocalStorage,
    init
  }
})
