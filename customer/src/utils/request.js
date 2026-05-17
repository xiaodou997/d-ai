import axios from 'axios'
import { ElMessage } from 'element-plus'
import { useAuthStore } from '@/stores/auth'
import router from '@/router'

const request = axios.create({
  baseURL: '',
  timeout: 30000,
  headers: {
    'Content-Type': 'application/json'
  }
})

let isRefreshing = false
let refreshSubscribers = []

const redirectToLogin = () => {
  if (router.currentRoute.value.path !== '/login') {
    router.replace('/login')
  }
}

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

request.interceptors.request.use(
  (config) => {
    const authStore = useAuthStore()
    if (authStore.accessToken) {
      config.headers.Authorization = `Bearer ${authStore.accessToken}`
    }
    return config
  },
  (error) => Promise.reject(error)
)

request.interceptors.response.use(
  (response) => {
    const res = response.data

    if (res.code !== undefined && res.code !== 0) {
      if (res.code === 10001 || res.code === 10002) {
        const authStore = useAuthStore()
        authStore.clearState()
        authStore.stopAutoRefresh()
        ElMessage.error(res.message || '登录已过期，请重新登录')
        redirectToLogin()
        return Promise.reject(new Error(res.message || '登录已过期'))
      }

      ElMessage.error(res.message || '请求失败')
      return Promise.reject(new Error(res.message || '请求失败'))
    }

    return res.data !== undefined ? res.data : res
  },
  async (error) => {
    if (error.response) {
      const { status, data } = error.response

      if (status === 401) {
        const authStore = useAuthStore()

        const isRefreshRequest =
          error.config.url?.includes('/oauth2/token') &&
          error.config.data instanceof URLSearchParams &&
          error.config.data.get('grant_type') === 'refresh_token'
        if (isRefreshRequest) {
          ElMessage.error('登录已过期，请重新登录')
          authStore.clearState()
          authStore.stopAutoRefresh()
          isRefreshing = false
          refreshSubscribers = []
          redirectToLogin()
          return Promise.reject(error)
        }

        if (error.config.url?.includes('/oauth2/revoke')) {
          authStore.clearState()
          authStore.stopAutoRefresh()
          redirectToLogin()
          return Promise.reject(error)
        }

        if (isRefreshing) {
          return new Promise((resolve, reject) => {
            subscribeTokenRefresh((newToken) => {
              const config = error.config
              config.headers.Authorization = `Bearer ${newToken}`
              resolve(request(config))
            }, reject)
          })
        }

        isRefreshing = true
        try {
          const newToken = await authStore.refreshAccessToken()
          isRefreshing = false
          onRefreshed(newToken.accessToken)

          const config = error.config
          config.headers.Authorization = `Bearer ${newToken.accessToken}`
          return request(config)
        } catch (refreshError) {
          isRefreshing = false
          onRefreshFailed(refreshError)
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
