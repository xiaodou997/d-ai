import { expect, test, type Page, type Request, type Route } from "@playwright/test";

type PortalRole = {
  userType: 1 | 2 | 3 | 4;
  username: string;
  theme: "admin" | "tenant" | "customer";
  defaultPath: string;
};

type MockState = {
  activeRole: PortalRole;
  expireNextUserInfo: boolean;
  permissionUserType?: PortalRole["userType"];
  revoked: boolean;
  refreshCount: number;
  logoutCount: number;
  requests: Array<{ method: string; path: string; body: unknown }>;
  registration?: unknown;
};

const roles: PortalRole[] = [
  { userType: 1, username: "e2e-super-admin", theme: "admin", defaultPath: "/admin/dashboard" },
  { userType: 2, username: "e2e-platform-admin", theme: "admin", defaultPath: "/admin/dashboard" },
  { userType: 3, username: "e2e-tenant", theme: "tenant", defaultPath: "/tenant/overview/business" },
  { userType: 4, username: "e2e-customer", theme: "customer", defaultPath: "/customer/workbench" }
];

const liveCredentials: Record<string, { username: string; password: string }> = {
  admin: {
    username: process.env.DAI_E2E_ADMIN_USERNAME?.trim() || "dai_admin",
    password: process.env.DAI_E2E_ADMIN_PASSWORD || "DaiAdmin123!"
  },
  platformAdmin: {
    username: process.env.DAI_E2E_PLATFORM_ADMIN_USERNAME?.trim() || "dai_platform_admin",
    password: process.env.DAI_E2E_PLATFORM_ADMIN_PASSWORD || "DaiAdmin123!"
  },
  tenant: {
    username: process.env.DAI_E2E_TENANT_USERNAME?.trim() || "dai_tenant",
    password: process.env.DAI_E2E_TENANT_PASSWORD || "DaiAdmin123!"
  },
  customer: {
    username: process.env.DAI_E2E_CUSTOMER_USERNAME?.trim() || "u_dai_user",
    password: process.env.DAI_E2E_CUSTOMER_PASSWORD || "DaiAdmin123!"
  }
};

const useMockApi = process.env.DAI_E2E_MOCK !== "0";

function credentialsFor(role: PortalRole) {
  if (!useMockApi) {
    const key = role.userType === 1 ? "admin" : role.userType === 2 ? "platformAdmin" : role.theme;
    return liveCredentials[key];
  }
  return { username: role.username, password: "Correct-Horse-47" };
}

function watchBrowserErrors(page: Page) {
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(`pageerror: ${error.message}`));
  page.on("console", (message) => {
    if (message.type() === "error") errors.push(`console: ${message.text()}`);
  });
  return errors;
}

async function fulfillJson(route: Route, payload: unknown, status = 200) {
  await route.fulfill({
    status,
    contentType: "application/json",
    body: JSON.stringify(payload)
  });
}

async function installMockApi(page: Page, role: PortalRole, options: Partial<MockState> = {}) {
  const state: MockState = {
    activeRole: role,
    expireNextUserInfo: false,
    revoked: false,
    refreshCount: 0,
    logoutCount: 0,
    requests: [],
    ...options
  };

  if (!useMockApi) return state;

  await page.route("**/*", async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    if (!url.pathname.startsWith("/api/") && !url.pathname.startsWith("/runtime/")) {
      await route.continue();
      return;
    }

    const body = readPostData(request);
    state.requests.push({ method: request.method(), path: url.pathname, body });

    if (url.pathname === "/api/auth/login" && request.method() === "POST") {
      const username = typeof body === "object" && body !== null && "username" in body
        ? String((body as { username?: unknown }).username)
        : "";
      const matched = roles.find((item) => item.username === username);
      if (matched) state.activeRole = matched;
      await fulfillJson(route, { accessToken: `e2e-access-${state.activeRole.userType}`, expiresIn: 900, refreshExpiresIn: 604800 });
      return;
    }

    if (url.pathname === "/api/auth/me" && request.method() === "GET") {
      if (state.expireNextUserInfo) {
        state.expireNextUserInfo = false;
        await fulfillJson(route, { status: 401, title: "Unauthorized", detail: "session expired" }, 401);
        return;
      }
      const userType = state.permissionUserType ?? state.activeRole.userType;
      await fulfillJson(route, userInfoFor(userType, state.activeRole.username));
      return;
    }

    if (url.pathname === "/api/auth/refresh" && request.method() === "POST") {
      state.refreshCount += 1;
      if (state.revoked) {
        await fulfillJson(route, { status: 401, title: "Unauthorized", detail: "session revoked" }, 401);
        return;
      }
      await fulfillJson(route, { accessToken: `e2e-refreshed-${state.refreshCount}`, expiresIn: 900, refreshExpiresIn: 604800 });
      return;
    }

    if (url.pathname === "/api/auth/logout" && request.method() === "POST") {
      state.logoutCount += 1;
      state.revoked = true;
      await fulfillJson(route, { success: true });
      return;
    }

    if (url.pathname === "/api/auth/password-policy") {
      await fulfillJson(route, {
        minLength: 12,
        maxBytes: 72,
        requiredCharacterClasses: 3,
        description: "至少 12 个字符，至少包含三类字符"
      });
      return;
    }

    if (url.pathname === "/api/v1/public/invitations/ABCD2345" && request.method() === "GET") {
      await fulfillJson(route, invitationFixture());
      return;
    }

    if (url.pathname === "/api/v1/public/invitations/ABCD2345/registrations" && request.method() === "POST") {
      state.registration = body;
      await fulfillJson(route, { success: true, userId: "e2e-registered", message: "registered" });
      return;
    }

    if (url.pathname.includes("messages:stream")) {
      await route.fulfill({ status: 200, contentType: "text/event-stream", body: "event: done\ndata: [DONE]\n\n" });
      return;
    }

    await fulfillJson(route, fixtureFor(url.pathname, request.method()));
  });

  return state;
}

async function loginAs(page: Page, role: PortalRole, redirect = "/overview", options: Partial<MockState> = {}) {
  const state = await installMockApi(page, role, options);
  const credentials = credentialsFor(role);
  await page.goto(`/login?redirect=${encodeURIComponent(redirect)}`);
  await page.locator('input[name="username"]').fill(credentials.username);
  await page.locator('input[name="password"]').fill(credentials.password);
  await page.getByRole("button", { name: "登录" }).click();
  return state;
}

for (const role of roles) {
  test(`${role.userType}: login resolves the correct shell and authorized menu`, async ({ page }) => {
    const errors = watchBrowserErrors(page);
    await loginAs(page, role);

    await expect(page).toHaveURL(new RegExp(`${escapeRegExp(role.defaultPath)}$`));
    await expect(page.locator(".ds-app-shell")).toHaveClass(new RegExp(`ds-theme-${role.theme}`));
    await expect(page.locator(".ds-topbar__user-name")).toHaveText(credentialsFor(role).username);

    const menu = (await page.locator(".ds-sidebar__link").allTextContents()).map((item) => item.trim()).filter(Boolean);
    if (role.userType === 1) {
      expect(menu).toContain("管理员与身份");
      expect(menu).toContain("系统模块");
    } else if (role.userType === 2) {
      expect(menu).not.toContain("管理员与身份");
      expect(menu).toContain("系统模块");
    } else if (role.userType === 3) {
      expect(menu).toContain("用户管理");
      expect(menu).toContain("AI 对话");
      expect(menu).not.toContain("系统模块");
    } else {
      expect(menu).toContain("工作台");
      expect(menu).toContain("模型定价");
      expect(menu).not.toContain("用户管理");
    }
    expect(errors).toEqual([]);
  });
}

test("invitation registration submits legal versions and reaches success", async ({ page }) => {
  test.skip(!useMockApi, "requires the deterministic invitation fixture; run with DAI_E2E_MOCK=1");
  const errors = watchBrowserErrors(page);
  const state = await installMockApi(page, roles[3]);
  await page.goto("/register/ABCD2345");

  await expect(page.getByRole("heading", { name: "加入 示例工作区" })).toBeVisible();
  await page.locator('input[name="username"]').fill("registered-user");
  await page.locator('input[name="password"]').fill("Correct-Horse-47");
  await page.locator('input[name="confirmPassword"]').fill("Correct-Horse-47");
  await page.locator('input[name="email"]').fill("registered@example.com");
  await page.locator('input[name="accepted"]').check();
  await page.getByRole("button", { name: "创建账号" }).click();

  await expect(page.getByRole("heading", { name: "注册成功" })).toBeVisible();
  expect(state.registration).toMatchObject({
    username: "registered-user",
    email: "registered@example.com",
    termsVersion: "2026-07-19",
    privacyVersion: "2026-07-19"
  });
  expect(errors).toEqual([]);
});

test("tenant and customer AI, user, usage, account, recharge, and subscription paths load", async ({ page }) => {
  test.skip(!useMockApi, "uses deterministic cross-role API fixtures; run with DAI_E2E_MOCK=1");
  const errors = watchBrowserErrors(page);
  await loginAs(page, roles[2], "/tenant/users/directory");
  const tenantPaths: Array<[string, string]> = [
    ["/tenant/users/directory", "用户管理"],
    ["/tenant/developer/keys", "API 密钥"],
    ["/tenant/workbench/chat", "AI 对话"],
    ["/tenant/workbench/images", "AI 生图"],
    ["/tenant/workbench/tasks", "任务中心"],
    ["/tenant/ai/usage", "使用记录"]
  ];
  for (const [path, label] of tenantPaths) {
    await page.goto(path);
    await expect(page.getByText(label, { exact: true }).first()).toBeVisible();
  }

  await loginAs(page, roles[3], "/customer/workbench");
  const customerPaths: Array<[string, string]> = [
    ["/customer/workbench/chat", "AI 对话"],
    ["/customer/workbench/images", "AI 生图"],
    ["/customer/workbench/tasks", "我的任务"],
    ["/customer/usage", "使用记录"],
    ["/customer/account/overview", "我的余额"],
    ["/customer/account/topup", "选择充值金额"],
    ["/customer/services/subscription", "订阅套餐"]
  ];
  for (const [path, label] of customerPaths) {
    await page.goto(path);
    await expect(page.getByText(label, { exact: true }).first()).toBeVisible();
  }
  expect(errors).toEqual([]);
});

test("admin billing and usage paths expose recharge/refund workflow surfaces", async ({ page }) => {
  test.skip(!useMockApi, "uses deterministic billing fixtures; run with DAI_E2E_MOCK=1");
  const errors = watchBrowserErrors(page);
  const state = await loginAs(page, roles[0], "/admin/settlement/recharges");
  await page.goto("/admin/settlement/recharges");
  await expect(page.getByText("充值订单", { exact: true }).first()).toBeVisible();
  await page.goto("/admin/ai/usage");
  await expect(page.getByText("使用记录", { exact: true }).first()).toBeVisible();
  await page.goto("/admin/settlement/withdrawals");
  await expect(page.getByText("提现记录", { exact: true }).first()).toBeVisible();
  expect(state.requests.some((request) => request.path.includes("/admin/recharge-orders"))).toBe(true);
  expect(errors).toEqual([]);
});

test("refresh recovery, cross-tab logout, and permission changes are enforced by the router", async ({ page }) => {
  test.skip(!useMockApi, "uses deterministic session rotation fixtures; run with DAI_E2E_MOCK=1");
  const errors = watchBrowserErrors(page);
  const state = await loginAs(page, roles[3], "/customer/workbench", { expireNextUserInfo: true });
  await expect(page).toHaveURL(/\/customer\/workbench$/);
  expect(state.refreshCount).toBe(1);

  errors.splice(0);
  state.revoked = true;
  await page.evaluate(() => {
    localStorage.removeItem("dai:portal:userInfo");
    window.dispatchEvent(new StorageEvent("storage", { key: "dai:portal:userInfo", newValue: null, storageArea: localStorage }));
  });
  await page.reload();
  await expect(page).toHaveURL(/\/login\?redirect=%2Fcustomer%2Fworkbench$/);
  errors.splice(0);

  const permissionState = await loginAs(page, roles[2], "/tenant/overview/business");
  permissionState.permissionUserType = 4;
  await page.goto("/admin/dashboard");
  await expect(page).toHaveURL(/\/customer\/workbench$/);
  expect(errors).toEqual([]);
});

test.afterEach(async ({ page }, testInfo) => {
  await page.screenshot({ path: testInfo.outputPath("portal.png"), fullPage: true });
});

function readPostData(request: Request) {
  try {
    return request.postDataJSON?.() ?? null;
  } catch {
    return request.postData?.() ?? null;
  }
}

function userInfoFor(userType: PortalRole["userType"], username: string) {
  return {
    sub: `e2e-user-${userType}`,
    username,
    userType,
    tenantId: userType >= 3 ? "e2e-tenant" : "",
    tenantName: userType >= 3 ? "E2E Tenant" : "",
    mfaEnabled: false
  };
}

function invitationFixture() {
  return {
    code: "ABCD2345",
    tenantName: "E2E Tenant",
    customerSiteName: "示例工作区",
    description: "加入团队后即可使用 AI 服务。",
    status: "active",
    canRegister: true,
    message: "",
    legal: {
      termsUrl: "/legal/terms",
      termsVersion: "2026-07-19",
      privacyUrl: "/legal/privacy",
      privacyVersion: "2026-07-19"
    }
  };
}

function fixtureFor(path: string, method: string): unknown {
  if (path === "/api/v1/customer/portal-brand") return { siteName: "D-AI", faviconPath: "" };
  if (path.includes("topup-config")) {
    return {
      enabled: true,
      currency: "USD",
      feeRateBp: 0,
      minMicroUsd: 1_000_000,
      maxMicroUsd: 100_000_000,
      validityDays: 30,
      packages: [{ id: "e2e-package", name: "体验包", paymentAmountMinor: 100, paymentAmountMicroUsd: 1_000_000, giftAmountMicroUsd: 0, enabled: true }]
    };
  }
  if (path.includes("balance")) {
    return { currency: "USD", totalUsd: 10, remainingUsd: 10, availableUsd: 10, usedUsd: 0, permanentUsd: 10, timedUsd: 0, outstandingDebtMicroUsd: 0, serviceState: "active", balanceLots: [] };
  }
  if (path.endsWith("/subscriptions/current")) return null;
  if (path.includes("dashboard/summary")) return { total_requests: 0, success_requests: 0, failed_requests: 0, total_tokens: 0, total_amount_usd: 0, avg_latency_ms: 0, total_tenant_payable_usd: 0, total_user_charged_usd: 0 };
  if (path.includes("summary") && path.includes("usage")) return { total_requests: 0, success_requests: 0, failed_requests: 0, total_tokens: 0, total_amount_usd: 0, avg_latency_ms: 0 };
  if (path.includes("public/invitations")) return invitationFixture();
  if (path.startsWith("/runtime/")) return runtimeFixture(path, method);
  if (path.includes("/models") || path.includes("/sessions") || path.includes("/api-keys") || path.includes("/groups") || path.includes("subscription") || path.includes("usage") || path.includes("tasks") || path.includes("users") || path.includes("tenants") || path.includes("recharge") || path.includes("orders") || path.includes("ledger") || path.includes("announcements") || path.includes("audit")) {
    return { items: [], total: 0, page: 1, size: 20, included: [], records: [] };
  }
  if (method === "DELETE") return { deleted: true };
  return { success: true, items: [], total: 0 };
}

function runtimeFixture(path: string, method: string): unknown {
  if (path.includes("messages:stream")) return "event: done\ndata: [DONE]\n\n";
  if (path.includes("/images/tasks") && method === "POST") {
    return { data: { id: "e2e-image-task", operation: "generation", status: "queued" } };
  }
  if (path.includes("/tasks") && !path.includes("/images")) return { data: { items: [], has_more: false } };
  return { data: [] };
}

function escapeRegExp(value: string) {
  return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}
