import { createPortalAuthStore, createUrmAuthApi } from "./auth";

import type { BackendService, PortalEnv } from "./env";
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
  // The unified login page can select admin/tenant/customer before SSO. Build
  // the small auth facade per call so a new selection is used without
  // recreating the Pinia store.
  const authApiFor = (service: BackendService) => createUrmAuthApi({
    request,
    baseUrl: options.env.urmBaseUrl,
    clientType: options.env.clientTypeHeader,
    xClientId: options.env.serviceClientIds?.[service] || options.env.xClientId
  });
  useAuthStore = createPortalAuthStore({
    storeId: options.storeId,
    storagePrefix: options.env.storagePrefix,
    expectedUserTypes: options.expectedUserTypes,
    refreshToken: (refreshToken) => authApiFor("urm").refreshToken(refreshToken),
    exchangeCode: (code, redirectUri, codeVerifier) => authApiFor("urm").exchangeCode(code, redirectUri, codeVerifier),
    serviceRefreshTokens: {
      ai: (refreshToken) => authApiFor("ai").refreshToken(refreshToken),
    },
    serviceExchangeCodes: {
      ai: (code, redirectUri, codeVerifier) => authApiFor("ai").exchangeCode(code, redirectUri, codeVerifier),
    },
    logout: () => authApiFor("urm").logout(),
    logoutRedirectUrl: () =>
      authApiFor("urm").ssoLogoutUrl(
        currentRedirectUri(`${window.location.pathname}${window.location.search}${window.location.hash}`),
        options.env.clientTypeHeader
      ),
    getCurrentUser: () => authApiFor("urm").getCurrentUser(),
    getCurrentCapabilities: () => authApiFor("urm").getCurrentCapabilities(),
    serviceClientIDs: options.env.serviceClientIds
  });

  return useAuthStore;
}
