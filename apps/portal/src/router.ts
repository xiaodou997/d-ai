import { createRouter, createWebHistory, type RouteRecordRaw } from "vue-router";
import { attachPortalAuthGuard } from "@/platform/portal-router";

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
