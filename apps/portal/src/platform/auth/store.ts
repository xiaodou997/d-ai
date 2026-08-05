import { computed, ref } from "vue";
import { defineStore } from "pinia";

import type { CurrentCapabilitiesResponse, UserInfoResponse } from "./api";

export interface AuthStoreOptions {
  storeId: string;
  storagePrefix: string;
  expectedUserTypes: number[];
  refreshToken: (refreshToken: string) => Promise<{
    access_token: string;
    refresh_token?: string;
    expires_in: number;
  }>;
  exchangeCode?: (code: string, redirectUri: string, codeVerifier: string) => Promise<{
    access_token: string;
    refresh_token?: string;
    expires_in: number;
  }>;
  serviceRefreshTokens?: Record<
    string,
    (refreshToken: string) => Promise<{
      access_token: string;
      refresh_token?: string;
      expires_in: number;
    }>
  >;
  serviceExchangeCodes?: Record<
    string,
    (code: string, redirectUri: string, codeVerifier: string) => Promise<{
      access_token: string;
      refresh_token?: string;
      expires_in: number;
    }>
  >;
  logout: () => Promise<unknown>;
  logoutRedirectUrl?: string | (() => string | null);
  getCurrentUser: () => Promise<UserInfoResponse>;
  getCurrentCapabilities?: () => Promise<CurrentCapabilitiesResponse>;
  serviceClientIDs?: Record<string, string | undefined>;
}

interface StoredToken {
  accessToken: string;
  refreshToken: string;
  expiresIn: number;
}

export interface EnsureSessionOptions {
  force?: boolean;
}

const SESSION_VALIDATION_TTL_MS = 60 * 1000;

export function createPortalAuthStore(options: AuthStoreOptions) {
  return defineStore(options.storeId, () => {
    const accessToken = ref(localStorage.getItem(`${options.storagePrefix}:accessToken`) || "");
    const refreshTokenValue = ref(localStorage.getItem(`${options.storagePrefix}:refreshToken`) || "");
    const expiresIn = ref(Number(localStorage.getItem(`${options.storagePrefix}:expiresIn`) || "0"));
    const userInfo = ref<UserInfoResponse | null>(readUserInfo(options.storagePrefix));
    const serviceTokens = ref<Record<string, StoredToken>>(readServiceTokens(options.storagePrefix));
    const enabledClientIds = ref<string[]>(readEnabledClientIDs(options.storagePrefix));
    const capabilitiesLoaded = ref(false);
    const sessionValidatedAt = ref(0);
    let refreshTimer: ReturnType<typeof setTimeout> | null = null;
    let refreshInFlight: Promise<{
      access_token: string;
      refresh_token?: string;
      expires_in: number;
    }> | null = null;
    let sessionValidationInFlight: Promise<UserInfoResponse> | null = null;
    const serviceRefreshInFlight: Record<string, Promise<StoredToken> | undefined> = {};

    const isAuthenticated = computed(() => Boolean(accessToken.value && userInfo.value));
    const username = computed(() => userInfo.value?.username || "");
    const userType = computed(() => Number(userInfo.value?.userType || 0));
    const tenantName = computed(() => userInfo.value?.tenantName || "");

    function persist() {
      localStorage.setItem(`${options.storagePrefix}:accessToken`, accessToken.value);
      localStorage.setItem(`${options.storagePrefix}:refreshToken`, refreshTokenValue.value);
      localStorage.setItem(`${options.storagePrefix}:expiresIn`, String(expiresIn.value));
      if (userInfo.value) {
        localStorage.setItem(`${options.storagePrefix}:userInfo`, JSON.stringify(userInfo.value));
      }
      localStorage.setItem(`${options.storagePrefix}:serviceTokens`, JSON.stringify(serviceTokens.value));
      localStorage.setItem(`${options.storagePrefix}:enabledClientIds`, JSON.stringify(enabledClientIds.value));
    }

    function clear() {
      accessToken.value = "";
      refreshTokenValue.value = "";
      expiresIn.value = 0;
      userInfo.value = null;
      serviceTokens.value = {};
      enabledClientIds.value = [];
      capabilitiesLoaded.value = false;
      sessionValidatedAt.value = 0;
      refreshInFlight = null;
      sessionValidationInFlight = null;
      for (const key of Object.keys(serviceRefreshInFlight)) {
        delete serviceRefreshInFlight[key];
      }
      localStorage.removeItem(`${options.storagePrefix}:accessToken`);
      localStorage.removeItem(`${options.storagePrefix}:refreshToken`);
      localStorage.removeItem(`${options.storagePrefix}:expiresIn`);
      localStorage.removeItem(`${options.storagePrefix}:userInfo`);
      localStorage.removeItem(`${options.storagePrefix}:serviceTokens`);
      localStorage.removeItem(`${options.storagePrefix}:enabledClientIds`);
      stopAutoRefresh();
    }

    function startAutoRefresh() {
      stopAutoRefresh();
      if (!expiresIn.value || !refreshTokenValue.value) return;
      const delay = Math.max(1000, (expiresIn.value - 300) * 1000);
      refreshTimer = setTimeout(() => {
        void refreshAccessToken();
      }, delay);
    }

    function stopAutoRefresh() {
      if (refreshTimer) {
        clearTimeout(refreshTimer);
        refreshTimer = null;
      }
    }

    async function fetchUserInfo() {
      try {
        const response = await options.getCurrentUser();
        if (!options.expectedUserTypes.includes(response.userType)) {
          clear();
          throw new Error("当前账号无权访问该平台");
        }
        userInfo.value = response;
        await refreshCapabilities();
        sessionValidatedAt.value = Date.now();
        persist();
        return response;
      } catch (error) {
        sessionValidatedAt.value = 0;
        throw error;
      }
    }

    async function refreshCapabilities() {
      if (!options.getCurrentCapabilities) {
        enabledClientIds.value = [];
        capabilitiesLoaded.value = true;
        persist();
        return enabledClientIds.value;
      }
      try {
        const capabilities = await options.getCurrentCapabilities();
        enabledClientIds.value = normalizeClientIDs(capabilities.enabledClientIds);
        serviceTokens.value = pruneUnavailableServiceTokens(serviceTokens.value, options.serviceClientIDs, enabledClientIds.value);
        capabilitiesLoaded.value = true;
        persist();
        return enabledClientIds.value;
      } catch (error) {
        enabledClientIds.value = [];
        capabilitiesLoaded.value = false;
        persist();
        throw error;
      }
    }

    function clearServiceToken(service: string) {
      if (service === "urm") return;
      const next = { ...serviceTokens.value };
      delete next[service];
      serviceTokens.value = next;
      persist();
    }

    async function ensureSession(sessionOptions: EnsureSessionOptions = {}) {
      if (!accessToken.value) {
        clear();
        throw new Error("access token missing");
      }
      if (!sessionOptions.force && userInfo.value && Date.now() - sessionValidatedAt.value < SESSION_VALIDATION_TTL_MS) {
        return userInfo.value;
      }
      if (sessionValidationInFlight) {
        return sessionValidationInFlight;
      }
      sessionValidationInFlight = fetchUserInfo()
        .catch((error) => {
          if (userInfo.value && !isSessionInvalidError(error)) {
            sessionValidatedAt.value = Date.now();
            return userInfo.value;
          }
          throw error;
        })
        .finally(() => {
          sessionValidationInFlight = null;
        });
      return sessionValidationInFlight;
    }

    async function exchangeCode(code: string, redirectUri: string, codeVerifier: string) {
      if (!options.exchangeCode) {
        throw new Error("authorization_code flow is not configured");
      }
      const token = await options.exchangeCode(code, redirectUri, codeVerifier);
      accessToken.value = token.access_token;
      refreshTokenValue.value = token.refresh_token || "";
      expiresIn.value = token.expires_in;
      persist();
      await fetchUserInfo();
      startAutoRefresh();
      return token;
    }

    async function exchangeServiceCode(
      service: string,
      code: string,
      redirectUri: string,
      codeVerifier: string
    ) {
      const exchange = options.serviceExchangeCodes?.[service];
      if (!exchange) {
        throw new Error(`authorization_code flow is not configured for ${service}`);
      }
      const token = await exchange(code, redirectUri, codeVerifier);
      serviceTokens.value = {
        ...serviceTokens.value,
        [service]: {
          accessToken: token.access_token,
          refreshToken: token.refresh_token || "",
          expiresIn: token.expires_in
        }
      };
      persist();
      return token;
    }

    function accessTokenForService(service: string) {
      if (service === "urm") {
        return accessToken.value;
      }
      return serviceTokens.value[service]?.accessToken || accessToken.value;
    }

    async function refreshServiceAccessToken(service: string) {
      if (service === "urm") {
        await refreshAccessToken();
        return {
          accessToken: accessToken.value,
          refreshToken: refreshTokenValue.value,
          expiresIn: expiresIn.value
        };
      }
      const stored = serviceTokens.value[service];
      const refreshService = options.serviceRefreshTokens?.[service];
      if (!stored?.refreshToken || !refreshService) {
        await refreshAccessToken();
        return {
          accessToken: accessToken.value,
          refreshToken: refreshTokenValue.value,
          expiresIn: expiresIn.value
        };
      }
      if (serviceRefreshInFlight[service]) {
        return serviceRefreshInFlight[service];
      }
      serviceRefreshInFlight[service] = refreshService(stored.refreshToken)
        .then((token) => {
          const nextToken = {
            accessToken: token.access_token,
            refreshToken: token.refresh_token || stored.refreshToken,
            expiresIn: token.expires_in
          };
          serviceTokens.value = {
            ...serviceTokens.value,
            [service]: nextToken
          };
          persist();
          return nextToken;
        })
        .catch((error) => {
          const next = { ...serviceTokens.value };
          delete next[service];
          serviceTokens.value = next;
          persist();
          throw error;
        })
        .finally(() => {
          delete serviceRefreshInFlight[service];
        });
      return serviceRefreshInFlight[service];
    }

    async function refreshAccessToken() {
      if (refreshInFlight) {
        return refreshInFlight;
      }
      if (!refreshTokenValue.value) {
        clear();
        throw new Error("refresh token missing");
      }
      refreshInFlight = options
        .refreshToken(refreshTokenValue.value)
        .then((token) => {
          accessToken.value = token.access_token;
          if (token.refresh_token) {
            refreshTokenValue.value = token.refresh_token;
          }
          expiresIn.value = token.expires_in;
          persist();
          startAutoRefresh();
          return token;
        })
        .catch((error) => {
          clear();
          throw error;
        })
        .finally(() => {
          refreshInFlight = null;
        });
      return refreshInFlight;
    }

    async function logout() {
      const redirectUrl = resolveLogoutRedirectUrl(options.logoutRedirectUrl);
      try {
        if (accessToken.value) {
          await options.logout();
        }
      } finally {
        clear();
        if (redirectUrl && typeof window !== "undefined") {
          window.location.assign(redirectUrl);
          return true;
        }
      }
      return false;
    }

    function init() {
      if (accessToken.value && refreshTokenValue.value) {
        startAutoRefresh();
      }
    }

    function hasClientAccess(clientID: string) {
      return capabilitiesLoaded.value && enabledClientIds.value.includes(clientID);
    }

    return {
      accessToken,
      refreshToken: refreshTokenValue,
      expiresIn,
      serviceTokens,
      enabledClientIds,
      capabilitiesLoaded,
      userInfo,
      username,
      userType,
      tenantName,
      isAuthenticated,
      init,
      hasClientAccess,
      clear,
      exchangeCode,
      exchangeServiceCode,
      logout,
      fetchUserInfo,
      refreshCapabilities,
      clearServiceToken,
      ensureSession,
      accessTokenForService,
      refreshAccessToken,
      refreshServiceAccessToken,
      startAutoRefresh,
      stopAutoRefresh
    };
  });
}

function resolveLogoutRedirectUrl(
  input?: string | (() => string | null)
): string | null {
  if (!input) return null;
  if (typeof input === "function") {
    return input();
  }
  return input;
}

function readServiceTokens(storagePrefix: string): Record<string, StoredToken> {
  const raw = localStorage.getItem(`${storagePrefix}:serviceTokens`);
  if (!raw) return {};
  try {
    return JSON.parse(raw) as Record<string, StoredToken>;
  } catch {
    return {};
  }
}

function readUserInfo(storagePrefix: string): UserInfoResponse | null {
  const raw = localStorage.getItem(`${storagePrefix}:userInfo`);
  if (!raw) return null;
  try {
    return JSON.parse(raw) as UserInfoResponse;
  } catch {
    return null;
  }
}

function readEnabledClientIDs(storagePrefix: string): string[] {
  const raw = localStorage.getItem(`${storagePrefix}:enabledClientIds`);
  if (!raw) return [];
  try {
    return normalizeClientIDs(JSON.parse(raw));
  } catch {
    return [];
  }
}

function normalizeClientIDs(value: unknown): string[] {
  if (!Array.isArray(value)) return [];
  return [...new Set(value.filter((item): item is string => typeof item === "string" && item.length > 0))];
}

function pruneUnavailableServiceTokens(
  tokens: Record<string, StoredToken>,
  serviceClientIDs: Record<string, string | undefined> | undefined,
  enabledClientIDs: readonly string[]
): Record<string, StoredToken> {
  if (!serviceClientIDs) return tokens;
  return Object.fromEntries(
    Object.entries(tokens).filter(([service]) => {
      const clientID = serviceClientIDs[service];
      return Boolean(clientID && enabledClientIDs.includes(clientID));
    })
  );
}

function isSessionInvalidError(error: unknown): boolean {
  if (!error || typeof error !== "object") {
    return false;
  }
  const status = Number((error as { status?: unknown }).status);
  return status === 401 || status === 403 || status === 404;
}
