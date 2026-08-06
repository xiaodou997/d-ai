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
    expect(leavesFor(1)).toHaveLength(11);
    expect(leavesFor(2)).toHaveLength(10);
    expect(leavesFor(3)).toHaveLength(13);
    expect(leavesFor(4)).toHaveLength(8);
  });

  it("builds task-oriented workspaces instead of legacy page entries", () => {
    expect(leavesFor(1).map((item) => item.label)).toEqual([
      "经营",
      "AI 运营",
      "运行监控",
      "租户与用户",
      "管理员与身份",
      "账户与交易",
      "结算与支付",
      "上游与定价",
      "租户策略",
      "审计与风控",
      "公告管理"
    ]);
    expect(leavesFor(4).map((item) => item.label)).toEqual([
      "工作台",
      "AI 对话",
      "AI 生图",
      "任务记录",
      "模型与套餐",
      "使用记录",
      "开发中心",
      "账户中心"
    ]);
  });

  it("exposes overview as a category with business, AI operation, and admin monitoring menus", () => {
    const adminOverview = buildPortalNav(1, "/admin/overview/ai")[0];
    const tenantOverview = buildPortalNav(3, "/tenant/overview/business")[0];

    expect(adminOverview).toMatchObject({
      label: "概览",
      active: true,
      children: [
        { label: "经营", to: "/admin/overview/platform", active: false },
        { label: "AI 运营", to: "/admin/overview/ai", active: true },
        { label: "运行监控", to: "/admin/ai/monitoring", active: false }
      ]
    });
    expect(tenantOverview).toMatchObject({
      label: "概览",
      active: true,
      children: [
        { label: "经营", to: "/tenant/overview/business", active: true },
        { label: "AI 运营", to: "/tenant/overview/ai", active: false }
      ]
    });

    const monitoringOverview = buildPortalNav(1, "/admin/ai/monitoring/status")[0];
    expect(monitoringOverview).toMatchObject({
      label: "概览",
      active: true,
      children: [
        { label: "经营", active: false },
        { label: "AI 运营", active: false },
        { label: "运行监控", active: true }
      ]
    });
  });

  it("marks a workspace active for any child or detail route", () => {
    const activeTenant = leavesFor(3, "/tenant/users/directory/user-1").find((item) => item.active);
    const activeAdmin = leavesFor(1, "/admin/ai/security/usage/request-1").find((item) => item.active);

    expect(activeTenant?.id).toBe("tenant-user-workspace");
    expect(activeAdmin?.id).toBe("admin-security-workspace");
    expect(leavesFor(4, "/customer/workbench/chat").filter((item) => item.active).map((item) => item.id)).toEqual([
      "customer-chat"
    ]);
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
    expect(defaultPortalPathForUserType(1)).toBe("/admin/overview/platform");
    expect(defaultPortalPathForUserType(3)).toBe("/tenant/overview/business");
    expect(defaultPortalPathForUserType(4)).toBe("/customer/workbench");
    expect(profilePathForUserType(2)).toBe("/admin/profile");
    expect(profilePathForUserType(3)).toBe("/tenant/profile");
    expect(profilePathForUserType(4)).toBe("/customer/profile");
  });
});
