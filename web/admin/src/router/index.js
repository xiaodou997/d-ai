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
    component: () => import('@/components/Layout/Layout.vue'),
    redirect: '/dashboard',
    meta: { requiresAuth: true },
    children: [
      // 数据监控
      {
        path: '/dashboard',
        name: 'Dashboard',
        component: () => import('@/views/Dashboard/index.vue'),
        meta: { title: '控制概览', requiresAuth: true, roles: [1] }
      },
      {
        path: '/finance/account-overview',
        name: 'AccountOverview',
        component: () => import('@/views/Finance/AccountOverview.vue'),
        meta: { title: '账户全景', requiresAuth: true, roles: [1] }
      },
      // 业务管理
      {
        path: '/tenants',
        name: 'TenantList',
        component: () => import('@/views/Tenant/TenantList.vue'),
        meta: { title: '租户管理', requiresAuth: true, roles: [1] }
      },
      {
        path: '/tenants/:id',
        name: 'TenantDetail',
        component: () => import('@/views/Tenant/TenantDetail.vue'),
        meta: { title: '租户详情', requiresAuth: true, hidden: true, roles: [1] }
      },
      {
        path: '/users',
        name: 'UserList',
        component: () => import('@/views/User/UserList.vue'),
        meta: { title: '终端用户', requiresAuth: true, roles: [1] }
      },
      {
        path: '/apps',
        name: 'Apps',
        component: () => import('@/views/Apps/AppList.vue'),
        meta: { title: '应用系统', requiresAuth: true, roles: [1] }
      },
      {
        path: '/ai-gateway',
        name: 'AIGateway',
        component: () => import('@/views/AIGateway/index.vue'),
        meta: { title: 'AI 网关', requiresAuth: true, roles: [1, 2, 3] }
      },
      // 财务中心
      {
        path: '/finance/recharge',
        name: 'Recharge',
        component: () => import('@/views/Finance/Recharge.vue'),
        meta: { title: '租户充值', requiresAuth: true, roles: [1] }
      },
      {
        path: '/finance/recharge-records',
        name: 'RechargeRecords',
        component: () => import('@/views/Finance/RechargeRecords.vue'),
        meta: { title: '充值记录', requiresAuth: true, roles: [1] }
      },
      {
        path: '/finance/transactions',
        name: 'Transactions',
        component: () => import('@/views/Finance/TransactionList.vue'),
        meta: { title: '交易流水', requiresAuth: true, roles: [1] }
      },
      {
        path: '/finance/tenant/grant-logs',
        name: 'TenantGrantLogs',
        component: () => import('@/views/Finance/tenant/GrantLogs.vue'),
        meta: { title: '租户补发记录', requiresAuth: true, roles: [1] }
      },
      {
        path: '/finance/user/grant-logs',
        name: 'UserGrantLogs',
        component: () => import('@/views/Finance/user/GrantLogs.vue'),
        meta: { title: '用户补发记录', requiresAuth: true, roles: [1] }
      },
      // 系统审计
      {
        path: '/system/audit-log',
        name: 'AuditLog',
        component: () => import('@/views/System/AuditLog.vue'),
        meta: { title: '操作审计', requiresAuth: true, roles: [1], superAdminOnly: true }
      },
      {
        path: '/system/admins',
        name: 'AdminList',
        component: () => import('@/views/System/AdminUserList.vue'),
        meta: { title: '系统管理员', requiresAuth: true, roles: [1], superAdminOnly: true }
      },
      {
        path: '/system/jwt-keys',
        name: 'JwtKeys',
        component: () => import('@/views/System/JwtKeys.vue'),
        meta: { title: 'JWT 密钥', requiresAuth: true, roles: [1], superAdminOnly: true }
      }
    ]
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
  }

  if (to.path === '/login' && authStore.isAuthenticated()) {
    return authStore.defaultRoute
  }

  const superAdminOnly = to.matched.some(record => record.meta.superAdminOnly)
  if (superAdminOnly && authStore.userType !== 1) {
    return authStore.defaultRoute
  }

  const allowedRoles = to.matched.flatMap(record => record.meta.roles || [])
  if (allowedRoles.length > 0 && !allowedRoles.includes(authStore.userType)) {
    if (to.path === authStore.defaultRoute) {
      return true
    }
    return authStore.defaultRoute
  }

  return true
})

export default router
