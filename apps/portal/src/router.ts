import { createRouter, createWebHistory, type RouteRecordRaw } from "vue-router";
import { attachPortalAuthGuard } from "@/platform";

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

      // ── 平台管理（userType 1/2）──
      { path: "platform/tenants", component: () => import("./views/admin/platform/TenantsView.vue"), meta: { title: "租户管理", allowedUserTypes: [1, 2] } },
      { path: "platform/tenants/:id", component: () => import("./views/admin/platform/TenantDetailView.vue"), meta: { title: "租户详情", allowedUserTypes: [1, 2] } },
      { path: "platform/users", component: () => import("./views/admin/platform/EndUsersView.vue"), meta: { title: "终端用户", allowedUserTypes: [1, 2] } },
      { path: "platform/invitations", component: () => import("./views/tenant/platform/InviteCodesView.vue"), meta: { title: "邀请码", allowedUserTypes: [3] } },
      { path: "platform/dashboard", component: () => import("./views/admin/platform/DashboardView.vue"), meta: { title: "管理大盘", allowedUserTypes: [1, 2] } },
      { path: "platform/admins", component: () => import("./views/admin/platform/AdminUsersView.vue"), meta: { title: "平台管理员", allowedUserTypes: [1] } },
      { path: "platform/audit-log", component: () => import("./views/admin/platform/AuditLogView.vue"), meta: { title: "认证审计", allowedUserTypes: [1] } },
      { path: "platform/jwt-keys", component: () => import("./views/admin/platform/JwtKeysView.vue"), meta: { title: "JWT 密钥", allowedUserTypes: [1] } },
      { path: "platform/announcements", component: () => import("./views/admin/platform/AnnouncementsView.vue"), meta: { title: "公告管理", allowedUserTypes: [1, 2] } },

      // ── 财务管理（userType 1/2/3）──
      { path: "billing/tenant-credit", component: () => import("./views/admin/platform/AccountOverviewView.vue"), meta: { title: "租户积分", allowedUserTypes: [1, 2, 3] } },
      { path: "billing/recharge", component: () => import("./views/admin/platform/RechargeRecordsView.vue"), meta: { title: "充值记录", allowedUserTypes: [1, 2, 3, 4] } },
      { path: "billing/transactions", component: () => import("./views/admin/platform/TransactionsView.vue"), meta: { title: "积分明细", allowedUserTypes: [1, 2, 3, 4] } },
      { path: "billing/payment-settings", component: () => import("./views/admin/platform/PaymentSettingsView.vue"), meta: { title: "支付配置", allowedUserTypes: [1, 2] } },
      { path: "billing/payment-orders", component: () => import("./views/admin/platform/PaymentOrdersView.vue"), meta: { title: "支付订单", allowedUserTypes: [1, 2] } },
      { path: "billing/withdrawals", component: () => import("./views/admin/platform/WithdrawalsView.vue"), meta: { title: "提现审核", allowedUserTypes: [1, 2] } },
      { path: "billing/cash-accounts", component: () => import("./views/admin/platform/CashAccountsView.vue"), meta: { title: "现金账户", allowedUserTypes: [1, 2, 3] } },

      // ── AI 网关管理（userType 1/2）──
      { path: "ai-gateway/overview", component: () => import("./views/admin/ai/DashboardView.vue"), meta: { title: "数据大盘", allowedUserTypes: [1, 2] } },
      { path: "ai-gateway/status", component: () => import("./views/admin/ai/SystemStatusView.vue"), meta: { title: "系统状态", allowedUserTypes: [1, 2] } },
      { path: "ai-gateway/pricing", component: () => import("./views/admin/ai/gateway/PricingView.vue"), meta: { title: "价格表", allowedUserTypes: [1, 2] } },
      { path: "ai-gateway/accounts", component: () => import("./views/admin/ai/gateway/AccountsView.vue"), meta: { title: "上游账号", allowedUserTypes: [1, 2] } },
      { path: "ai-gateway/credential-pools", component: () => import("./views/admin/ai/gateway/CredentialPoolsView.vue"), meta: { title: "账号池", allowedUserTypes: [1, 2] } },
      { path: "ai-gateway/access", component: () => import("./views/admin/ai/gateway/AccessView.vue"), meta: { title: "租户策略", allowedUserTypes: [1, 2] } },
      { path: "ai-gateway/usage", component: () => import("./views/admin/ai/gateway/UsageView.vue"), meta: { title: "使用记录", allowedUserTypes: [1, 2, 3, 4] } },
      { path: "ai-gateway/usage/:id", component: () => import("./views/admin/ai/gateway/UsageDetailView.vue"), meta: { title: "使用详情", allowedUserTypes: [1, 2] } },
      { path: "ai-gateway/audit", component: () => import("./views/admin/ai/gateway/AuditView.vue"), meta: { title: "网关审计", allowedUserTypes: [1, 2] } },
      { path: "ai-gateway/risk-control", component: () => import("./views/admin/ai/gateway/RiskControlView.vue"), meta: { title: "风控中心", allowedUserTypes: [1, 2] } },
      { path: "ai-gateway/subscriptions", component: () => import("./views/tenant/ai/SubscriptionManagementView.vue"), meta: { title: "订阅套餐", allowedUserTypes: [1, 2, 3] } },

      // ── 租户 AI 运营（userType 3）──
      { path: "workspace/groups", component: () => import("./views/tenant/ai/GroupManagementView.vue"), meta: { title: "模型分组", allowedUserTypes: [3] } },
      { path: "workspace/groups/:id", component: () => import("./views/tenant/ai/GroupDetailView.vue"), meta: { title: "分组详情", allowedUserTypes: [3] } },
      { path: "workspace/keys", component: () => import("./views/tenant/ai/KeysView.vue"), meta: { title: "密钥管理", allowedUserTypes: [3, 4] } },
      { path: "workspace/api-keys", component: () => import("./views/tenant/ai/ApiKeysView.vue"), meta: { title: "API 密钥", allowedUserTypes: [3, 4] } },
      { path: "workspace/usage-records", component: () => import("./views/tenant/ai/UserConsumptionView.vue"), meta: { title: "使用记录", allowedUserTypes: [3, 4] } },
      { path: "workspace/subscription", component: () => import("./views/customer/ai/SubscriptionView.vue"), meta: { title: "订阅套餐", allowedUserTypes: [4] } },

      // ── 终端用户工作台（userType 4）──
      { path: "workspace", component: () => import("./views/customer/ai/WorkspaceView.vue"), meta: { title: "工作台", allowedUserTypes: [4] } },
      { path: "workspace/chat", component: () => import("./views/customer/ai/chat/ChatWorkspace.vue"), meta: { title: "AI 对话", allowedUserTypes: [4] } },
      { path: "workspace/images", component: () => import("./views/customer/ai/ImageStudioView.vue"), meta: { title: "AI 生图", allowedUserTypes: [4] } },
      { path: "workspace/tasks", component: () => import("./views/customer/ai/TasksView.vue"), meta: { title: "我的任务", allowedUserTypes: [4] } },

      // ── 租户自助 Platform ──
      { path: "account", component: () => import("./views/tenant/platform/AccountCenterView.vue"), meta: { title: "积分账户", allowedUserTypes: [3, 4] } },
      { path: "billing/topup", component: () => import("./views/customer/platform/TopupView.vue"), meta: { title: "充值积分", allowedUserTypes: [3, 4] } },

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

attachPortalAuthGuard(router, {
  env: portalEnv as any,
  useAuthStore: useAuthStore as any
});

export default router;
