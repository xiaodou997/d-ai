import type { AppShellNavItem, BackendService } from "@dai/app-core";

/**
 * 统一菜单 —— 合并三端菜单，按 userType 过滤可见性。
 *
 * userType:
 *   1 = 超级管理员（全部可见）
 *   2 = 平台管理员（全部，除超管专属）
 *   3 = 租户（租户自助 + AI 运营）
 *   4 = 终端用户（用户中心 + AI 工作台）
 */

interface MenuLeaf {
  id: string;
  label: string;
  to: string;
  icon?: string;
  userTypes: number[];
}

interface MenuGroup {
  id: string;
  label: string;
  children: MenuLeaf[];
}

interface BusinessModule {
  id: BackendService;
  label: string;
  userTypes: number[];
  groups: MenuGroup[];
}

// ===== 用户中心（所有人可见，功能按 userType 分化） =====
const urmModule: BusinessModule = {
  id: "urm",
  label: "用户中心",
  userTypes: [1, 2, 3, 4],
  groups: [
    {
      id: "urm-account",
      label: "积分账户",
      children: [
        { id: "urm-my-account", label: "我的账户", to: "/account", icon: "wallet", userTypes: [3, 4] },
        { id: "urm-tenant-credit", label: "租户积分", to: "/billing/tenant-credit", icon: "wallet", userTypes: [1, 2, 3] }
      ]
    },
    {
      id: "urm-recharge",
      label: "充值与明细",
      children: [
        { id: "urm-topup", label: "充值积分", to: "/billing/topup", icon: "credit-card", userTypes: [3, 4] },
        { id: "urm-recharge-orders", label: "充值记录", to: "/billing/recharge", icon: "receipt-text", userTypes: [1, 2, 3, 4] },
        { id: "urm-transactions", label: "积分明细", to: "/billing/transactions", icon: "arrow-left-right", userTypes: [1, 2, 3, 4] }
      ]
    },
    {
      id: "urm-finance",
      label: "财务管理",
      children: [
        { id: "urm-payment-settings", label: "支付配置", to: "/billing/payment-settings", icon: "settings", userTypes: [1, 2] },
        { id: "urm-payment-orders", label: "支付订单", to: "/billing/payment-orders", icon: "receipt", userTypes: [1, 2] },
        { id: "urm-withdrawals", label: "提现审核", to: "/billing/withdrawals", icon: "banknote", userTypes: [1, 2] },
        { id: "urm-cash-accounts", label: "现金账户", to: "/billing/cash-accounts", icon: "banknote", userTypes: [1, 2, 3] }
      ]
    },
    {
      id: "urm-users",
      label: "用户管理",
      children: [
        { id: "urm-tenants", label: "租户管理", to: "/urm/tenants", icon: "building", userTypes: [1, 2] },
        { id: "urm-end-users", label: "终端用户", to: "/urm/users", icon: "users", userTypes: [1, 2, 3] },
        { id: "urm-invitations", label: "邀请码", to: "/urm/invitations", icon: "ticket", userTypes: [3] }
      ]
    },
    {
      id: "urm-security",
      label: "安全审计",
      children: [
        { id: "urm-audit-log", label: "认证审计", to: "/security/audit-log", icon: "shield", userTypes: [1] },
        { id: "urm-admins", label: "平台管理员", to: "/security/admins", icon: "user-cog", userTypes: [1] }
      ]
    },
    {
      id: "urm-settings",
      label: "账户设置",
      children: [
        { id: "urm-profile", label: "个人资料", to: "/profile", icon: "user", userTypes: [1, 2, 3, 4] }
      ]
    }
  ]
};

// ===== 智能服务 =====
const aiModule: BusinessModule = {
  id: "ai",
  label: "智能服务",
  userTypes: [1, 2, 3, 4],
  groups: [
    {
      id: "ai-monitor",
      label: "数据监控",
      children: [
        { id: "ai-dashboard", label: "数据大盘", to: "/ai-gateway/overview", icon: "bar-chart-3", userTypes: [1, 2] },
        { id: "ai-system-status", label: "系统状态", to: "/ai-gateway/status", icon: "heart-pulse", userTypes: [1, 2] }
      ]
    },
    {
      id: "ai-gateway-config",
      label: "网关配置",
      children: [
        { id: "ai-pricing", label: "价格表", to: "/ai-gateway/pricing", icon: "tags", userTypes: [1, 2] },
        { id: "ai-accounts", label: "上游账号", to: "/ai-gateway/accounts", icon: "database", userTypes: [1, 2] },
        { id: "ai-credential-pools", label: "账号池", to: "/ai-gateway/credential-pools", icon: "boxes", userTypes: [1, 2] }
      ]
    },
    {
      id: "ai-operations",
      label: "运营管理",
      children: [
        { id: "ai-access", label: "租户策略", to: "/ai-gateway/access", icon: "sliders-horizontal", userTypes: [1, 2] },
        { id: "ai-subscription-plans", label: "订阅套餐", to: "/ai-gateway/subscriptions", icon: "calendar-clock", userTypes: [1, 2, 3] }
      ]
    },
    {
      id: "ai-audit",
      label: "日志审计",
      children: [
        { id: "ai-usage", label: "使用记录", to: "/ai-gateway/usage", icon: "scroll-text", userTypes: [1, 2, 3, 4] },
        { id: "ai-audit-log", label: "网关审计", to: "/ai-gateway/audit", icon: "file-text", userTypes: [1, 2] },
        { id: "ai-risk-control", label: "风控中心", to: "/ai-gateway/risk-control", icon: "shield-alert", userTypes: [1, 2] }
      ]
    },
    {
      id: "ai-workspace",
      label: "工作台",
      children: [
        { id: "ai-ws", label: "工作台", to: "/workspace", icon: "layout-dashboard", userTypes: [4] },
        { id: "ai-chat", label: "AI 对话", to: "/workspace/chat", icon: "message-square", userTypes: [4] },
        { id: "ai-images", label: "AI 生图", to: "/workspace/images", icon: "image", userTypes: [4] },
        { id: "ai-tasks", label: "我的任务", to: "/workspace/tasks", icon: "list-checks", userTypes: [4] }
      ]
    },
    {
      id: "ai-services",
      label: "我的服务",
      children: [
        { id: "ai-groups", label: "模型定价", to: "/workspace/groups", icon: "tags", userTypes: [3, 4] },
        { id: "ai-subscription", label: "订阅套餐", to: "/workspace/subscription", icon: "calendar-clock", userTypes: [4] },
        { id: "ai-usage-records", label: "使用记录", to: "/workspace/usage-records", icon: "scroll-text", userTypes: [3, 4] }
      ]
    },
    {
      id: "ai-keys",
      label: "开发接入",
      children: [
        { id: "ai-api-keys", label: "密钥管理", to: "/workspace/keys", icon: "key-round", userTypes: [3, 4] }
      ]
    }
  ]
};

const allModules: BusinessModule[] = [urmModule, aiModule];

/**
 * 根据 userType 过滤菜单，返回 AppShellNavItem 格式
 */
export function buildUnifiedNav(userType: number): AppShellNavItem[] {
  return allModules
    .filter((m) => m.userTypes.includes(userType))
    .map((m) => ({
      id: m.id,
      label: m.label,
      children: m.groups
        .map((g) => ({
          id: g.id,
          label: g.label,
          children: g.children
            .filter((c) => c.userTypes.includes(userType))
            .map((c) => ({
              id: c.id,
              label: c.label,
              to: c.to,
              icon: c.icon
            }))
        }))
        .filter((g) => g.children.length > 0)
    }))
    .filter((m) => (m.children as any[]).length > 0);
}
