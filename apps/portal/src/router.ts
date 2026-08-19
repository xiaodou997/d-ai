import { createRouter, createWebHistory, type RouteRecordRaw } from "vue-router";
import { attachPortalAuthGuard } from "@/platform/portal-router";

import { portalEnv } from "./env";
import {
  buildPortalModuleRoutes,
  defaultPortalPathForUserType,
  userHasPortalCapability
} from "./modules/portalModules";
import LegalCenterLayout from "./platform/legal/LegalCenterLayout.vue";
import LegalDocumentView from "./platform/legal/LegalDocumentView.vue";
import { useAuthStore } from "./stores/auth";

const LayoutView = () => import("./views/LayoutView.vue");

export const routes: RouteRecordRaw[] = [
  {
    path: "/",
    component: LayoutView,
    children: [
      { path: "", redirect: "/overview" },
      {
        path: "overview",
        component: () => import("./modules/PortalHomeRedirect.vue"),
        meta: { title: "工作台" }
      },
      { path: "admin/overview", redirect: "/admin/overview/business" },
      { path: "admin/overview/platform", redirect: "/admin/overview/business" },
      { path: "admin/overview/ai", redirect: "/admin/ai/analytics" },
      { path: "admin/overview/health", redirect: "/admin/overview/operations?tab=health" },
      { path: "admin/ai/monitoring", redirect: "/admin/overview/operations?tab=health" },
      { path: "admin/ai/monitoring/status", redirect: "/admin/overview/operations?tab=health" },
      { path: "admin/ai/monitoring/analytics", redirect: "/admin/ai/analytics" },
      { path: "tenant/overview/ai", redirect: "/tenant/overview/business" },
      ...buildPortalModuleRoutes()
    ]
  },
  { path: "/login", component: () => import("./views/LoginView.vue"), meta: { public: true, title: "登录" } },
  {
    path: "/register/:code",
    component: () => import("./views/RegisterView.vue"),
    meta: { public: true, title: "邀请注册" }
  },
  {
    path: "/legal",
    component: LegalCenterLayout,
    meta: { public: true, title: "法律中心" },
    children: [
      { path: "", redirect: "/legal/privacy" },
      {
        path: ":document",
        component: LegalDocumentView,
        meta: { public: true, title: "法律文件" }
      }
    ]
  },
  { path: "/:pathMatch(.*)*", redirect: "/overview" }
];

const router = createRouter({
  history: createWebHistory(),
  routes
});

attachPortalAuthGuard(router, {
  env: portalEnv as any,
  defaultPathForUserType: defaultPortalPathForUserType,
  hasCapability: userHasPortalCapability,
  useAuthStore: useAuthStore as any
});

export default router;
