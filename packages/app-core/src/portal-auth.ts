import { createPortalAuthStore, createUrmAuthApi } from "../../auth/src";

import type { PortalEnv } from "./env";
import { createFetchAdapter } from "./http";
import { currentRedirectUri } from "./sso";

export interface CreateStandardPortalAuthStoreOptions {
  env: PortalEnv;
  storeId: string;
  expectedUserTypes: number[];
}

export function createStandardPortalAuthStore(options: CreateStandardPortalAuthStoreOptions) {
  let useAuthStore!: ReturnType<typeof createPortalAuthStore>;

  // userinfo/revoke 走 userAuth 中间件，需 Bearer。token 端点(login/refresh/exchange)走另一条。
  // 登出分两段：先 revoke 当前 access token，再整页跳到 /api/oauth2/logout 清 SSO cookie。
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
  const authApi = createUrmAuthApi({
    request,
    baseUrl: options.env.urmBaseUrl,
    clientType: options.env.clientTypeHeader,
    xClientId: options.env.serviceClientIds?.urm || options.env.xClientId
  });
  const aiAuthApi = createUrmAuthApi({
    request,
    baseUrl: options.env.urmBaseUrl,
    clientType: options.env.clientTypeHeader,
    xClientId: options.env.serviceClientIds?.ai || options.env.xClientId
  });
  const proxyAuthApi = createUrmAuthApi({
    request,
    baseUrl: options.env.urmBaseUrl,
    clientType: options.env.clientTypeHeader,
    xClientId: options.env.serviceClientIds?.proxy || options.env.xClientId
  });

  useAuthStore = createPortalAuthStore({
    storeId: options.storeId,
    storagePrefix: options.env.storagePrefix,
    expectedUserTypes: options.expectedUserTypes,
    refreshToken: authApi.refreshToken,
    exchangeCode: authApi.exchangeCode,
    serviceRefreshTokens: {
      ai: aiAuthApi.refreshToken,
      proxy: proxyAuthApi.refreshToken
    },
    serviceExchangeCodes: {
      ai: aiAuthApi.exchangeCode,
      proxy: proxyAuthApi.exchangeCode
    },
    logout: authApi.logout,
    logoutRedirectUrl: () =>
      authApi.ssoLogoutUrl(
        currentRedirectUri(`${window.location.pathname}${window.location.search}${window.location.hash}`),
        options.env.clientTypeHeader
      ),
    getCurrentUser: authApi.getCurrentUser,
    getCurrentCapabilities: authApi.getCurrentCapabilities,
    serviceClientIDs: options.env.serviceClientIds
  });

  return useAuthStore;
}
