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
    redirect: () => {
      const authStore = useAuthStore()
      return authStore.defaultRoute
    },
    meta: { requiresAuth: true },
    children: [
      {
        path: '/dashboard',
        name: 'Dashboard',
        component: () => import('@/views/Dashboard/index.vue'),
        meta: { title: '控制概览', requiresAuth: true, roles: [1] }
      },
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
        path: '/finance/recharge',
        name: 'TenantRecharge',
        component: () => import('@/views/Finance/Recharge.vue'),
        meta: { title: '租户充值', requiresAuth: true, hidden: true, roles: [1] }
      },
      {
        path: '/finance/recharge-records',
        name: 'TenantRechargeRecords',
        component: () => import('@/views/Finance/RechargeRecords.vue'),
        meta: { title: '充值记录', requiresAuth: true, roles: [1] }
      },
      {
        path: '/ai-gateway',
        component: () => import('@/views/AIGateway/index.vue'),
        redirect: () => {
          const authStore = useAuthStore()
          return authStore.isPlatformAdmin ? '/ai-gateway/providers' : '/ai-gateway/access'
        },
        meta: { title: 'AI 网关', requiresAuth: true, roles: [1, 2, 3] },
        children: [
          {
            path: 'providers',
            name: 'GatewayProviders',
            component: () => import('@/views/AIGateway/GatewayProviders.vue'),
            meta: { title: '厂商接入', requiresAuth: true, roles: [1] }
          },
          {
            path: 'models',
            name: 'GatewayModels',
            component: () => import('@/views/AIGateway/GatewayModels.vue'),
            meta: { title: '模型映射', requiresAuth: true, roles: [1] }
          },
          {
            path: 'access',
            name: 'GatewayAccess',
            component: () => import('@/views/AIGateway/GatewayAccess.vue'),
            meta: { title: '授权与 Key', requiresAuth: true, roles: [1, 2, 3] }
          },
          {
            path: 'usage',
            name: 'GatewayUsage',
            component: () => import('@/views/AIGateway/GatewayUsage.vue'),
            meta: { title: '调用日志', requiresAuth: true, roles: [1, 2, 3] }
          },
          {
            path: 'limits',
            name: 'GatewayLimits',
            component: () => import('@/views/AIGateway/GatewayLimits.vue'),
            meta: { title: '限流策略', requiresAuth: true, roles: [1] }
          },
          {
            path: 'audit',
            name: 'GatewayAudit',
            component: () => import('@/views/AIGateway/GatewayAudit.vue'),
            meta: { title: '网关审计', requiresAuth: true, roles: [1] }
          }
        ]
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
