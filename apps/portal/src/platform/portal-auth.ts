import { createPortalAuthApi, createPortalAuthStore } from "./auth";

import type { PortalEnv } from "./env";
import { createFetchAdapter } from "./http";

export interface CreateStandardPortalAuthStoreOptions {
  env: PortalEnv;
  storeId: string;
  expectedUserTypes: number[];
}

export function createStandardPortalAuthStore(options: CreateStandardPortalAuthStoreOptions) {
  let useAuthStore!: ReturnType<typeof createPortalAuthStore>;

  // userinfo/revoke 走 userAuth 中间件，需 Bearer；token 端点处理登录和刷新。
	// 登出时撤销当前 session family，并清空本地认证状态。
  // raw fetch 不受影响。这里惰性读取 store，避免与下方 useAuthStore 定义形成初始化环。
  const request = createFetchAdapter({
    getAccessToken: () => useAuthStore().accessToken,
    async onUnauthorized() {
      try {
        await useAuthStore().refreshAccessToken();
        return "retry";
      } catch {
        return false;
      }
    }
  });
  const authApi = createPortalAuthApi({
    request,
    baseUrl: options.env.apiBaseUrl
  });
  useAuthStore = createPortalAuthStore({
    storeId: options.storeId,
    storagePrefix: options.env.storagePrefix,
    expectedUserTypes: options.expectedUserTypes,
    login: (username, password) => authApi.login(username, password),
    refreshToken: (refreshToken) => authApi.refreshToken(refreshToken),
    logout: () => authApi.logout(),
    getCurrentUser: () => authApi.getCurrentUser()
  });

  return useAuthStore;
}
