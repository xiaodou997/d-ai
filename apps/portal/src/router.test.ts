import { describe, expect, it } from "vitest";
import type { RouteRecordRaw } from "vue-router";

import { buildPortalNav, defaultPortalPathForUserType, portalModules } from "./modules/portalModules";
import { routes } from "./router";

interface FlatRoute {
  path: string;
  name?: string;
  allowedUserTypes?: number[];
}

function joinRoutePath(parent: string, child: string): string {
  if (child.startsWith("/")) return child;
  const prefix = parent === "/" ? "" : parent.replace(/\/$/, "");
  return `${prefix}/${child}`.replace(/\/+/g, "/");
}

function flattenRoutes(records: readonly RouteRecordRaw[], parent = ""): FlatRoute[] {
  return records.flatMap((record) => {
    const path = joinRoutePath(parent, record.path);
    const current: FlatRoute = {
      path,
      name: typeof record.name === "string" ? record.name : undefined,
      allowedUserTypes: Array.isArray(record.meta?.allowedUserTypes)
        ? (record.meta.allowedUserTypes as number[])
        : undefined
    };
    return [current, ...flattenRoutes(record.children ?? [], path)];
  });
}

function menuPaths(userType: number): string[] {
  return buildPortalNav(userType, defaultPortalPathForUserType(userType)).flatMap((node) =>
    node.to ? [node.to] : (node.children ?? []).flatMap((item) => (item.to ? [item.to] : []))
  );
}

function routeMatchesMenuPath(routePath: string, menuPath: string): boolean {
  return routePath === menuPath || routePath.replace(/\/:[^/]+\?$/, "") === menuPath;
}

describe("unified portal route contract", () => {
  const flatRoutes = flattenRoutes(routes);

  it("backs every visible menu with a route allowed for that user type", () => {
    for (const userType of [1, 2, 3, 4]) {
      for (const path of menuPaths(userType)) {
        const route = flatRoutes.find((item) => routeMatchesMenuPath(item.path, path));
        expect(route, `userType ${userType} menu ${path}`).toBeDefined();
        expect(
          route?.allowedUserTypes?.includes(userType) ?? true,
          `userType ${userType} must be allowed to open ${path}`
        ).toBe(true);
      }
    }
  });

  it("keeps named detail routes inside their consolidated workspaces", () => {
    expect(flatRoutes.find((route) => route.name === "platform-tenant-policy")?.path).toBe("/admin/organization/tenants/:id/policy");
    expect(flatRoutes.find((route) => route.name === "ai-group-detail")?.path).toBe("/tenant/ai/models/groups/:groupId");
    expect(flatRoutes.find((route) => route.name === "ai-usage-detail")?.path).toBe("/admin/ai/usage/:requestId");
    expect(flatRoutes.find((route) => route.name === "ai-user-management")?.path).toBe("/tenant/users/policy/:userId?");
  });

  it("generates one route tree from every registered module", () => {
    const routePaths = flatRoutes.map((route) => route.path);
    for (const module of portalModules) {
      expect(routePaths, module.id).toContain(module.path);
    }
    expect(routePaths).toContain("/tenant/developer/docs/:section?");
    expect(routePaths).toContain("/customer/developer/docs/:section?");
  });
});
