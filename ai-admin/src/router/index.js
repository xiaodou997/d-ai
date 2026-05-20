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
        meta: { title: '数据大盘', requiresAuth: true, roles: [1, 2] }
      },
      {
        path: '/system-status',
        name: 'SystemStatus',
        component: () => import('@/views/SystemStatus/index.vue'),
        meta: { title: '系统状态', requiresAuth: true, roles: [1, 2] }
      },
      {
        path: '/tenants',
        name: 'TenantList',
        component: () => import('@/views/Tenant/TenantList.vue'),
        meta: { title: '租户管理', requiresAuth: true, roles: [1, 2] }
      },
      {
        path: '/tenants/:id',
        name: 'TenantDetail',
        component: () => import('@/views/Tenant/TenantDetail.vue'),
        meta: { title: '租户详情', requiresAuth: true, hidden: true, roles: [1, 2] }
      },
      {
        path: '/users',
        name: 'UserList',
        component: () => import('@/views/User/UserList.vue'),
        meta: { title: '终端用户', requiresAuth: true, roles: [1, 2] }
      },
      {
        path: '/finance/recharge',
        name: 'TenantRecharge',
        component: () => import('@/views/Finance/Recharge.vue'),
        meta: { title: '租户充值', requiresAuth: true, hidden: true, roles: [1, 2] }
      },
      {
        path: '/finance/recharge-records',
        name: 'TenantRechargeRecords',
        component: () => import('@/views/Finance/RechargeRecords.vue'),
        meta: { title: '充值记录', requiresAuth: true, roles: [1, 2] }
      },
      {
        path: '/docs/api',
        name: 'ApiDocs',
        component: () => import('@/views/Docs/ApiDocs.vue'),
        meta: { title: '接入文档', requiresAuth: true, roles: [1, 2] }
      },
      {
        path: '/ai-gateway',
        component: () => import('@/views/AIGateway/index.vue'),
        redirect: '/ai-gateway/providers',
        meta: { title: 'AI 网关', requiresAuth: true, roles: [1, 2] },
        children: [
          {
            path: 'providers',
            name: 'GatewayProviders',
            component: () => import('@/views/AIGateway/GatewayProviders.vue'),
            meta: { title: '厂商接入', requiresAuth: true, roles: [1, 2] }
          },
          {
            path: 'models',
            name: 'GatewayModels',
            component: () => import('@/views/AIGateway/GatewayModels.vue'),
            meta: { title: '模型映射', requiresAuth: true, roles: [1, 2] }
          },
          {
            path: 'access',
            name: 'GatewayAccess',
            component: () => import('@/views/AIGateway/GatewayAccess.vue'),
            meta: { title: '模型授权', requiresAuth: true, roles: [1, 2] }
          },
          {
            path: 'usage',
            name: 'GatewayUsage',
            component: () => import('@/views/AIGateway/GatewayUsage.vue'),
            meta: { title: '调用日志', requiresAuth: true, roles: [1, 2] }
          },
          {
            path: 'limits',
            name: 'GatewayLimits',
            component: () => import('@/views/AIGateway/GatewayLimits.vue'),
            meta: { title: '限流策略', requiresAuth: true, roles: [1, 2] }
          },
          {
            path: 'credential-pools',
            name: 'GatewayCredentialPools',
            component: () => import('@/views/AIGateway/GatewayCredentialPools.vue'),
            meta: { title: '账号池', requiresAuth: true, roles: [1, 2] }
          },
          {
            path: 'audit',
            name: 'GatewayAudit',
            component: () => import('@/views/AIGateway/GatewayAudit.vue'),
            meta: { title: '网关审计', requiresAuth: true, roles: [1, 2] }
          },
          {
            path: 'routing',
            name: 'GatewayRouting',
            component: () => import('@/views/AIGateway/GatewayRouting.vue'),
            meta: { title: '路由策略', requiresAuth: true, roles: [1, 2] }
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

router.beforeEach((to) => {
  const authStore = useAuthStore()
  const requiresAuth = to.matched.some(record => record.meta.requiresAuth)

  if (requiresAuth && !authStore.isAuthenticated()) {
    return '/login'
  }

  if (authStore.isAuthenticated() && authStore.userType !== 1 && authStore.userType !== 2) {
    authStore.clearState()
    return '/login'
  }

  if (to.path === '/login' && authStore.isAuthenticated()) {
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
