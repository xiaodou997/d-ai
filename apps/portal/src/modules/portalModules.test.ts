import { describe, expect, it } from "vitest";

import {
  buildPortalNav,
  defaultPortalPathForUserType,
  portalModules,
  profilePathForUserType,
  userHasPortalCapability
} from "./portalModules";

function leavesFor(userType: number, path = defaultPortalPathForUserType(userType)) {
  return buildPortalNav(userType, path).flatMap((node) => (node.to ? [node] : (node.children ?? [])));
}

describe("portal module registry", () => {
  it("keeps the destructive V2 navigation within the agreed role budgets", () => {
    expect(leavesFor(1)).toHaveLength(19);
    expect(leavesFor(2)).toHaveLength(18);
    expect(leavesFor(3)).toHaveLength(14);
    expect(leavesFor(4)).toHaveLength(9);
  });

  it("builds separate admin overview pages instead of legacy page entries", () => {
    expect(leavesFor(1).map((item) => item.label)).toEqual([
      "仪表盘",
      "业务概览",
      "成本分析",
      "运维监控",
      "租户管理",
      "终端用户",
      "管理员与身份",
      "资金中心",
      "支付设置",
      "上游账号",
      "账号池",
      "价格表",
      "使用记录",
      "用量分析",
      "审计与风控",
      "公告管理",
      "敏感信息保护",
      "代理出口",
      "系统模块"
    ]);
    expect(leavesFor(4).map((item) => item.label)).toEqual([
      "工作台",
      "AI 对话",
      "AI 生图",
      "任务记录",
      "模型定价",
      "订阅套餐",
      "使用记录",
      "API 密钥",
      "账户中心"
    ]);
  });

  it("keeps health details inside the operations overview instead of a separate menu", () => {
    const adminOverview = buildPortalNav(1, "/admin/overview/operations")[0];
    const tenantOverview = buildPortalNav(3, "/tenant/overview/business")[0];

    expect(portalModules.some((module) => module.path === "/admin/overview")).toBe(false);
    expect(portalModules.some((module) => module.path === "/admin/overview/health")).toBe(false);
    expect(portalModules.some((module) => module.path === "/admin/ai/monitoring")).toBe(false);

    expect(adminOverview).toMatchObject({
      label: "概览",
      active: true,
      children: [
        { label: "仪表盘", to: "/admin/dashboard", active: false },
        { label: "业务概览", to: "/admin/overview/business", active: false },
        { label: "成本分析", to: "/admin/overview/cost", active: false },
        { label: "运维监控", to: "/admin/overview/operations", active: true }
      ]
    });
    expect(tenantOverview).toMatchObject({
      label: "概览",
      active: true,
      children: [
        { label: "业务概览", to: "/tenant/overview/business", active: true },
        { label: "财务概览", to: "/tenant/overview/finance", active: false }
      ]
    });
  });

  it("exposes sensitive information protection and proxy egress as operations menus", () => {
    const operationsMenus = buildPortalNav(1, "/admin/proxy-nodes")
      .find((item) => item.id === "admin-operations")
      ?.children;

    expect(operationsMenus).toMatchObject([
      { id: "admin-announcements", label: "公告管理", active: false },
      {
        id: "admin-sensitive-information-protection",
        label: "敏感信息保护",
        to: "/admin/system-modules/pii-protection",
        icon: "shield-check",
        active: false
      },
      {
        id: "admin-proxy-egress",
        label: "代理出口",
        to: "/admin/proxy-nodes",
        icon: "network",
        active: true
      },
      { id: "admin-system-modules", label: "系统模块", active: false }
    ]);
  });

  it("marks a workspace active for any child or detail route", () => {
    const activeTenant = leavesFor(3, "/tenant/users/directory/user-1").find((item) => item.active);
    const activeAdmin = leavesFor(1, "/admin/ai/usage/request-1").find((item) => item.active);

    expect(activeTenant?.id).toBe("tenant-user-workspace-directory");
    expect(activeAdmin?.id).toBe("admin-usage");
    expect(leavesFor(4, "/customer/workbench/chat").filter((item) => item.active).map((item) => item.id)).toEqual([
      "customer-chat"
    ]);
  });

  it("exposes user management and invitations as separate tenant menus", () => {
    const userMenus = buildPortalNav(3, "/tenant/users/invitations")
      .find((item) => item.id === "tenant-users")
      ?.children;

    expect(userMenus).toMatchObject([
      {
        id: "tenant-user-workspace-directory",
        label: "用户管理",
        to: "/tenant/users/directory",
        icon: "users",
        active: false
      },
      {
        id: "tenant-invitations",
        label: "邀请码",
        to: "/tenant/users/invitations",
        icon: "ticket",
        active: true
      }
    ]);

    const userWorkspace = portalModules.find((module) => module.id === "tenant-user-workspace");
    expect(userWorkspace?.tabs?.filter((tab) => tab.nav !== false).map((tab) => tab.label)).toEqual([
      "用户管理"
    ]);
  });

  it("exposes model pricing and subscriptions as separate customer menus", () => {
    const serviceMenus = buildPortalNav(4, "/customer/services/subscription")
      .find((item) => item.id === "customer-services")
      ?.children;

    expect(serviceMenus).toMatchObject([
      {
        id: "customer-service-workspace-models",
        label: "模型定价",
        to: "/customer/services/models",
        icon: "layers",
        active: false
      },
      {
        id: "customer-service-workspace-subscription",
        label: "订阅套餐",
        to: "/customer/services/subscription",
        icon: "calendar-clock",
        active: true
      },
      {
        id: "customer-usage",
        label: "使用记录",
        to: "/customer/usage",
        icon: "scroll-text",
        active: false
      }
    ]);
  });

  it("keeps tenant and customer developer centers focused on API access", () => {
    for (const moduleId of ["tenant-developer", "customer-developer"]) {
      const developerModule = portalModules.find((module) => module.id === moduleId);
      expect(developerModule?.tabs?.map(({ id, label, path }) => ({ id, label, path }))).toEqual([
        { id: "keys", label: "API 密钥", path: "keys" },
        { id: "tooling", label: "工具接入指南", path: "tooling" }
      ]);
    }
  });

  it("exposes tenants and end users as separate admin menus", () => {
    const organizationMenus = buildPortalNav(1, "/admin/organization/users")
      .find((item) => item.id === "admin-organization")
      ?.children;

    expect(organizationMenus).toMatchObject([
      {
        id: "admin-organization-workspace-tenants",
        label: "租户管理",
        to: "/admin/organization/tenants",
        icon: "building-2",
        active: false
      },
      {
        id: "admin-organization-workspace-users",
        label: "终端用户",
        to: "/admin/organization/users",
        icon: "users",
        active: true
      },
      {
        id: "admin-identity-workspace",
        label: "管理员与身份"
      }
    ]);

    const tenantDetailMenus = buildPortalNav(1, "/admin/organization/tenants/tenant-1")
      .find((item) => item.id === "admin-organization")
      ?.children;
    expect(tenantDetailMenus?.find((item) => item.id === "admin-organization-workspace-tenants")?.active).toBe(true);
  });

  it("does not expose tenant policy as a standalone admin menu", () => {
    expect(leavesFor(1).some((item) => item.label === "租户策略")).toBe(false);
    expect(portalModules.some((module) => module.path === "/admin/ai/tenant-policy")).toBe(false);
  });

  it("keeps recharge operations in settlement and removes the legacy billing-event tab", () => {
    const settlement = portalModules.find((module) => module.id === "admin-settlement-workspace");

    expect(settlement?.tabs?.map((tab) => tab.id)).toContain("recharges");
    expect(settlement?.tabs?.some((tab) => tab.id === "transactions")).toBe(false);
    expect(settlement?.tabs?.map((tab) => tab.id)).toEqual(["recharges", "withdrawals"]);
    expect(portalModules.find((module) => module.id === "admin-payment-settings")).toMatchObject({
      label: "支付设置",
      path: "/admin/settings/payment"
    });
  });

  it("keeps upstream, pool and pricing pages as separate admin menus", () => {
    const aiGatewayMenus = buildPortalNav(1, "/admin/ai/upstreams/accounts")
      .find((item) => item.id === "admin-ai")?.children;
    expect(aiGatewayMenus).toMatchObject([
      {
        id: "admin-upstream-accounts",
        label: "上游账号",
        to: "/admin/ai/upstreams/accounts",
        icon: "database",
        active: true
      },
      {
        id: "admin-credential-pools",
        label: "账号池",
        to: "/admin/ai/upstreams/pools",
        icon: "boxes",
        active: false
      },
      {
        id: "admin-price-books",
        label: "价格表",
        to: "/admin/ai/upstreams/pricing",
        icon: "tags",
        active: false
      },
      { id: "admin-usage", label: "使用记录", active: false },
      { id: "admin-usage-analytics", label: "用量分析", active: false },
      { id: "admin-security-workspace", label: "审计与风控", active: false }
    ]);
  });

  it("keeps upstream, pool and pricing pages active independently", () => {
    for (const [path, id] of [
      ["/admin/ai/upstreams/accounts", "admin-upstream-accounts"],
      ["/admin/ai/upstreams/pools", "admin-credential-pools"],
      ["/admin/ai/upstreams/pricing", "admin-price-books"]
    ] as const) {
      const active = leavesFor(1, path).filter((item) => item.active).map((item) => item.id);
      expect(active).toEqual([id]);
    }
  });

  it("exposes usage records separately from security controls", () => {
    const usageMenu = leavesFor(1, "/admin/ai/usage/request-1").find((item) => item.id === "admin-usage");
    const analyticsMenu = leavesFor(1, "/admin/ai/analytics").find((item) => item.id === "admin-usage-analytics");
    const security = portalModules.find((module) => module.id === "admin-security-workspace");

    expect(usageMenu).toMatchObject({
      label: "使用记录",
      to: "/admin/ai/usage",
      icon: "scroll-text",
      active: true
    });
    expect(analyticsMenu).toMatchObject({
      label: "用量分析",
      to: "/admin/ai/analytics",
      icon: "bar-chart-3",
      active: true
    });
    expect(security?.tabs?.map((tab) => tab.id)).toEqual(["audit", "risk"]);
  });

  it("uses capabilities as the role access source", () => {
    expect(userHasPortalCapability(1, "admin.identity")).toBe(true);
    expect(userHasPortalCapability(2, "admin.identity")).toBe(false);
    expect(userHasPortalCapability(3, "tenant.developer")).toBe(true);
    expect(userHasPortalCapability(4, "tenant.developer")).toBe(false);
    expect(userHasPortalCapability(4, "customer.developer")).toBe(true);
    expect(userHasPortalCapability(1, "unknown.capability")).toBe(false);
  });

  it("keeps module ids and paths unique", () => {
    expect(new Set(portalModules.map((module) => module.id)).size).toBe(portalModules.length);
    expect(new Set(portalModules.map((module) => module.path)).size).toBe(portalModules.length);
  });

  it("provides role-specific home and profile destinations", () => {
    expect(defaultPortalPathForUserType(1)).toBe("/admin/dashboard");
    expect(defaultPortalPathForUserType(3)).toBe("/tenant/overview/business");
    expect(defaultPortalPathForUserType(4)).toBe("/customer/workbench");
    expect(profilePathForUserType(2)).toBe("/admin/profile");
    expect(profilePathForUserType(3)).toBe("/tenant/profile");
    expect(profilePathForUserType(4)).toBe("/customer/profile");
  });
});
