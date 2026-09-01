import { createPinia, setActivePinia } from "pinia";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { createPortalAuthStore } from "./store";

const user = {
  sub: "user-1",
  username: "alice",
  userType: 4,
  tenantId: "tenant-1",
  tenantName: "Tenant"
};

function makeOptions(prefix: string, overrides: Partial<Parameters<typeof createPortalAuthStore>[0]> = {}) {
  return {
    storeId: `auth-${prefix}`,
    storagePrefix: `test-auth-${prefix}`,
    expectedUserTypes: [4],
    login: vi.fn().mockResolvedValue({ accessToken: "access-1", expiresIn: 3600, refreshExpiresIn: 604800 }),
    refreshToken: vi.fn().mockResolvedValue({ accessToken: "access-refresh", expiresIn: 3600, refreshExpiresIn: 604700 }),
    recentAuth: vi.fn().mockResolvedValue({ message: "重新认证成功" }),
    logout: vi.fn().mockResolvedValue({ success: true }),
    getCurrentUser: vi.fn().mockResolvedValue(user),
    ...overrides
  };
}

beforeEach(() => {
  setActivePinia(createPinia());
  localStorage.clear();
});

afterEach(() => {
  localStorage.clear();
});

describe("Portal auth memory and cookie session", () => {
  it("keeps access and refresh tokens out of Web Storage", async () => {
    const options = makeOptions("login");
    const store = createPortalAuthStore(options)();

    await store.login("alice", "password");

    expect(store.accessToken).toBe("access-1");
    expect(localStorage.getItem(`${options.storagePrefix}:accessToken`)).toBeNull();
    expect(localStorage.getItem(`${options.storagePrefix}:refreshToken`)).toBeNull();
    expect(localStorage.getItem(`${options.storagePrefix}:expiresIn`)).toBeNull();
    expect("refreshToken" in store).toBe(false);
    store.stopAutoRefresh();
  });

  it("rebuilds in-memory access state from the HttpOnly-cookie refresh path", async () => {
    const options = makeOptions("reload");
    localStorage.setItem(`${options.storagePrefix}:userInfo`, JSON.stringify(user));
    const store = createPortalAuthStore(options)();

    await store.ensureSession();

    expect(options.refreshToken).toHaveBeenCalledOnce();
    expect(store.accessToken).toBe("access-refresh");
    expect(store.userInfo?.sub).toBe(user.sub);
    store.stopAutoRefresh();
  });

  it("coordinates a login signaled by another tab through the storage event", async () => {
    const options = makeOptions("tabs");
    const store = createPortalAuthStore(options)();
    store.init();

    window.dispatchEvent(
      new StorageEvent("storage", {
        key: `${options.storagePrefix}:userInfo`,
        newValue: JSON.stringify(user),
        storageArea: window.localStorage
      })
    );
    await vi.waitFor(() => expect(store.accessToken).toBe("access-refresh"));

    expect(options.refreshToken).toHaveBeenCalledOnce();
    expect(store.userInfo?.sub).toBe(user.sub);
    store.stopAutoRefresh();
  });

  it("delegates recent authentication without replacing the access token", async () => {
    const recentAuth = vi.fn().mockResolvedValue({ message: "重新认证成功" });
    const options = makeOptions("reauth", { recentAuth });
    const store = createPortalAuthStore(options)();
    await store.login("alice", "password");

    await store.reauthenticate("current-password", "123456");

    expect(recentAuth).toHaveBeenCalledWith("current-password", "123456");
    expect(store.accessToken).toBe("access-1");
    store.stopAutoRefresh();
  });
});
