import { createRouter, createWebHistory } from 'vue-router'
import { useAuthStore } from '@/stores/auth'

const routes = [
  {
    path: '/login',
    name: 'Login',
    component: () => import('@/views/Login.vue'),
    meta: { requiresAuth: false }
  },
  {
    path: '/oauth/callback',
    name: 'OAuthCallback',
    component: () => import('@/views/OAuthCallback.vue'),
    meta: { requiresAuth: false }
  },
  {
    path: '/',
    component: () => import('@/views/Layout/Layout.vue'),
    redirect: '/dashboard',
    meta: { requiresAuth: true },
    children: [
      // 数据监控
      {
        path: '/dashboard',
        name: 'Dashboard',
        component: () => import('@/views/Dashboard/index.vue'),
        meta: { title: '控制概览', requiresAuth: true }
      },
      // URM remote pages are resolved from the URM tenant manifest.
      {
        path: '/urm/:pathMatch(.*)*',
        name: 'UrmTenantRemote',
        component: () => import('@/views/Remote/UrmTenantRemoteView.vue'),
        meta: { title: 'URM', requiresAuth: true }
      },
      // 接入文档
      {
        path: '/docs/api',
        name: 'ApiDocs',
        component: () => import('@/views/Docs/ApiDocs.vue'),
        meta: { title: '接入文档', requiresAuth: true }
      },
      // AI Gateway
      {
        path: '/ai/models',
        name: 'AIAvailableModels',
        component: () => import('@/views/AIGateway/AvailableModels.vue'),
        meta: { title: '已授权模型', requiresAuth: true }
      },
      {
        path: '/ai/chat',
        name: 'AIChat',
        component: () => import('@/views/AIGateway/AIChat.vue'),
        meta: { title: 'AI 对话', requiresAuth: true }
      },
      {
        path: '/ai/prices',
        name: 'AITenantPrices',
        component: () => import('@/views/AIGateway/TenantPrices.vue'),
        meta: { title: '租户定价', requiresAuth: true }
      },
      {
        path: '/ai/api-keys',
        name: 'AIAPIKeys',
        component: () => import('@/views/AIGateway/APIKeys.vue'),
        meta: { title: 'API Key', requiresAuth: true }
      },
      {
        path: '/ai/user-consumption',
        name: 'AIUserConsumption',
        component: () => import('@/views/AIGateway/UserConsumption.vue'),
        meta: { title: '用户消耗', requiresAuth: true }
      }
    ]
  },
  {
    path: '/:pathMatch(.*)*',
    redirect: '/dashboard'
  }
]

const router = createRouter({
  history: createWebHistory(),
  routes
})

router.beforeEach((to) => {
  const authStore = useAuthStore()
  const requiresAuth = to.matched.some(record => record.meta.requiresAuth)

  if (requiresAuth && !authStore.isAuthenticated()) {
    return '/login'
  }

  if (authStore.isAuthenticated() && authStore.userType !== 0 && authStore.userType !== 3) {
    authStore.clearState()
    return '/login'
  }

  if (to.path === '/login' && authStore.isAuthenticated()) {
    return '/dashboard'
  }
})

export default router
