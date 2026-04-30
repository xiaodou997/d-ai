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
      // 我的账户
      {
        path: '/dashboard',
        name: 'MyAccount',
        component: () => import('@/views/Dashboard/MyAccount.vue'),
        meta: { title: '我的账户', requiresAuth: true }
      },
      // 财务中心
      {
        path: '/finance/transactions',
        name: 'Transactions',
        component: () => import('@/views/Finance/Transactions.vue'),
        meta: { title: '积分流水', requiresAuth: true }
      },
      {
        path: '/finance/recharge',
        name: 'Recharge',
        component: () => import('@/views/Finance/Recharge.vue'),
        meta: { title: '充值记录', requiresAuth: true }
      },
      // 个人中心
      {
        path: '/profile',
        name: 'Profile',
        component: () => import('@/views/Profile/index.vue'),
        meta: { title: '个人中心', requiresAuth: true }
      },
      // AI Gateway
      {
        path: '/ai/api-keys',
        name: 'MyAPIKeys',
        component: () => import('@/views/AIGateway/MyAPIKeys.vue'),
        meta: { title: '我的 API Key', requiresAuth: true }
      },
      {
        path: '/ai/usage',
        name: 'MyUsage',
        component: () => import('@/views/AIGateway/MyUsage.vue'),
        meta: { title: '使用统计', requiresAuth: true }
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

router.beforeEach((to, from) => {
  const authStore = useAuthStore()
  const requiresAuth = to.matched.some(record => record.meta.requiresAuth)

  if (requiresAuth && !authStore.isAuthenticated()) {
    return '/login'
  } else if (to.path === '/login' && authStore.isAuthenticated()) {
    return '/dashboard'
  }
})

export default router
