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
      // 工作台（首页）
      {
        path: '/dashboard',
        name: 'Workspace',
        component: () => import('@/views/AIGateway/MyUsage.vue'),
        meta: { title: '工作台', requiresAuth: true }
      },
      // AI Gateway
      {
        path: '/ai/models',
        name: 'AvailableModels',
        component: () => import('@/views/AIGateway/AvailableModels.vue'),
        meta: { title: '可用模型', requiresAuth: true }
      },
      {
        path: '/ai/api-keys',
        name: 'MyAPIKeys',
        component: () => import('@/views/AIGateway/MyAPIKeys.vue'),
        meta: { title: '我的 API Key', requiresAuth: true }
      },
      // URM remote pages are resolved from the URM customer manifest.
      {
        path: '/urm/:pathMatch(.*)*',
        name: 'UrmCustomerRemote',
        component: () => import('@/views/Remote/UrmCustomerRemoteView.vue'),
        meta: { title: 'URM', requiresAuth: true }
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

  if (authStore.isAuthenticated() && authStore.userType !== 0 && authStore.userType !== 4) {
    authStore.clearState()
    return '/login'
  }

  if (to.path === '/login' && authStore.isAuthenticated()) {
    return '/dashboard'
  }
})

export default router
