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

// 刷新锁，防止并发请求时多次刷新 Token
let isRefreshing = false
// 刷新队列：等待刷新完成的请求
let refreshSubscribers = []

const subscribeTokenRefresh = (resolve, reject) => {
  refreshSubscribers.push({ resolve, reject })
}

const onRefreshed = (accessToken) => {
  refreshSubscribers.forEach((cb) => cb.resolve(accessToken))
  refreshSubscribers = []
}

const onRefreshFailed = (error) => {
  refreshSubscribers.forEach((cb) => cb.reject(error))
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

    if (res.code !== undefined && res.code !== 200) {
      ElMessage.error(res.message || '请求失败')
      return Promise.reject(new Error(res.message || '请求失败'))
    }

    return res.data !== undefined ? res.data : res
  },
  async (error) => {
    console.error('Response error:', error)

    if (error.response) {
      const { status, data } = error.response

      if (status === 401) {
        const authStore = useAuthStore()

        // 判断是否是登录请求（grant_type=password）
        const isLoginRequest =
          error.config.url?.includes('/oauth2/token') &&
          typeof error.config.data === 'string' &&
          error.config.data.includes('grant_type=password')
        if (isLoginRequest) {
          // 登录请求返回 401 = 用户名或密码错误，直接返回
          ElMessage.error(data?.message || '用户名或密码错误')
          return Promise.reject(error)
        }

        // 如果是刷新 token 接口返回 401，说明 refresh token 也过期了
        const isRefreshRequest =
          error.config.url?.includes('/oauth2/token') &&
          typeof error.config.data === 'string' &&
          error.config.data.includes('grant_type=refresh_token')
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
        if (error.config.url?.includes('/oauth2/revoke')) {
          authStore.clearState()
          authStore.stopAutoRefresh()
          router.push('/login')
          return Promise.reject(error)
        }

        // 如果正在刷新，将请求加入队列等待
        if (isRefreshing) {
          return new Promise((resolve, reject) => {
            subscribeTokenRefresh((newToken) => {
              const config = error.config
              config.headers['Authorization'] = `Bearer ${newToken}`
              resolve(request(config))
            }, reject)
          })
        }

        // 尝试刷新 Token
        isRefreshing = true
        try {
          const newToken = await authStore.refreshAccessToken()
          isRefreshing = false
          onRefreshed(newToken.accessToken)

          const config = error.config
          config.headers['Authorization'] = `Bearer ${newToken.accessToken}`
          return request(config)
        } catch (refreshError) {
          isRefreshing = false
          onRefreshFailed(refreshError)
          ElMessage.error('登录已过期，请重新登录')
          authStore.clearState()
          authStore.stopAutoRefresh()
          router.push('/login')
          return Promise.reject(refreshError)
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
