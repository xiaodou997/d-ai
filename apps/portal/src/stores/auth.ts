import { defineStore } from "pinia";
import { ref, computed } from "vue";
import { createPortalAuthStore, type PortalAuthStoreLike } from "@dai/auth";

import { portalEnv } from "../env";

/**
 * 统一 Auth Store —— 支持所有 userType (1-4)
 * 与原三端 auth store 共享同一套 SSO 认证流程
 */
const baseStore = createPortalAuthStore({
  portal: "unified" as any,
  env: portalEnv as any
});

export const useAuthStore = defineStore("unified-auth", () => {
  // 复用 base store 的所有逻辑
  const inner = baseStore();

  const accessToken = ref("");
  const userInfo = ref<Record<string, any>>({});
  const userType = ref<number>(0);
  const enabledClientIds = ref<string[]>([]);

  // 代理 base store 的方法
  const init = () => inner.init();
  const ensureSession = () => inner.ensureSession();
  const exchangeCode = (code: string, redirectUri: string, codeVerifier: string) =>
    inner.exchangeCode(code, redirectUri, codeVerifier);
  const fetchUserInfo = () => inner.fetchUserInfo();
  const clear = () => inner.clear();
  const logout = () => inner.logout();
  const hasClientAccess = (clientID: string) => inner.hasClientAccess(clientID);

  const username = computed(() => (userInfo.value?.username as string) || "");

  return {
    accessToken,
    userInfo,
    userType,
    enabledClientIds,
    username,
    init,
    ensureSession,
    exchangeCode,
    fetchUserInfo,
    clear,
    logout,
    hasClientAccess
  };
});

export type { PortalAuthStoreLike };
