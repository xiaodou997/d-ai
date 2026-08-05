import { createRouter, createWebHistory, type RouteRecordRaw } from "vue-router";
import { attachPortalSSOGuard, type BackendService } from "@/platform";

import { portalEnv } from "./env";
import { useAuthStore } from "./stores/auth";
import LayoutView from "./views/LayoutView.vue";

// 路由表 —— 合并三端路由，按 userType 在路由守卫中过滤
const routes: RouteRecordRaw[] = [
  {
    path: "/",
    component: LayoutView,
    children: [
      { path: "", redirect: "/overview" },

      // ── 共享页面 ──
      { path: "overview", component: () => import("./views/OverviewView.vue"), meta: { title: "概览" } },
      { path: "profile", component: () => import("./views/admin/ProfileView.vue"), meta: { title: "个人资料" } },

      // ── URM 管理端（userType 1/2）──
      { path: "urm/tenants", component: () => import("./views/admin/urm/TenantsView.vue"), meta: { service: "urm" as BackendService, title: "租户管理", allowedUserTypes: [1, 2] } },
      { path: "urm/tenants/:id", component: () => import("./views/admin/urm/TenantDetailView.vue"), meta: { service: "urm", title: "租户详情", allowedUserTypes: [1, 2] } },
      { path: "urm/users", component: () => import("./views/admin/urm/EndUsersView.vue"), meta: { service: "urm", title: "终端用户", allowedUserTypes: [1, 2] } },
      { path: "urm/invitations", component: () => import("./views/tenant/urm/InviteCodesView.vue"), meta: { service: "urm", title: "邀请码", allowedUserTypes: [3] } },
      { path: "urm/dashboard", component: () => import("./views/admin/urm/DashboardView.vue"), meta: { service: "urm", title: "管理大盘", allowedUserTypes: [1, 2] } },
      { path: "urm/admins", component: () => import("./views/admin/urm/AdminUsersView.vue"), meta: { service: "urm", title: "平台管理员", allowedUserTypes: [1] } },
      { path: "urm/audit-log", component: () => import("./views/admin/urm/AuditLogView.vue"), meta: { service: "urm", title: "认证审计", allowedUserTypes: [1] } },
      { path: "urm/jwt-keys", component: () => import("./views/admin/urm/JwtKeysView.vue"), meta: { service: "urm", title: "JWT 密钥", allowedUserTypes: [1] } },
      { path: "urm/announcements", component: () => import("./views/admin/urm/AnnouncementsView.vue"), meta: { service: "urm", title: "公告管理", allowedUserTypes: [1, 2] } },

      // ── 财务管理（userType 1/2/3）──
      { path: "billing/tenant-credit", component: () => import("./views/admin/urm/AccountOverviewView.vue"), meta: { service: "urm", title: "租户积分", allowedUserTypes: [1, 2, 3] } },
      { path: "billing/recharge", component: () => import("./views/admin/urm/RechargeRecordsView.vue"), meta: { service: "urm", title: "充值记录", allowedUserTypes: [1, 2, 3, 4] } },
      { path: "billing/transactions", component: () => import("./views/admin/urm/TransactionsView.vue"), meta: { service: "urm", title: "积分明细", allowedUserTypes: [1, 2, 3, 4] } },
      { path: "billing/payment-settings", component: () => import("./views/admin/urm/PaymentSettingsView.vue"), meta: { service: "urm", title: "支付配置", allowedUserTypes: [1, 2] } },
      { path: "billing/payment-orders", component: () => import("./views/admin/urm/PaymentOrdersView.vue"), meta: { service: "urm", title: "支付订单", allowedUserTypes: [1, 2] } },
      { path: "billing/withdrawals", component: () => import("./views/admin/urm/WithdrawalsView.vue"), meta: { service: "urm", title: "提现审核", allowedUserTypes: [1, 2] } },
      { path: "billing/cash-accounts", component: () => import("./views/admin/urm/CashAccountsView.vue"), meta: { service: "urm", title: "现金账户", allowedUserTypes: [1, 2, 3] } },

      // ── AI 网关管理（userType 1/2）──
      { path: "ai-gateway/overview", component: () => import("./views/admin/ai/DashboardView.vue"), meta: { service: "ai" as BackendService, title: "数据大盘", allowedUserTypes: [1, 2] } },
      { path: "ai-gateway/status", component: () => import("./views/admin/ai/SystemStatusView.vue"), meta: { service: "ai", title: "系统状态", allowedUserTypes: [1, 2] } },
      { path: "ai-gateway/pricing", component: () => import("./views/admin/ai/gateway/PricingView.vue"), meta: { service: "ai", title: "价格表", allowedUserTypes: [1, 2] } },
      { path: "ai-gateway/accounts", component: () => import("./views/admin/ai/gateway/AccountsView.vue"), meta: { service: "ai", title: "上游账号", allowedUserTypes: [1, 2] } },
      { path: "ai-gateway/credential-pools", component: () => import("./views/admin/ai/gateway/CredentialPoolsView.vue"), meta: { service: "ai", title: "账号池", allowedUserTypes: [1, 2] } },
      { path: "ai-gateway/access", component: () => import("./views/admin/ai/gateway/AccessView.vue"), meta: { service: "ai", title: "租户策略", allowedUserTypes: [1, 2] } },
      { path: "ai-gateway/usage", component: () => import("./views/admin/ai/gateway/UsageView.vue"), meta: { service: "ai", title: "使用记录", allowedUserTypes: [1, 2, 3, 4] } },
      { path: "ai-gateway/usage/:id", component: () => import("./views/admin/ai/gateway/UsageDetailView.vue"), meta: { service: "ai", title: "使用详情", allowedUserTypes: [1, 2] } },
      { path: "ai-gateway/audit", component: () => import("./views/admin/ai/gateway/AuditView.vue"), meta: { service: "ai", title: "网关审计", allowedUserTypes: [1, 2] } },
      { path: "ai-gateway/risk-control", component: () => import("./views/admin/ai/gateway/RiskControlView.vue"), meta: { service: "ai", title: "风控中心", allowedUserTypes: [1, 2] } },
      { path: "ai-gateway/subscriptions", component: () => import("./views/tenant/ai/SubscriptionManagementView.vue"), meta: { service: "ai", title: "订阅套餐", allowedUserTypes: [1, 2, 3] } },

      // ── 租户 AI 运营（userType 3）──
      { path: "workspace/groups", component: () => import("./views/tenant/ai/GroupManagementView.vue"), meta: { service: "ai", title: "模型分组", allowedUserTypes: [3] } },
      { path: "workspace/groups/:id", component: () => import("./views/tenant/ai/GroupDetailView.vue"), meta: { service: "ai", title: "分组详情", allowedUserTypes: [3] } },
      { path: "workspace/keys", component: () => import("./views/tenant/ai/KeysView.vue"), meta: { service: "ai", title: "密钥管理", allowedUserTypes: [3, 4] } },
      { path: "workspace/api-keys", component: () => import("./views/tenant/ai/ApiKeysView.vue"), meta: { service: "ai", title: "API 密钥", allowedUserTypes: [3, 4] } },
      { path: "workspace/usage-records", component: () => import("./views/tenant/ai/UserConsumptionView.vue"), meta: { service: "ai", title: "使用记录", allowedUserTypes: [3, 4] } },
      { path: "workspace/subscription", component: () => import("./views/customer/ai/SubscriptionView.vue"), meta: { service: "ai", title: "订阅套餐", allowedUserTypes: [4] } },

      // ── 终端用户工作台（userType 4）──
      { path: "workspace", component: () => import("./views/customer/ai/WorkspaceView.vue"), meta: { service: "ai", title: "工作台", allowedUserTypes: [4] } },
      { path: "workspace/chat", component: () => import("./views/customer/ai/chat/ChatWorkspace.vue"), meta: { service: "ai", title: "AI 对话", allowedUserTypes: [4] } },
      { path: "workspace/images", component: () => import("./views/customer/ai/ImageStudioView.vue"), meta: { service: "ai", title: "AI 生图", allowedUserTypes: [4] } },
      { path: "workspace/tasks", component: () => import("./views/customer/ai/TasksView.vue"), meta: { service: "ai", title: "我的任务", allowedUserTypes: [4] } },

      // ── 租户自助 URM ──
      { path: "account", component: () => import("./views/tenant/urm/AccountCenterView.vue"), meta: { service: "urm", title: "积分账户", allowedUserTypes: [3, 4] } },
      { path: "billing/topup", component: () => import("./views/customer/urm/TopupView.vue"), meta: { service: "urm", title: "充值积分", allowedUserTypes: [3, 4] } },

      // ── 帮助 ──
      { path: "help", component: () => import("./views/PlaceholderView.vue"), meta: { title: "使用说明" } },
    ]
  },
  { path: "/login", component: () => import("./views/LoginView.vue"), meta: { public: true, title: "登录" } },
  { path: "/:pathMatch(.*)*", redirect: "/overview" }
];

const router = createRouter({
  history: createWebHistory(),
  routes
});

attachPortalSSOGuard(router, {
  env: portalEnv as any,
  useAuthStore: useAuthStore as any
});

export default router;
