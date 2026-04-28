import axios from 'axios'
import { ElMessage } from 'element-plus'
import { useAuthStore } from '@/stores/auth'
import router from '@/router'

// 创建 axios 实例
const request = axios.create({
  baseURL: '',
  timeout: 30000,
  headers: {
    'Content-Type': 'application/json'
  }
})

// Token 刷新队列
let isRefreshing = false
let refreshSubscribers = []

// 添加重试请求到队列
function subscribeTokenRefresh(cb) {
  refreshSubscribers.push(cb)
}

// 执行所有等待的请求
function onRefreshed(token) {
  refreshSubscribers.forEach(cb => cb(token))
  refreshSubscribers = []
}

// 拒绝所有等待的请求
function onRefreshFailed() {
  refreshSubscribers.forEach(cb => cb(null))
  refreshSubscribers = []
}

// 请求拦截器
request.interceptors.request.use(
  (config) => {
    const authStore = useAuthStore()
    if (authStore.accessToken) {
      config.headers['Authorization'] = `Bearer ${authStore.accessToken}`
    }
    return config
  },
  (error) => {
    console.error('Request error:', error)
    return Promise.reject(error)
  }
)

// 响应拦截器
request.interceptors.response.use(
  (response) => {
    const res = response.data

    // 如果返回的 code 不是 200，则认为是错误
    if (res.code !== undefined && res.code !== 200) {
      ElMessage.error(res.message || '请求失败')

      if (res.code === 401) {
        const authStore = useAuthStore()
        authStore.clearState()
        router.push('/login')
      }

      return Promise.reject(new Error(res.message || '请求失败'))
    }

    // 返回 data 字段，或直接返回整个 res（兼容不同后端格式）
    return res.data !== undefined ? res.data : res
  },
  async (error) => {
    console.error('Response error:', error)

    const originalRequest = error.config

    if (error.response) {
      const { status, data } = error.response

      // 401 未授权，尝试刷新 token
      if (status === 401) {
        const authStore = useAuthStore()

        // 如果是刷新 token 接口返回 401，说明 refresh token 也过期了
        const isRefreshRequest =
          originalRequest.url?.includes('/oauth2/token') &&
          typeof originalRequest.data === 'string' &&
          originalRequest.data.includes('grant_type=refresh_token')
        if (isRefreshRequest) {
          ElMessage.error('登录已过期，请重新登录')
          authStore.clearState()
          authStore.stopAutoRefresh()
          isRefreshing = false
          refreshSubscribers = []
          router.push('/login')
          return Promise.reject(error)
        }

        // 如果是登出接口，直接清理状态
        if (originalRequest.url?.includes('/oauth2/revoke')) {
          authStore.clearState()
          authStore.stopAutoRefresh()
          router.push('/login')
          return Promise.reject(error)
        }

        // 防止无限重试
        if (originalRequest._retryCount >= 1) {
          ElMessage.error('登录已过期，请重新登录')
          authStore.clearState()
          authStore.stopAutoRefresh()
          isRefreshing = false
          refreshSubscribers = []
          router.push('/login')
          return Promise.reject(error)
        }

        originalRequest._retryCount = (originalRequest._retryCount || 0) + 1

        if (!isRefreshing) {
          isRefreshing = true

          try {
            // 尝试刷新 token
            await authStore.refreshAccessToken()
            isRefreshing = false

            // 刷新成功，重试所有等待的请求
            onRefreshed(authStore.accessToken)

            // 重试原请求
            originalRequest.headers['Authorization'] = `Bearer ${authStore.accessToken}`
            return request(originalRequest)
          } catch (refreshError) {
            // 刷新失败，auth store 内部已处理 clearState 和跳转
            isRefreshing = false
            onRefreshFailed()
            return Promise.reject(refreshError)
          }
        } else {
          // 正在刷新中，将请求加入队列等待
          return new Promise((resolve) => {
            subscribeTokenRefresh((newToken) => {
              if (newToken) {
                originalRequest.headers['Authorization'] = `Bearer ${newToken}`
                resolve(request(originalRequest))
              } else {
                // 刷新失败，拒绝请求
                resolve(Promise.reject(new Error('Token refresh failed')))
              }
            })
          })
        }
      } else if (status === 403) {
        ElMessage.error('没有权限访问')
      } else if (status === 404) {
        ElMessage.error('请求的资源不存在')
      } else if (status === 500) {
        ElMessage.error('服务器错误')
      } else {
        ElMessage.error(data?.message || '请求失败')
      }
    } else if (error.request) {
      ElMessage.error('网络错误，请检查网络连接')
    } else {
      ElMessage.error('请求配置错误')
    }

    return Promise.reject(error)
  }
)

export default request
