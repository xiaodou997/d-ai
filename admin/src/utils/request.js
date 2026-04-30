import axios from 'axios'
import { ElMessage } from 'element-plus'
import { useAuthStore } from '@/stores/auth'
import router from '@/router'

// 创建 axios 实例
const request = axios.create({
  baseURL: import.meta.env.VITE_API_BASE_URL,
  timeout: 30000,
  headers: {
    'Content-Type': 'application/json'
  }
})

// 刷新锁，防止并发请求时多次刷新 Token
let isRefreshing = false
// 刷新队列
let refreshSubscribers = []

const redirectToLogin = () => {
  if (router.currentRoute.value.path !== '/login') {
    router.replace('/login')
  }
}

// 添加到刷新队列
const subscribeTokenRefresh = (resolve, reject) => {
  refreshSubscribers.push({ resolve, reject })
}

// 执行刷新队列
const onRefreshed = (accessToken) => {
  refreshSubscribers.forEach((callback) => {
    callback.resolve(accessToken)
  })
  refreshSubscribers = []
}

// 刷新队列失败
const onRefreshFailed = (error) => {
  refreshSubscribers.forEach((callback) => {
    callback.reject(error)
  })
  refreshSubscribers = []
}

// 请求拦截器
request.interceptors.request.use(
  async (config) => {
    const authStore = useAuthStore()
    
    // 添加 JWT Token 到请求头
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
    if (res.code !== 200) {
      // 401: Token 过期或无效
      if (res.code === 401) {
        const authStore = useAuthStore()
        
        // 清理本地 token 并跳转登录页
        authStore.clearState()
        authStore.stopAutoRefresh()
        
        // 显示提示
        ElMessage.error(res.message || '登录已过期，请重新登录')
        
        // 跳转到登录页
        redirectToLogin()
        
        return Promise.reject(new Error(res.message || '登录已过期'))
      }
      
      // 其他错误
      ElMessage.error(res.message || '请求失败')
      return Promise.reject(new Error(res.message || '请求失败'))
    }

    // 返回数据
    return res.data
  },
  async (error) => {
    console.error('Response error:', error)

    // 处理 HTTP 错误
    if (error.response) {
      const { status, data } = error.response

      if (status === 401) {
        const authStore = useAuthStore()
        
        // 如果是刷新 token 接口返回 401，说明 refresh token 也过期了
        const isRefreshRequest =
          error.config.url.includes('/oauth2/token') &&
          typeof error.config.data === 'string' &&
          error.config.data.includes('grant_type=refresh_token')
        if (isRefreshRequest) {
          ElMessage.error('登录已过期，请重新登录')
          authStore.clearState()
          authStore.stopAutoRefresh()
          isRefreshing = false
          refreshSubscribers = []
          redirectToLogin()
          return Promise.reject(error)
        }

        // 如果是登出接口，直接清理状态
        if (error.config.url.includes('/oauth2/revoke')) {
          authStore.clearState()
          authStore.stopAutoRefresh()
          redirectToLogin()
          return Promise.reject(error)
        }

        // 其他接口返回 401，尝试刷新 token
        if (isRefreshing) {
          // 如果正在刷新，将请求加入队列
          return new Promise((resolve, reject) => {
            subscribeTokenRefresh((newToken) => {
              const config = error.config
              config.headers['Authorization'] = `Bearer ${newToken}`
              resolve(request(config))
            }, reject)
          })
        }

        // 开始刷新 token
        isRefreshing = true
        try {
          const newToken = await authStore.refreshAccessToken()
          isRefreshing = false
          onRefreshed(newToken.accessToken)

          // 重试之前的请求
          const config = error.config
          config.headers['Authorization'] = `Bearer ${newToken.accessToken}`
          return request(config)
        } catch (refreshError) {
          isRefreshing = false
          onRefreshFailed(refreshError)
          
          // 刷新失败，清理状态并跳转
          ElMessage.error('登录已过期，请重新登录')
          authStore.clearState()
          authStore.stopAutoRefresh()
          redirectToLogin()
          
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
