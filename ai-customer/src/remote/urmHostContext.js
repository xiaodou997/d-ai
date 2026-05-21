import request from '@/utils/request'
import { useAuthStore } from '@/stores/auth'

export function createUrmHostContext({ router, route, standalonePath }) {
  const authStore = useAuthStore()

  return {
    clientId: import.meta.env.VITE_SSO_CLIENT_ID || 'uni-ai-api',
    clientType: import.meta.env.VITE_SSO_CLIENT_TYPE || 'customer',
    userInfo: {
      userId: authStore.userId,
      username: authStore.username,
      tenantId: authStore.tenantId,
      userType: authStore.userType
    },
    returnTo: route?.fullPath || '',
    request,
    getAccessToken: () => authStore.accessToken,
    refreshAccessToken: () => authStore.refreshAccessToken(),
    logout: () => authStore.logout(),
    navigate: (target) => {
      if (!target) return
      router.push(target)
    },
    openStandalone: (url) => {
      window.open(url || standalonePath || '/', '_blank', 'noopener,noreferrer')
    }
  }
}
