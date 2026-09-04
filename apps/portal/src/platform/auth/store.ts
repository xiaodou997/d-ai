import { computed, ref } from "vue";
import { defineStore } from "pinia";

import { MFARequiredError } from "./api";
import type { AuthTokenResponse, UserInfoResponse } from "./api";

export interface AuthStoreOptions {
  storeId: string;
  storagePrefix: string;
  expectedUserTypes: number[];
  login?: (username: string, password: string) => Promise<AuthTokenResponse>;
  refreshToken: () => Promise<AuthTokenResponse>;
  recentAuth?: (password: string, code?: string) => Promise<unknown>;
  verifyMFA?: (challengeToken: string, code: string) => Promise<AuthTokenResponse>;
  logout: () => Promise<unknown>;
  logoutRedirectUrl?: string | (() => string | null);
  getCurrentUser: () => Promise<UserInfoResponse>;
}

export interface EnsureSessionOptions {
  force?: boolean;
}

export interface TenantOperationsToken {
  accessToken: string;
  expiresIn: number;
  tenantId: string;
  tenantName: string;
}

interface RestorableAuthState {
  accessToken: string;
  expiresIn: number;
  userInfo: UserInfoResponse | null;
  sessionValidatedAt: number;
}

const SESSION_VALIDATION_TTL_MS = 60 * 1000;

export function createPortalAuthStore(options: AuthStoreOptions) {
  return defineStore(options.storeId, () => {
    // Access and refresh tokens deliberately never enter Web Storage. The
    // refresh token is HttpOnly; the access token only lives in this tab's heap.
    const accessToken = ref("");
    const expiresIn = ref(0);
    const userInfo = ref<UserInfoResponse | null>(readUserInfo(options.storagePrefix));
    const mfaChallengeToken = ref("");
    const sessionValidatedAt = ref(0);
    const tenantOperations = ref<{ tenantId: string; tenantName: string } | null>(null);
    let refreshTimer: ReturnType<typeof setTimeout> | null = null;
    let tenantOperationsTimer: ReturnType<typeof setTimeout> | null = null;
    let refreshInFlight: Promise<AuthTokenResponse> | null = null;
    let sessionValidationInFlight: Promise<UserInfoResponse> | null = null;
    let storageListenerInstalled = false;
    let operatorState: RestorableAuthState | null = null;

    const isAuthenticated = computed(() => Boolean(accessToken.value && userInfo.value));
    const username = computed(() => userInfo.value?.username || "");
    const userType = computed(() => Number(userInfo.value?.userType || 0));
    const tenantName = computed(() => userInfo.value?.tenantName || "");
    const isTenantOperations = computed(() => tenantOperations.value !== null);

    function persist() {
      // Tenant operations is deliberately tab-local. Publishing its effective
      // user info would make other tabs show tenant menus while they still hold
      // the platform administrator access token.
      if (isTenantOperations.value) return;
      if (userInfo.value) {
        localStorage.setItem(`${options.storagePrefix}:userInfo`, JSON.stringify(userInfo.value));
      }
    }

    function clearState() {
      accessToken.value = "";
      expiresIn.value = 0;
      userInfo.value = null;
      mfaChallengeToken.value = "";
      sessionValidatedAt.value = 0;
      tenantOperations.value = null;
      operatorState = null;
      refreshInFlight = null;
      sessionValidationInFlight = null;
      stopAutoRefresh();
      stopTenantOperationsTimer();
    }

    function clear() {
      clearState();
      localStorage.removeItem(`${options.storagePrefix}:userInfo`);
    }

    function startAutoRefresh() {
      stopAutoRefresh();
      if (isTenantOperations.value) return;
      if (!expiresIn.value || !accessToken.value) return;
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

    function stopTenantOperationsTimer() {
      if (tenantOperationsTimer) {
        clearTimeout(tenantOperationsTimer);
        tenantOperationsTimer = null;
      }
    }

    function restoreOperatorState() {
      if (!operatorState) return false;
      accessToken.value = operatorState.accessToken;
      expiresIn.value = operatorState.expiresIn;
      userInfo.value = operatorState.userInfo;
      sessionValidatedAt.value = operatorState.sessionValidatedAt;
      tenantOperations.value = null;
      operatorState = null;
      stopTenantOperationsTimer();
      persist();
      startAutoRefresh();
      return true;
    }

    async function enterTenantOperations(token: TenantOperationsToken) {
      if (isTenantOperations.value) {
        throw new Error("请先退出当前租户代运维");
      }
      operatorState = {
        accessToken: accessToken.value,
        expiresIn: expiresIn.value,
        userInfo: userInfo.value,
        sessionValidatedAt: sessionValidatedAt.value
      };
      stopAutoRefresh();
      accessToken.value = token.accessToken;
      expiresIn.value = token.expiresIn;
      tenantOperations.value = { tenantId: token.tenantId, tenantName: token.tenantName };
      userInfo.value = null;
      sessionValidatedAt.value = 0;
      try {
        await fetchUserInfo();
      } catch (error) {
        restoreOperatorState();
        throw error;
      }

      stopTenantOperationsTimer();
      tenantOperationsTimer = setTimeout(() => {
        restoreOperatorState();
        if (typeof window !== "undefined") {
          window.location.assign("/admin/organization/tenants");
        }
      }, Math.max(1000, token.expiresIn * 1000));
    }

    function exitTenantOperations() {
      return restoreOperatorState();
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
        await refreshAccessToken();
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
      clearState();
      const token = await options.login(username, password);
      if (token.mfaRequired) {
        if (!token.mfaChallengeToken) throw new Error("MFA challenge missing");
        mfaChallengeToken.value = token.mfaChallengeToken;
        throw new MFARequiredError(token.mfaChallengeToken);
      }
      accessToken.value = token.accessToken;
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

    async function verifyMFA(code: string) {
      if (!options.verifyMFA || !mfaChallengeToken.value) throw new Error("MFA challenge missing");
      const token = await options.verifyMFA(mfaChallengeToken.value, code);
      accessToken.value = token.accessToken;
      expiresIn.value = token.expiresIn;
      mfaChallengeToken.value = "";
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
      refreshInFlight = withRefreshLock(`${options.storagePrefix}:refresh-lock`, async () => options.refreshToken())
        .then((token) => {
          accessToken.value = token.accessToken;
          expiresIn.value = token.expiresIn;
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

    async function reauthenticate(password: string, code = "") {
      if (!options.recentAuth) {
        throw new Error("近期认证服务未配置");
      }
      return options.recentAuth(password, code);
    }

    async function logout() {
      if (isTenantOperations.value) {
        restoreOperatorState();
        return false;
      }
      const redirectUrl = resolveLogoutRedirectUrl(options.logoutRedirectUrl);
      try {
        if (!accessToken.value) {
          try {
            await refreshAccessToken();
          } catch {
            // There may simply be no active HttpOnly session to revoke.
          }
        }
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
      if (!storageListenerInstalled && typeof window !== "undefined") {
        storageListenerInstalled = true;
        const userInfoKey = `${options.storagePrefix}:userInfo`;
        window.addEventListener("storage", (event) => {
          if (event.storageArea && event.storageArea !== window.localStorage) return;
          if (event.key !== userInfoKey) return;
          if (event.newValue === null) {
            clearState();
            return;
          }
          if (isTenantOperations.value) return;
          let next: UserInfoResponse;
          try {
            next = JSON.parse(event.newValue) as UserInfoResponse;
          } catch {
            return;
          }
          const sameUser = userInfo.value?.sub === next.sub;
          userInfo.value = next;
          if (sameUser && accessToken.value) return;
          clearState();
          userInfo.value = next;
          void ensureSession().catch(() => clearState());
        });
      }
      if (accessToken.value) {
        startAutoRefresh();
      }
    }

    return {
      accessToken,
      expiresIn,
      userInfo,
      username,
      userType,
      tenantName,
      tenantOperations,
      isTenantOperations,
      isAuthenticated,
      init,
      clear,
      login,
      verifyMFA,
      reauthenticate,
      enterTenantOperations,
      exitTenantOperations,
      logout,
      fetchUserInfo,
      ensureSession,
      refreshAccessToken,
      startAutoRefresh,
      stopAutoRefresh
    };
  });
}

interface NavigatorWithLocks {
  locks?: {
    request<T>(name: string, options: { mode: "exclusive" }, callback: () => Promise<T>): Promise<T>;
  };
}

async function withRefreshLock<T>(key: string, work: () => Promise<T>): Promise<T> {
  if (typeof navigator !== "undefined" && (navigator as NavigatorWithLocks).locks) {
    return (navigator as NavigatorWithLocks).locks!.request(key, { mode: "exclusive" }, work);
  }

  const owner = `${Date.now()}-${Math.random().toString(36).slice(2)}`;
  const deadline = Date.now() + 30_000;
  const lockValue = JSON.stringify({ owner, expiresAt: deadline });
  while (Date.now() < deadline) {
    let current: { owner?: string; expiresAt?: number } | null = null;
    try {
      const raw = localStorage.getItem(key);
      current = raw ? (JSON.parse(raw) as { owner?: string; expiresAt?: number }) : null;
      if (!current || Number(current.expiresAt) <= Date.now()) {
        localStorage.setItem(key, lockValue);
        if (localStorage.getItem(key) === lockValue) {
          try {
            return await work();
          } finally {
            if (localStorage.getItem(key) === lockValue) localStorage.removeItem(key);
          }
        }
      }
    } catch {
      // If storage is unavailable, do not silently fall back to concurrent
      // rotation: the server treats refresh-token replay as a family breach.
      throw new Error("跨标签页刷新锁不可用");
    }
    await new Promise((resolve) => setTimeout(resolve, 50));
  }
  throw new Error("跨标签页刷新等待超时");
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
