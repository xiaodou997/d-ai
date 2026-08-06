import { computed, ref } from "vue";
import { defineStore } from "pinia";

import type { UserInfoResponse } from "./api";

export interface AuthStoreOptions {
  storeId: string;
  storagePrefix: string;
  expectedUserTypes: number[];
  login?: (username: string, password: string) => Promise<{
    accessToken: string;
    refreshToken?: string;
    expiresIn: number;
  }>;
  refreshToken: (refreshToken: string) => Promise<{
    accessToken: string;
    refreshToken?: string;
    expiresIn: number;
  }>;
  logout: () => Promise<unknown>;
  logoutRedirectUrl?: string | (() => string | null);
  getCurrentUser: () => Promise<UserInfoResponse>;
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
    const sessionValidatedAt = ref(0);
    let refreshTimer: ReturnType<typeof setTimeout> | null = null;
    let refreshInFlight: Promise<{
      accessToken: string;
      refreshToken?: string;
      expiresIn: number;
    }> | null = null;
    let sessionValidationInFlight: Promise<UserInfoResponse> | null = null;

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
    }

    function clear() {
      accessToken.value = "";
      refreshTokenValue.value = "";
      expiresIn.value = 0;
      userInfo.value = null;
      sessionValidatedAt.value = 0;
      refreshInFlight = null;
      sessionValidationInFlight = null;
      localStorage.removeItem(`${options.storagePrefix}:accessToken`);
      localStorage.removeItem(`${options.storagePrefix}:refreshToken`);
      localStorage.removeItem(`${options.storagePrefix}:expiresIn`);
      localStorage.removeItem(`${options.storagePrefix}:userInfo`);
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
        sessionValidatedAt.value = Date.now();
        persist();
        return response;
      } catch (error) {
        sessionValidatedAt.value = 0;
        throw error;
      }
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

    async function login(username: string, password: string) {
      if (!options.login) {
        throw new Error("password login is not configured");
      }
      const token = await options.login(username, password);
      accessToken.value = token.accessToken;
      refreshTokenValue.value = token.refreshToken || "";
      expiresIn.value = token.expiresIn;
      persist();
      try {
        await fetchUserInfo();
      } catch (error) {
        clear();
        throw error;
      }
      startAutoRefresh();
      return token;
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
          accessToken.value = token.accessToken;
          if (token.refreshToken) {
            refreshTokenValue.value = token.refreshToken;
          }
          expiresIn.value = token.expiresIn;
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

    return {
      accessToken,
      refreshToken: refreshTokenValue,
      expiresIn,
      userInfo,
      username,
      userType,
      tenantName,
      isAuthenticated,
      init,
      clear,
      login,
      logout,
      fetchUserInfo,
      ensureSession,
      refreshAccessToken,
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

function readUserInfo(storagePrefix: string): UserInfoResponse | null {
  const raw = localStorage.getItem(`${storagePrefix}:userInfo`);
  if (!raw) return null;
  try {
    return JSON.parse(raw) as UserInfoResponse;
  } catch {
    return null;
  }
}

function isSessionInvalidError(error: unknown): boolean {
  if (!error || typeof error !== "object") {
    return false;
  }
  const status = Number((error as { status?: unknown }).status);
  return status === 401 || status === 403 || status === 404;
}
