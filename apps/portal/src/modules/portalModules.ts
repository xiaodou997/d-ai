import type { RouteLocationNormalizedLoaded, RouteRecordRaw } from "vue-router";

import type { AppShellNavItem } from "@/platform";

export type PortalUserType = 1 | 2 | 3 | 4;

export type PortalCapability =
  | "admin.overview"
  | "admin.organization"
  | "admin.identity"
  | "admin.billing"
  | "admin.settlement"
  | "admin.ai.monitor"
  | "admin.ai.upstream"
  | "admin.ai.security"
  | "admin.announcements"
  | "admin.system"
  | "tenant.overview"
  | "tenant.users"
  | "tenant.ai.models"
  | "tenant.ai.plans"
  | "tenant.ai.usage"
  | "tenant.developer"
  | "tenant.workbench"
  | "tenant.account"
  | "tenant.payment"
  | "tenant.operations"
  | "customer.workbench"
  | "customer.services"
  | "customer.usage"
  | "customer.developer"
  | "customer.account"
  | "profile.admin"
  | "profile.tenant"
  | "profile.customer";

// Portal capabilities are a presentation taxonomy for navigation and route
// hints. They are never an authorization decision; every API operation must
// enforce its backend capability/policy independently.
const capabilityUserTypes: Record<PortalCapability, readonly PortalUserType[]> = {
  "admin.overview": [1, 2],
  "admin.organization": [1, 2],
  "admin.identity": [1],
  "admin.billing": [1, 2],
  "admin.settlement": [1, 2],
  "admin.ai.monitor": [1, 2],
  "admin.ai.upstream": [1, 2],
  "admin.ai.security": [1, 2],
  "admin.announcements": [1, 2],
  "admin.system": [1, 2],
  "tenant.overview": [3],
  "tenant.users": [3],
  "tenant.ai.models": [3],
  "tenant.ai.plans": [3],
  "tenant.ai.usage": [3],
  "tenant.developer": [3],
  "tenant.workbench": [3],
  "tenant.account": [3],
  "tenant.payment": [3],
  "tenant.operations": [3],
  "customer.workbench": [4],
  "customer.services": [4],
  "customer.usage": [4],
  "customer.developer": [4],
  "customer.account": [4],
  "profile.admin": [1, 2],
  "profile.tenant": [3],
  "profile.customer": [4]
};

interface PortalNavGroup {
  id: string;
  label: string;
  order: number;
}

export interface PortalModuleTab {
  id: string;
  label: string;
  navLabel?: string;
  icon?: string;
  path: string;
  component: NonNullable<RouteRecordRaw["component"]>;
  name?: string;
  nav?: boolean;
  activeTabId?: string;
  capability?: PortalCapability;
  props?: boolean | Record<string, unknown> | ((route: RouteLocationNormalizedLoaded) => Record<string, unknown>);
}

export interface PortalModule {
  id: string;
  label: string;
  path: string;
  icon: string;
  capability: PortalCapability;
  order: number;
  navGroup?: PortalNavGroup;
  navTabs?: boolean;
  nav?: boolean;
  name?: string;
  props?: boolean | Record<string, unknown> | ((route: RouteLocationNormalizedLoaded) => Record<string, unknown>);
  component?: NonNullable<RouteRecordRaw["component"]>;
  tabs?: PortalModuleTab[];
}

const adminOverview = { id: "admin-overview-category", label: "概览", order: 0 };
const adminOrganization = { id: "admin-organization", label: "组织与权限", order: 10 };
const adminFinance = { id: "admin-finance", label: "资金管理", order: 20 };
const adminAi = { id: "admin-ai", label: "AI 网关", order: 30 };
const adminOperations = { id: "admin-operations", label: "平台运营", order: 40 };
const tenantUsers = { id: "tenant-users", label: "用户与权限", order: 10 };
const tenantAi = { id: "tenant-ai", label: "AI 服务", order: 20 };
const tenantWorkbench = { id: "tenant-workbench", label: "AI 工作台", order: 30 };
const tenantAccount = { id: "tenant-account", label: "账户与设置", order: 40 };
const customerWorkbench = { id: "customer-workbench", label: "AI 工作台", order: 10 };
const customerServices = { id: "customer-services", label: "我的服务", order: 20 };
const customerAccount = { id: "customer-account", label: "账户与开发", order: 30 };

export const portalModules: PortalModule[] = [
  {
    id: "admin-dashboard",
    label: "仪表盘",
    path: "/admin/dashboard",
    icon: "layout-dashboard",
    capability: "admin.overview",
    navGroup: adminOverview,
    order: 0,
    component: () => import("@/views/admin/overview/AdminDashboardView.vue")
  },
  {
    id: "admin-business-overview",
    label: "业务概览",
    path: "/admin/overview/business",
    icon: "bar-chart-3",
    capability: "admin.overview",
    navGroup: adminOverview,
    order: 10,
    component: () => import("@/views/admin/overview/BusinessOverviewView.vue")
  },
  {
    id: "admin-cost-analysis",
    label: "成本分析",
    path: "/admin/overview/cost",
    icon: "banknote",
    capability: "admin.overview",
    navGroup: adminOverview,
    order: 20,
    component: () => import("@/views/admin/overview/CostAnalysisView.vue")
  },
  {
    id: "admin-operations-overview",
    label: "运维监控",
    path: "/admin/overview/operations",
    icon: "gauge",
    capability: "admin.overview",
    navGroup: adminOverview,
    order: 30,
    component: () => import("@/views/admin/overview/OperationsOverviewView.vue")
  },
  {
    id: "admin-organization-workspace",
    label: "租户与用户",
    path: "/admin/organization",
    icon: "building-2",
    capability: "admin.organization",
    navGroup: adminOrganization,
    order: 10,
    navTabs: true,
    tabs: [
      { id: "tenants", label: "租户管理", icon: "building-2", path: "tenants", component: () => import("@/views/admin/platform/TenantsView.vue") },
      { id: "tenant-policy", label: "平台策略", path: "tenants/:id/policy", component: () => import("@/views/admin/platform/TenantPolicyView.vue"), name: "platform-tenant-policy", nav: false, activeTabId: "tenants" },
      { id: "tenant-detail", label: "租户详情", path: "tenants/:id", component: () => import("@/views/admin/platform/TenantDetailView.vue"), name: "platform-tenant-detail", nav: false, activeTabId: "tenants" },
      { id: "users", label: "终端用户", icon: "users", path: "users", component: () => import("@/views/admin/platform/EndUsersView.vue") }
    ]
  },
  {
    id: "admin-identity-workspace",
    label: "管理员与身份",
    path: "/admin/identity",
    icon: "shield-check",
    capability: "admin.identity",
    navGroup: adminOrganization,
    order: 20,
    tabs: [
      { id: "admins", label: "平台管理员", path: "admins", component: () => import("@/views/admin/platform/AdminUsersView.vue") },
      { id: "audit", label: "认证审计", path: "audit", component: () => import("@/views/admin/platform/AuditLogView.vue") },
      { id: "jwt", label: "JWT 密钥", path: "jwt", component: () => import("@/views/admin/platform/JwtKeysView.vue") }
    ]
  },
  {
    id: "admin-settlement-workspace",
    label: "资金中心",
    path: "/admin/settlement",
    icon: "banknote",
    capability: "admin.settlement",
    navGroup: adminFinance,
    order: 20,
    tabs: [
      { id: "recharges", label: "充值订单", icon: "receipt-text", path: "recharges", component: () => import("@/views/admin/platform/RechargeRecordsView.vue") },
      { id: "withdrawals", label: "提现记录", icon: "banknote-arrow-down", path: "withdrawals", component: () => import("@/views/admin/platform/WithdrawalsView.vue") }
    ]
  },
  {
    id: "admin-payment-settings",
    label: "支付设置",
    path: "/admin/settings/payment",
    icon: "credit-card",
    capability: "admin.settlement",
    navGroup: adminFinance,
    order: 30,
    component: () => import("@/views/admin/platform/PaymentSettingsView.vue")
  },
  {
    id: "admin-upstream-accounts",
    label: "上游账号",
    path: "/admin/ai/upstreams/accounts",
    icon: "database",
    capability: "admin.ai.upstream",
    navGroup: adminAi,
    order: 20,
    component: () => import("@/views/admin/ai/gateway/AccountsView.vue")
  },
  {
    id: "admin-credential-pools",
    label: "账号池",
    path: "/admin/ai/upstreams/pools",
    icon: "boxes",
    capability: "admin.ai.upstream",
    navGroup: adminAi,
    order: 21,
    component: () => import("@/views/admin/ai/gateway/CredentialPoolsView.vue")
  },
  {
    id: "admin-price-books",
    label: "价格表",
    path: "/admin/ai/upstreams/pricing",
    icon: "tags",
    capability: "admin.ai.upstream",
    navGroup: adminAi,
    order: 22,
    component: () => import("@/views/admin/ai/gateway/PricingView.vue")
  },
  {
    id: "admin-usage",
    label: "使用记录",
    path: "/admin/ai/usage",
    icon: "scroll-text",
    capability: "admin.ai.security",
    navGroup: adminAi,
    order: 40,
    name: "ai-usage",
    component: () => import("@/views/admin/ai/gateway/UsageView.vue")
  },
  {
    id: "admin-usage-analytics",
    label: "用量分析",
    path: "/admin/ai/analytics",
    icon: "bar-chart-3",
    capability: "admin.ai.monitor",
    navGroup: adminAi,
    order: 45,
    component: () => import("@/views/admin/ai/gateway/UsageAnalyticsView.vue")
  },
  {
    id: "admin-usage-detail",
    label: "使用详情",
    path: "/admin/ai/usage/:requestId",
    icon: "scroll-text",
    capability: "admin.ai.security",
    order: 41,
    nav: false,
    name: "ai-usage-detail",
    component: () => import("@/views/admin/ai/gateway/UsageDetailView.vue")
  },
  {
    id: "admin-security-workspace",
    label: "审计与风控",
    path: "/admin/ai/security",
    icon: "shield-alert",
    capability: "admin.ai.security",
    navGroup: adminAi,
    order: 50,
    tabs: [
      { id: "audit", label: "网关审计", path: "audit", component: () => import("@/views/admin/ai/gateway/AuditView.vue") },
      { id: "risk", label: "风控中心", path: "risk", component: () => import("@/views/admin/ai/gateway/RiskControlView.vue") }
    ]
  },
  {
    id: "admin-announcements",
    label: "公告管理",
    path: "/admin/operations/announcements",
    icon: "megaphone",
    capability: "admin.announcements",
    navGroup: adminOperations,
    order: 10,
    component: () => import("@/views/admin/platform/AnnouncementsView.vue")
  },
  {
    id: "admin-sensitive-information-protection",
    label: "敏感信息保护",
    path: "/admin/system-modules/pii-protection",
    icon: "shield-check",
    capability: "admin.system",
    navGroup: adminOperations,
    order: 20,
    component: () => import("@/views/admin/platform/SensitiveInformationProtectionView.vue")
  },
  {
    id: "admin-proxy-egress",
    label: "代理出口",
    path: "/admin/proxy-nodes",
    icon: "network",
    capability: "admin.system",
    navGroup: adminOperations,
    order: 30,
    component: () => import("@/views/admin/platform/ProxyEgressView.vue")
  },
  {
    id: "admin-system-modules",
    label: "系统模块",
    path: "/admin/system-modules",
    icon: "blocks",
    capability: "admin.system",
    navGroup: adminOperations,
    order: 40,
    component: () => import("@/views/admin/platform/SystemModulesView.vue")
  },
  {
    id: "tenant-overview",
    label: "概览",
    path: "/tenant/overview",
    icon: "layout-dashboard",
    capability: "tenant.overview",
    order: 0,
    navTabs: true,
    tabs: [
      { id: "business", label: "业务概览", icon: "bar-chart-3", path: "business", component: () => import("@/views/tenant/ai/DashboardView.vue") },
      { id: "finance", label: "财务概览", icon: "banknote", path: "finance", component: () => import("@/views/tenant/platform/DashboardView.vue") }
    ]
  },
  {
    id: "tenant-user-workspace",
    label: "用户管理",
    path: "/tenant/users",
    icon: "users",
    capability: "tenant.users",
    navGroup: tenantUsers,
    order: 10,
    navTabs: true,
    tabs: [
      { id: "directory", label: "用户管理", path: "directory", component: () => import("@/views/tenant/platform/UsersView.vue") },
      { id: "user-detail", label: "用户详情", path: "directory/:userId", component: () => import("@/views/tenant/platform/UserDetailView.vue"), name: "tenant-user-detail", nav: false, activeTabId: "directory" }
    ]
  },
  {
    id: "tenant-invitations",
    label: "邀请码",
    path: "/tenant/users/invitations",
    icon: "ticket",
    capability: "tenant.users",
    navGroup: tenantUsers,
    order: 20,
    component: () => import("@/views/tenant/platform/InviteCodesView.vue")
  },
  {
    id: "tenant-model-workspace",
    label: "模型与价格",
    path: "/tenant/ai/models",
    icon: "layers",
    capability: "tenant.ai.models",
    navGroup: tenantAi,
    order: 10,
    tabs: [
      { id: "groups", label: "模型分组", path: "groups", component: () => import("@/views/tenant/ai/GroupManagementView.vue"), name: "ai-groups" },
      { id: "group-detail", label: "分组详情", path: "groups/:groupId", component: () => import("@/views/tenant/ai/GroupDetailView.vue"), name: "ai-group-detail", nav: false, activeTabId: "groups" },
      { id: "prices", label: "租户价格", path: "prices", component: () => import("@/views/tenant/ai/PricesView.vue") },
      { id: "upstreams", label: "上游目录", path: "upstreams", component: () => import("@/views/tenant/ai/UpstreamCatalogView.vue") }
    ]
  },
  {
    id: "tenant-plans",
    label: "套餐管理",
    path: "/tenant/ai/plans",
    icon: "calendar-clock",
    capability: "tenant.ai.plans",
    navGroup: tenantAi,
    order: 20,
    component: () => import("@/views/tenant/ai/SubscriptionManagementView.vue")
  },
  {
    id: "tenant-usage",
    label: "使用分析",
    path: "/tenant/ai/usage",
    icon: "gauge",
    capability: "tenant.ai.usage",
    navGroup: tenantAi,
    order: 30,
    component: () => import("@/views/tenant/ai/UserConsumptionView.vue")
  },
  {
    id: "tenant-developer",
    label: "API 密钥",
    path: "/tenant/developer",
    icon: "key-round",
    capability: "tenant.developer",
    navGroup: tenantAi,
    order: 40,
    tabs: [
      { id: "keys", label: "API 密钥", path: "keys", component: () => import("@/views/tenant/ai/KeysView.vue") },
      { id: "tooling", label: "工具接入指南", path: "tooling", component: () => import("@/views/AiDocsView.vue"), props: { section: "tooling" } }
    ]
  },
  {
    id: "tenant-chat",
    label: "AI 对话",
    path: "/tenant/workbench/chat",
    icon: "message-square",
    capability: "tenant.workbench",
    navGroup: tenantWorkbench,
    order: 10,
    component: () => import("@/views/tenant/ai/chat/ChatWorkspace.vue")
  },
  {
    id: "tenant-images",
    label: "AI 生图",
    path: "/tenant/workbench/images",
    icon: "image",
    capability: "tenant.workbench",
    navGroup: tenantWorkbench,
    order: 20,
    component: () => import("@/views/tenant/ai/ImageStudioView.vue")
  },
  {
    id: "tenant-tasks",
    label: "任务中心",
    path: "/tenant/workbench/tasks",
    icon: "list-checks",
    capability: "tenant.workbench",
    navGroup: tenantWorkbench,
    order: 30,
    component: () => import("@/views/tenant/ai/TasksView.vue")
  },
  {
    id: "tenant-account-center",
    label: "账户中心",
    path: "/tenant/account",
    icon: "wallet",
    capability: "tenant.account",
    navGroup: tenantAccount,
    order: 10,
    component: () => import("@/views/tenant/platform/AccountCenterView.vue")
  },
  {
    id: "tenant-payment",
    label: "支付设置",
    path: "/tenant/payment",
    icon: "credit-card",
    capability: "tenant.payment",
    navGroup: tenantAccount,
    order: 20,
    component: () => import("@/views/tenant/platform/PaymentSettingsView.vue")
  },
  {
    id: "tenant-operations",
    label: "运营设置",
    path: "/tenant/operations",
    icon: "palette",
    capability: "tenant.operations",
    navGroup: tenantAccount,
    order: 30,
    tabs: [
      { id: "branding", label: "品牌设置", path: "branding", component: () => import("@/views/tenant/platform/TenantBrandingView.vue") },
      { id: "announcements", label: "公告管理", path: "announcements", component: () => import("@/views/tenant/platform/AnnouncementsView.vue") }
    ]
  },
  {
    id: "customer-workbench-home",
    label: "工作台",
    path: "/customer/workbench",
    icon: "layout-dashboard",
    capability: "customer.workbench",
    order: 0,
    component: () => import("@/views/customer/ai/WorkspaceView.vue")
  },
  {
    id: "customer-chat",
    label: "AI 对话",
    path: "/customer/workbench/chat",
    icon: "message-square",
    capability: "customer.workbench",
    navGroup: customerWorkbench,
    order: 10,
    component: () => import("@/views/customer/ai/chat/ChatWorkspace.vue")
  },
  {
    id: "customer-images",
    label: "AI 生图",
    path: "/customer/workbench/images",
    icon: "image",
    capability: "customer.workbench",
    navGroup: customerWorkbench,
    order: 20,
    component: () => import("@/views/customer/ai/ImageStudioView.vue")
  },
  {
    id: "customer-tasks",
    label: "任务记录",
    path: "/customer/workbench/tasks",
    icon: "list-checks",
    capability: "customer.workbench",
    navGroup: customerWorkbench,
    order: 30,
    component: () => import("@/views/customer/ai/TasksView.vue")
  },
  {
    id: "customer-service-workspace",
    label: "模型与套餐",
    path: "/customer/services",
    icon: "layers",
    capability: "customer.services",
    navGroup: customerServices,
    order: 10,
    navTabs: true,
    tabs: [
      { id: "models", label: "模型定价", icon: "layers", path: "models", component: () => import("@/views/customer/ai/GroupsView.vue") },
      { id: "subscription", label: "订阅套餐", icon: "calendar-clock", path: "subscription", component: () => import("@/views/customer/ai/SubscriptionView.vue") }
    ]
  },
  {
    id: "customer-usage",
    label: "使用记录",
    path: "/customer/usage",
    icon: "scroll-text",
    capability: "customer.usage",
    navGroup: customerServices,
    order: 20,
    component: () => import("@/views/customer/ai/UsageRecordsView.vue")
  },
  {
    id: "customer-developer",
    label: "API 密钥",
    path: "/customer/developer",
    icon: "key-round",
    capability: "customer.developer",
    navGroup: customerAccount,
    order: 10,
    tabs: [
      { id: "keys", label: "API 密钥", path: "keys", component: () => import("@/views/customer/ai/KeysView.vue") },
      { id: "tooling", label: "工具接入指南", path: "tooling", component: () => import("@/views/AiDocsView.vue"), props: { section: "tooling" } }
    ]
  },
  {
    id: "customer-account-center",
    label: "账户中心",
    path: "/customer/account",
    icon: "wallet",
    capability: "customer.account",
    navGroup: customerAccount,
    order: 20,
    tabs: [
      { id: "overview", label: "USD 账户", path: "overview", component: () => import("@/views/customer/platform/AccountView.vue") },
      { id: "topup", label: "额度充值", path: "topup", component: () => import("@/views/customer/platform/TopupView.vue") },
      { id: "recharges", label: "充值记录", path: "recharges", component: () => import("@/views/customer/platform/RechargeView.vue") }
    ]
  },
  {
    id: "admin-profile",
    label: "个人资料",
    path: "/admin/profile",
    icon: "user",
    capability: "profile.admin",
    order: 100,
    nav: false,
    component: () => import("@/views/admin/ProfileView.vue")
  },
  {
    id: "tenant-profile",
    label: "个人资料",
    path: "/tenant/profile",
    icon: "user",
    capability: "profile.tenant",
    order: 100,
    nav: false,
    component: () => import("@/views/tenant/ProfileView.vue")
  },
  {
    id: "customer-profile",
    label: "个人资料",
    path: "/customer/profile",
    icon: "user",
    capability: "profile.customer",
    order: 100,
    nav: false,
    component: () => import("@/views/customer/ProfileView.vue")
  }
];

export const portalModulesById = new Map(portalModules.map((module) => [module.id, module]));

export function allowedUserTypesForCapability(capability: PortalCapability): PortalUserType[] {
  return [...capabilityUserTypes[capability]];
}

export function userHasPortalCapability(userType: number, capability: string): boolean {
  const allowedUserTypes = capabilityUserTypes[capability as PortalCapability];
  return Boolean(allowedUserTypes?.includes(userType as PortalUserType));
}

export function defaultPortalPathForUserType(userType: number): string {
  if (userType === 1 || userType === 2) return "/admin/dashboard";
  if (userType === 3) return "/tenant/overview/business";
  return "/customer/workbench";
}

export function profilePathForUserType(userType: number): string {
  if (userType === 1 || userType === 2) return "/admin/profile";
  if (userType === 3) return "/tenant/profile";
  return "/customer/profile";
}

function pathMatches(currentPath: string, targetPath: string): boolean {
  return currentPath === targetPath || currentPath.startsWith(`${targetPath}/`);
}

function moduleTabNavItems(
  module: PortalModule,
  userType: number,
  currentPath: string
): AppShellNavItem[] {
  return (module.tabs ?? [])
    .filter(
      (tab) => tab.nav !== false && userHasPortalCapability(userType, tab.capability ?? module.capability)
    )
    .map((tab, index) => {
      const tabPath = `${module.path}/${tab.path.split("/:", 1)[0]}`;
      return {
        id: `${module.id}-${tab.id}`,
        label: tab.navLabel ?? tab.label,
        to: tabPath,
        icon: tab.icon ?? module.icon,
        active:
          pathMatches(currentPath, tabPath) ||
          (currentPath === module.path && index === 0)
      };
    });
}

export function buildPortalNav(userType: number, currentPath: string): AppShellNavItem[] {
  const visibleModules = portalModules
    .filter((module) => module.nav !== false && userHasPortalCapability(userType, module.capability))
    .sort((left, right) => left.order - right.order);
  const activeModuleId = visibleModules
    .filter((module) => pathMatches(currentPath, module.path))
    .sort((left, right) => right.path.length - left.path.length)[0]?.id;

  const direct = visibleModules
    .filter((module) => !module.navGroup && !module.navTabs)
    .map((module) => ({
      id: module.id,
      label: module.label,
      to: module.path,
      icon: module.icon,
      active: module.id === activeModuleId
    }));

  const tabCategories = visibleModules
    .filter((module) => module.navTabs && !module.navGroup)
    .map((module) => ({
      id: `${module.id}-category`,
      label: module.label,
      active: module.id === activeModuleId,
      children: moduleTabNavItems(module, userType, currentPath)
    }));

  const groups = new Map<string, { group: PortalNavGroup; modules: PortalModule[] }>();
  for (const module of visibleModules) {
    if (!module.navGroup) continue;
    const existing = groups.get(module.navGroup.id);
    if (existing) existing.modules.push(module);
    else groups.set(module.navGroup.id, { group: module.navGroup, modules: [module] });
  }

  return [
    ...tabCategories,
    ...direct,
    ...[...groups.values()]
      .sort((left, right) => left.group.order - right.group.order)
      .map(({ group, modules }) => ({
        id: group.id,
        label: group.label,
        active: modules.some((module) => module.id === activeModuleId),
        children: modules
          .sort((left, right) => left.order - right.order)
          .flatMap((module) =>
            module.navTabs
              ? moduleTabNavItems(module, userType, currentPath)
              : [{
                  id: module.id,
                  label: module.label,
                  to: module.path,
                  icon: module.icon,
                  active: module.id === activeModuleId
                }]
          )
      }))
  ];
}

function routeMeta(capability: PortalCapability, title: string, extra: Record<string, unknown> = {}) {
  return {
    title,
    capability,
    allowedUserTypes: allowedUserTypesForCapability(capability),
    ...extra
  };
}

export function buildPortalModuleRoutes(): RouteRecordRaw[] {
  return portalModules.map((module) => {
    const path = module.path.replace(/^\//, "");
    if (!module.tabs?.length) {
      if (!module.component) throw new Error(`Portal module ${module.id} has no component`);
      return {
        path,
        name: module.name,
        component: module.component,
        props: module.props,
        meta: routeMeta(module.capability, module.label, { portalModuleId: module.id })
      };
    }

    const firstTab = module.tabs.find((tab) => tab.nav !== false);
    if (!firstTab) throw new Error(`Portal module ${module.id} has no navigable tab`);

    return {
      path,
      component: () => import("@/modules/PortalWorkspaceLayout.vue"),
      props: { moduleId: module.id },
      meta: routeMeta(module.capability, module.label, { portalModuleId: module.id }),
      children: [
        { path: "", redirect: `${module.path}/${firstTab.path.split("/:", 1)[0]}` },
        ...module.tabs.map((tab) => {
          const capability = tab.capability ?? module.capability;
          return {
            path: tab.path,
            name: tab.name,
            component: tab.component,
            props: tab.props,
            meta: routeMeta(capability, tab.label, {
              portalModuleId: module.id,
              portalTabId: tab.activeTabId ?? tab.id
            })
          } satisfies RouteRecordRaw;
        })
      ]
    };
  });
}
