import { createRouter, createWebHistory, type RouteRecordRaw } from "vue-router";
import { attachPortalAuthGuard } from "@/platform/portal-router";
import type { PortalRouterLike } from "@/platform/portal-router";

import { portalEnv } from "./env";
import {
  buildPortalModuleRoutes,
  defaultPortalPathForUserType,
  userHasPortalCapability
} from "./modules/portalModules";
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
    path: "/activate",
    component: () => import("./views/ActivateAccountView.vue"),
    meta: { public: true, title: "激活账号" }
  },
  {
    path: "/legal",
    component: () => import("./platform/legal/LegalCenterLayout.vue"),
    meta: { public: true, title: "法律中心" },
    children: [
      { path: "", redirect: "/legal/privacy" },
      {
        path: ":document",
        component: () => import("./platform/legal/LegalDocumentView.vue"),
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

attachPortalAuthGuard(router as unknown as PortalRouterLike, {
  env: portalEnv,
  defaultPathForUserType: defaultPortalPathForUserType,
  hasCapability: userHasPortalCapability,
  useAuthStore
});

export default router;
