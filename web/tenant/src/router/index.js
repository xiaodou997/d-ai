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
