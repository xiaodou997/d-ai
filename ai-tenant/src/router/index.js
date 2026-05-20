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
      // 用户管理
      {
        path: '/users',
        name: 'UserList',
        component: () => import('@/views/Users/UserList.vue'),
        meta: { title: '终端用户', requiresAuth: true }
      },
      // 邀请码
      {
        path: '/invite-codes',
        name: 'InviteCodeList',
        component: () => import('@/views/InviteCodes/InviteCodeList.vue'),
        meta: { title: '邀请码管理', requiresAuth: true }
      },
      // 财务中心
      {
        path: '/finance/account',
        name: 'FinanceAccount',
        component: () => import('@/views/Finance/Account.vue'),
        meta: { title: '我的账户', requiresAuth: true }
      },
      {
        path: '/finance/transactions',
        name: 'FinanceTransactions',
        component: () => import('@/views/Finance/Transactions.vue'),
        meta: { title: '交易流水', requiresAuth: true }
      },
      {
        path: '/finance/user-recharge-records',
        name: 'UserRechargeRecords',
        component: () => import('@/views/Finance/UserRechargeRecords.vue'),
        meta: { title: '用户充值记录', requiresAuth: true }
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
