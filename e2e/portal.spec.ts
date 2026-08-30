import AxeBuilder from "@axe-core/playwright";
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
  balanceUsd: number;
  topupOrderCreated: boolean;
  topupPollCount: number;
  subscriptionPurchased: boolean;
  apiKeyCreated: boolean;
  tenantUserCreated: boolean;
  chatSessionCreated: boolean;
  chatMessageSent: boolean;
  imageTaskCreated: boolean;
  adminRefunded: boolean;
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

async function expectNoAccessibilityViolations(page: Page) {
  const results = await new AxeBuilder({ page }).analyze();
  const violations = results.violations.flatMap((item) =>
    item.nodes.map((node) => `${item.id} ${node.target.join(" ")}: ${item.help}`)
  );
  expect(violations, violations.join("\n")).toEqual([]);
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
    balanceUsd: 10,
    topupOrderCreated: false,
    topupPollCount: 0,
    subscriptionPurchased: false,
    apiKeyCreated: false,
    tenantUserCreated: false,
    chatSessionCreated: false,
    chatMessageSent: false,
    imageTaskCreated: false,
    adminRefunded: false,
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

    if (url.pathname === "/api/v1/account/balance" && request.method() === "GET") {
      await fulfillJson(route, balanceFixture(state));
      return;
    }

    if (url.pathname === "/api/v1/payments/topup-config" && request.method() === "GET") {
      await fulfillJson(route, topupConfigFixture());
      return;
    }

    if (url.pathname === "/api/v1/payments/topup-orders" && request.method() === "POST") {
      state.topupOrderCreated = true;
      await fulfillJson(route, topupOrderCreatedFixture());
      return;
    }

    if (url.pathname.startsWith("/api/v1/payments/topup-orders/") && request.method() === "GET") {
      state.topupPollCount += 1;
      state.balanceUsd = 15;
      await fulfillJson(route, topupOrderStatusFixture());
      return;
    }

    if (url.pathname === "/api/v1/admin/recharge-orders" && request.method() === "GET") {
      await fulfillJson(route, { items: [adminRechargeOrderFixture(state)], total: 1, page: 1, size: 20 });
      return;
    }

    if (url.pathname === "/api/v1/admin/recharge-orders/e2e-refund-order" && request.method() === "GET") {
      await fulfillJson(route, adminRechargeOrderFixture(state));
      return;
    }

    if (url.pathname === "/api/v1/admin/recharge-orders/e2e-refund-order/refund-reversal" && request.method() === "POST") {
      state.adminRefunded = true;
      await fulfillJson(route, adminRechargeOrderFixture(state));
      return;
    }

    if (url.pathname === "/api/v1/users" && request.method() === "GET") {
      await fulfillJson(route, { items: [tenantUserFixture(state)], total: 1, page: 1, size: 20 });
      return;
    }

    if (url.pathname === "/api/v1/users" && request.method() === "POST") {
      state.tenantUserCreated = true;
      await fulfillJson(route, {
        userId: "e2e-new-user",
        tenantId: "e2e-tenant",
        username: "created-user",
        activationToken: "e2e-activation-token",
        activationExpiresIn: 86_400
      });
      return;
    }

    if (url.pathname === "/api/v1/user-api-keys" && request.method() === "GET") {
      await fulfillJson(route, { items: state.apiKeyCreated ? [apiKeyFixture()] : [], total: state.apiKeyCreated ? 1 : 0 });
      return;
    }

    if (url.pathname === "/api/v1/users/me/groups" && request.method() === "GET") {
      await fulfillJson(route, { items: [apiKeyGroupFixture()], total: 1 });
      return;
    }

    if (url.pathname === "/api/v1/users/me/api-keys" && request.method() === "POST") {
      state.apiKeyCreated = true;
      await fulfillJson(route, { plaintext_key: "dai-e2e-plaintext-key", key: apiKeyFixture() });
      return;
    }

    if (url.pathname === "/api/v1/users/me/subscription-plans" && request.method() === "GET") {
      await fulfillJson(route, { items: [subscriptionPlanFixture()], total: 1, page: 1, size: 100 });
      return;
    }

    if (url.pathname === "/api/v1/users/me/subscriptions/current" && request.method() === "GET") {
      await fulfillJson(route, state.subscriptionPurchased ? subscriptionFixture() : null);
      return;
    }

    if (url.pathname === "/api/v1/users/me/subscriptions" && request.method() === "GET") {
      await fulfillJson(route, { items: state.subscriptionPurchased ? [subscriptionFixture()] : [], total: state.subscriptionPurchased ? 1 : 0, page: 1, size: 50 });
      return;
    }

    if (url.pathname === "/api/v1/users/me/subscription-orders" && request.method() === "GET") {
      await fulfillJson(route, { items: state.subscriptionPurchased ? [subscriptionOrderFixture()] : [], total: state.subscriptionPurchased ? 1 : 0, page: 1, size: 50 });
      return;
    }

    if (url.pathname === "/api/v1/users/me/subscription-orders" && request.method() === "POST") {
      state.subscriptionPurchased = true;
      state.balanceUsd = 13;
      await fulfillJson(route, { processing: false, order: subscriptionOrderFixture(), subscription: subscriptionFixture() }, 201);
      return;
    }

    if (url.pathname === "/api/v1/users/me/workspace/chat/models" && request.method() === "GET") {
      await fulfillJson(route, { items: [chatModelFixture()], total: 1 });
      return;
    }

    if (url.pathname === "/api/v1/users/me/workspace/chat/sessions" && request.method() === "GET") {
      await fulfillJson(route, { items: state.chatSessionCreated ? [chatSessionFixture()] : [], total: state.chatSessionCreated ? 1 : 0 });
      return;
    }

    if (url.pathname === "/api/v1/users/me/workspace/chat/sessions" && request.method() === "POST") {
      state.chatSessionCreated = true;
      await fulfillJson(route, chatSessionFixture(), 201);
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
      state.chatMessageSent = true;
      await route.fulfill({
        status: 200,
        contentType: "text/event-stream",
        body: "event: delta\ndata: {\"delta\":\"E2E 回复已生成\"}\n\nevent: done\ndata: [DONE]\n\n"
      });
      return;
    }

    await fulfillJson(route, fixtureFor(url.pathname, request.method(), state));
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
    await page.waitForLoadState("networkidle");
    await expectNoAccessibilityViolations(page);
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

test("tenant user management creates an activation credential", async ({ page }) => {
  test.skip(!useMockApi, "uses deterministic tenant user fixtures; run with DAI_E2E_MOCK=1");
  const errors = watchBrowserErrors(page);
  const state = await loginAs(page, roles[2], "/tenant/users/directory");
  await page.goto("/tenant/users/directory");
  await expect(page.getByText("e2e-end-user", { exact: true }).first()).toBeVisible();

  await page.getByRole("button", { name: "创建用户" }).first().click();
  const createDialog = page.getByRole("dialog", { name: "创建终端用户" });
  await createDialog.getByLabel("用户名").fill("created-user");
  await createDialog.getByLabel("邮箱").fill("created@example.com");
  await createDialog.getByRole("button", { name: "创建" }).click();

  await expect.poll(() => state.tenantUserCreated).toBe(true);
  const activationBox = page.locator(".el-message-box");
  await expect(activationBox).toBeVisible();
  // Clipboard is unavailable in the deterministic browser fixture, so the
  // UI may show a manual-copy prompt while the first dialog's close
  // transition is still removing its DOM node. Always act on the topmost
  // dialog instead of querying a strict locator during that overlap.
  for (let attempt = 0; attempt < 3; attempt += 1) {
    const current = activationBox.last();
    if (!(await current.isVisible())) break;
    await current.getByRole("button").last().evaluate((button) => (button as HTMLButtonElement).click());
    await page.waitForTimeout(150);
  }
  await expect(page.getByText("created-user", { exact: true })).toBeVisible();
  expect(errors).toEqual([]);
});

test("customer API key, chat, and image workflows submit runtime requests", async ({ page }) => {
  test.skip(!useMockApi, "uses deterministic customer runtime fixtures; run with DAI_E2E_MOCK=1");
  const errors = watchBrowserErrors(page);
  const state = await loginAs(page, roles[3], "/customer/developer/keys");

  await page.goto("/customer/developer/keys");
  await expect(page.getByRole("heading", { name: "API 密钥" }).last()).toBeVisible();
  await page.getByRole("button", { name: "创建模型 API 密钥" }).first().click();
  const keyDialog = page.getByRole("dialog", { name: "创建模型 API 密钥" });
  await keyDialog.getByLabel("名称").fill("E2E Key");
  const groupSelect = keyDialog.getByRole("combobox");
  await groupSelect.click();
  await page.getByRole("option", { name: /默认分组/ }).click();
  await keyDialog.getByRole("button", { name: "创建" }).click();
  await expect.poll(() => state.apiKeyCreated).toBe(true);
  await expect(page.getByRole("dialog", { name: "模型 API 密钥明文" })).toContainText("dai-e2e-plaintext-key");
  await page.getByRole("dialog", { name: "模型 API 密钥明文" }).getByRole("button", { name: "关闭", exact: true }).click();

  await page.goto("/customer/workbench/chat");
  await expect(page.getByRole("heading", { name: "AI 对话" })).toBeVisible();
  const chatInput = page.getByPlaceholder("输入消息，Enter 发送，Shift + Enter 换行");
  await chatInput.fill("你好，D-AI");
  await page.getByRole("button", { name: "发送消息" }).click();
  await expect.poll(() => state.chatMessageSent).toBe(true);
  await expect(page.getByText("E2E 回复已生成", { exact: true })).toBeVisible();

  await page.goto("/customer/workbench/images");
  await expect(page.getByRole("heading", { name: "AI 生图" })).toBeVisible();
  await page.getByPlaceholder("描述你希望生成的画面").fill("一只 E2E 小猫");
  await page.getByRole("button", { name: "生成图片" }).click();
  await expect.poll(() => state.imageTaskCreated).toBe(true);
  await expect(page.getByText("一只 E2E 小猫", { exact: true })).toBeVisible();
  expect(errors).toEqual([]);
});

test("customer recharge and subscription update the account balance", async ({ page }) => {
  test.skip(!useMockApi, "uses deterministic payment fixtures; run with DAI_E2E_MOCK=1");
  const errors = watchBrowserErrors(page);
  const state = await loginAs(page, roles[3], "/customer/account/topup");

  await page.goto("/customer/account/topup");
  await expect(page.getByText("选择充值金额", { exact: true })).toBeVisible();
  await page.locator("button.package-card").filter({ hasText: "体验包" }).click();
  await expect.poll(() => state.topupOrderCreated).toBe(true);
  await expect(page.getByRole("dialog", { name: "微信扫码支付" })).toBeVisible();
  await expect.poll(() => state.topupPollCount, { timeout: 7_000 }).toBeGreaterThan(0);
  await expect(page.getByText("充值成功，USD 额度已到账", { exact: true })).toBeVisible();

  await page.goto("/customer/account/overview");
  await expect(page.getByText("$15.00", { exact: true }).first()).toBeVisible();

  await page.goto("/customer/services/subscription");
  await expect(page.getByText("成长套餐", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "购买" }).click();
  await expect(page.getByRole("dialog", { name: "确认购买订阅" })).toBeVisible();
  await page.getByRole("dialog", { name: "确认购买订阅" }).getByRole("button", { name: "确认购买" }).click();
  await expect.poll(() => state.subscriptionPurchased).toBe(true);
  await page.goto("/customer/account/overview");
  await expect(page.getByText("$13.00", { exact: true }).first()).toBeVisible();
  expect(errors).toEqual([]);
});

test("admin refund workflow records a completed refund and reversal", async ({ page }) => {
  test.skip(!useMockApi, "uses deterministic refund fixtures; run with DAI_E2E_MOCK=1");
  const errors = watchBrowserErrors(page);
  const state = await loginAs(page, roles[0], "/admin/settlement/recharges");

  await page.goto("/admin/settlement/recharges");
  await expect(page.getByText("e2e-refund-order", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "查看详情" }).click();
  const drawer = page.getByRole("dialog", { name: "充值订单详情" });
  await expect(drawer).toBeVisible();
  await drawer.getByRole("button", { name: "登记退款并冲正" }).click();
  const refundDialog = page.getByRole("dialog", { name: "登记已完成退款并整单冲正" });
  await refundDialog.getByLabel("商户退款单号").fill("e2e-refund-reference");
  await refundDialog.getByLabel("微信退款单号").fill("e2e-channel-refund");
  await refundDialog.getByLabel("退款原因").fill("E2E 用户退款");
  await refundDialog.getByRole("button", { name: "确认已退款并整单冲正" }).click();
  await expect.poll(() => state.adminRefunded).toBe(true);
  await expect(drawer.getByText("已退款", { exact: true }).first()).toBeVisible();
  const refundRequest = state.requests.find((request) => request.path.endsWith("/refund-reversal"));
  expect(refundRequest?.body).toMatchObject({
    method: "wechat",
    refundReference: "e2e-refund-reference",
    channelRefundId: "e2e-channel-refund",
    reason: "E2E 用户退款"
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

function balanceFixture(state: MockState) {
  return {
    currency: "USD",
    totalUsd: state.balanceUsd,
    usedUsd: 0,
    remainingUsd: state.balanceUsd,
    availableUsd: state.balanceUsd,
    permanentUsd: state.balanceUsd,
    timedUsd: 0,
    outstandingDebtMicroUsd: 0,
    serviceState: "active",
    balanceLots: [{
      balanceLotId: "e2e-balance-lot",
      totalUsd: state.balanceUsd,
      remainingUsd: state.balanceUsd,
      createdAt: "2026-08-30T00:00:00Z",
      expiresAt: null,
      source: state.topupOrderCreated ? "微信充值" : "管理员充值"
    }]
  };
}

function topupConfigFixture() {
  return {
    enabled: true,
    currency: "USD",
    feeRateBp: 0,
    minMicroUsd: 1_000_000,
    maxMicroUsd: 100_000_000,
    validityDays: 30,
    packages: [{
      id: "e2e-package",
      name: "体验包",
      paymentAmountMinor: 500,
      paymentAmountMicroUsd: 5_000_000,
      giftAmountMicroUsd: 0,
      validityDays: 30,
      enabled: true,
      sortOrder: 1
    }]
  };
}

function topupOrderCreatedFixture() {
  return {
    orderId: "e2e-topup-order",
    codeUrl: "https://pay.example.test/e2e-topup-order",
    paymentCurrency: "USD",
    paymentAmountMinor: 500,
    grossAmountMicroUsd: 5_000_000,
    feeAmountMicroUsd: 0,
    giftAmountMicroUsd: 0,
    creditedAmountMicroUsd: 5_000_000,
    topupMode: "package",
    packageName: "体验包",
    status: "created",
    expiresAt: Date.now() + 600_000,
    balanceExpiresAt: null
  };
}

function topupOrderStatusFixture() {
  return {
    ...topupOrderCreatedFixture(),
    status: "paid",
    transactionId: "e2e-wechat-transaction",
    paidAt: Date.now()
  };
}

function tenantUserFixture(state: MockState) {
  return {
    userId: state.tenantUserCreated ? "e2e-new-user" : "e2e-end-user",
    tenantId: "e2e-tenant",
    tenantName: "E2E Tenant",
    username: state.tenantUserCreated ? "created-user" : "e2e-end-user",
    email: "user@example.com",
    phone: "",
    status: 1,
    credentialState: "active",
    balanceUsd: 3,
    createdTime: Date.parse("2026-08-30T00:00:00Z")
  };
}

function apiKeyGroupFixture() {
  return {
    id: "e2e-group",
    name: "默认分组",
    description: "E2E 可用分组",
    effective_user_multiplier: 1,
    status: "active"
  };
}

function apiKeyFixture() {
  return {
    id: "e2e-api-key",
    owner_type: "user",
    tenant_id: "e2e-tenant",
    user_id: "e2e-user-4",
    group_id: "e2e-group",
    last_four: "1234",
    name: "E2E Key",
    quota_limit_micro_usd: null,
    quota_used_micro_usd: 0,
    status: "active",
    expires_at: null,
    last_used_at: null,
    limit_policy: null,
    created_by: "e2e-user-4",
    created_at: Date.parse("2026-08-30T00:00:00Z"),
    updated_at: Date.parse("2026-08-30T00:00:00Z")
  };
}

function subscriptionPolicyFixture() {
  return {
    lifetime_max_purchases: null,
    period_type: "none",
    period_max_purchases: null,
    rolling_window_hours: null,
    calendar_unit: undefined,
    calendar_timezone: undefined,
    allow_advance_purchase: true,
    version: 1
  };
}

function subscriptionPlanFixture() {
  return {
    id: "e2e-plan",
    name: "成长套餐",
    description: "E2E 订阅套餐",
    price_micro_usd: 2_000_000,
    duration_days: 30,
    total_limit_micro_usd: 10_000_000,
    window_5h_limit_micro_usd: 5_000_000,
    window_7d_limit_micro_usd: null,
    sale_limit: null,
    sold_count: 0,
    available_count: null,
    sold_out: false,
    groups: [{ id: "e2e-group", name: "默认分组", quota_debit_multiplier: 1 }],
    purchase_policy: subscriptionPolicyFixture(),
    purchase_eligibility: { allowed: true, blocking_rules: [] }
  };
}

function subscriptionOrderFixture() {
  return {
    id: "e2e-sub-order",
    order_no: "E2E-SUB-0001",
    tenant_id: "e2e-tenant",
    user_id: "e2e-user-4",
    plan_id: "e2e-plan",
    plan_name: "成长套餐",
    price_micro_usd: 2_000_000,
    status: "paid",
    debit_reference: "e2e-debit",
    debited_at: "2026-08-30T00:00:00Z",
    subscription_id: "e2e-subscription",
    purchase_policy_version: 1,
    purchase_policy: subscriptionPolicyFixture(),
    paid_at: "2026-08-30T00:00:00Z",
    created_at: "2026-08-30T00:00:00Z",
    updated_at: "2026-08-30T00:00:00Z"
  };
}

function subscriptionFixture() {
  return {
    id: "e2e-subscription",
    tenant_id: "e2e-tenant",
    user_id: "e2e-user-4",
    plan_id: "e2e-plan",
    order_id: "e2e-sub-order",
    plan_name: "成长套餐",
    duration_days: 30,
    status: "active",
    activated_at: "2026-08-30T00:00:00Z",
    expires_at: "2026-09-29T00:00:00Z",
    total_limit_micro_usd: 10_000_000,
    total_used_micro_usd: 0,
    total_remaining_micro_usd: 10_000_000,
    window_5h: { limit_micro_usd: 5_000_000, used_micro_usd: 0, remaining_micro_usd: 5_000_000, reset_at: null },
    window_7d: { limit_micro_usd: null, used_micro_usd: 0, remaining_micro_usd: null, reset_at: null },
    groups: [{ id: "e2e-group", name: "默认分组", quota_debit_multiplier: 1 }],
    created_at: "2026-08-30T00:00:00Z",
    updated_at: "2026-08-30T00:00:00Z"
  };
}

function chatModelFixture() {
  return {
    group_id: "e2e-group",
    group_name: "默认分组",
    effective_user_multiplier: 1,
    billing_group_label: "默认分组",
    model_code: "e2e-chat-model",
    capability_type: "chat",
    default_api_format: "openai",
    available_api_formats: ["openai"],
    supports_stream: true,
    status: "active"
  };
}

function chatSessionFixture() {
  return {
    id: "e2e-chat-session",
    title: "新对话",
    model_code: "e2e-chat-model",
    group_id: "e2e-group",
    group_name: "默认分组",
    effective_user_multiplier: 1,
    billing_group_label: "默认分组",
    provider_api_format: "openai",
    selected_route_id: "e2e-route",
    status: "active",
    created_at: Date.parse("2026-08-30T00:00:00Z"),
    updated_at: Date.parse("2026-08-30T00:00:00Z")
  };
}

function imageModelFixture() {
  return {
    group_id: "e2e-group",
    group_name: "默认分组",
    effective_user_multiplier: 1,
    billing_group_label: "默认分组",
    model_code: "e2e-image-model",
    capability_type: "image",
    status: "active",
    max_output_count: 1,
    edit_max_output_count: 1
  };
}

function imageJobFixture(state: MockState) {
  return {
    id: "e2e-image-task",
    operation: "generation",
    group_id: "e2e-group",
    model_code: "e2e-image-model",
    prompt: "一只 E2E 小猫",
    status: state.imageTaskCreated ? "completed" : "pending",
    storage_policy: "temporary",
    raw_image_retained: false,
    size: "1024x1024",
    requested_output_count: 1,
    caller_charge_usd: 0.1,
    image_count: 0,
    inline_count: 0,
    url_count: 0,
    assets: [],
    created_at: Date.parse("2026-08-30T00:00:00Z"),
    completed_at: state.imageTaskCreated ? Date.parse("2026-08-30T00:00:01Z") : undefined
  };
}

function adminRechargeOrderFixture(state: MockState) {
  const refunded = state.adminRefunded;
  return {
    orderId: "e2e-refund-order",
    balanceOrderId: "e2e-balance-order",
    method: "online",
    targetType: "user",
    orderType: "online_user_topup",
    tenantId: "e2e-tenant",
    tenantName: "E2E Tenant",
    userId: "e2e-user-4",
    username: "e2e-customer",
    paidAmountMinor: 500,
    grossAmountMicroUsd: 5_000_000,
    feeAmountMicroUsd: 0,
    giftAmountMicroUsd: 0,
    creditedAmountMicroUsd: 5_000_000,
    tenantIncomeMicroUsd: 1_000_000,
    paymentStatus: "paid",
    fulfillmentStatus: refunded ? "reversed" : "credited",
    refundStatus: refunded ? "refunded" : "none",
    outTradeNo: "e2e-out-trade",
    transactionId: "e2e-wechat-tx",
    topupMode: "package",
    packageName: "体验包",
    channel: "wechat_native",
    note: "E2E 退款订单",
    createdAt: Date.parse("2026-08-30T00:00:00Z"),
    paidAt: Date.parse("2026-08-30T00:00:01Z"),
    paymentExpiresAt: null,
    balanceExpiresAt: null,
    credits: [{
      balanceOrderId: "e2e-balance-order",
      orderType: "online_user_topup",
      primary: true,
      creditAmountMicroUsd: 5_000_000,
      status: refunded ? "reversed" : "credited",
      reversedAmountMicroUsd: refunded ? 5_000_000 : 0,
      lostAmountMicroUsd: 0,
      grantedAmountMicroUsd: 5_000_000,
      consumedAmountMicroUsd: 0,
      remainingAmountMicroUsd: refunded ? 0 : 5_000_000,
      lotStatus: refunded ? "reversed" : "active",
      refundAvailableMicroUsd: refunded ? 5_000_000 : 0,
      refundNonAvailableMicroUsd: 0,
      refundExpiredMicroUsd: 0,
      refundAccountDebitMicroUsd: refunded ? 5_000_000 : 0,
      refundBalanceAfterMicroUsd: refunded ? 0 : 5_000_000
    }],
    refund: refunded ? {
      refundId: "e2e-refund",
      method: "wechat",
      refundReference: "e2e-refund-reference",
      channelRefundId: "e2e-channel-refund",
      refundAmountMinor: 500,
      status: "completed",
      refundedAt: Date.parse("2026-08-30T00:01:00Z"),
      reason: "E2E 用户退款",
      note: "E2E 退款备注",
      operatorId: "e2e-super-admin",
      createdAt: Date.parse("2026-08-30T00:01:00Z")
    } : undefined
  };
}

function fixtureFor(path: string, method: string, state: MockState): unknown {
  if (path === "/api/v1/customer/portal-brand") return { siteName: "D-AI", faviconPath: "" };
  if (path.includes("topup-config")) return topupConfigFixture();
  if (path.includes("balance")) return balanceFixture(state);
  if (path.endsWith("/subscriptions/current")) return null;
  if (path.includes("dashboard/summary")) return { total_requests: 0, success_requests: 0, failed_requests: 0, total_tokens: 0, total_amount_usd: 0, avg_latency_ms: 0, total_tenant_payable_usd: 0, total_user_charged_usd: 0 };
  if (path.includes("summary") && path.includes("usage")) return { total_requests: 0, success_requests: 0, failed_requests: 0, total_tokens: 0, total_amount_usd: 0, avg_latency_ms: 0 };
  if (path.includes("public/invitations")) return invitationFixture();
  if (path.startsWith("/runtime/")) return runtimeFixture(path, method, state);
  if (path.includes("/models") || path.includes("/sessions") || path.includes("/api-keys") || path.includes("/groups") || path.includes("subscription") || path.includes("usage") || path.includes("tasks") || path.includes("users") || path.includes("tenants") || path.includes("recharge") || path.includes("orders") || path.includes("ledger") || path.includes("announcements") || path.includes("audit")) {
    return { items: [], total: 0, page: 1, size: 20, included: [], records: [] };
  }
  if (method === "DELETE") return { deleted: true };
  return { success: true, items: [], total: 0 };
}

function runtimeFixture(path: string, method: string, state: MockState): unknown {
  if (path.includes("messages:stream")) return "event: done\ndata: [DONE]\n\n";
  if (path.endsWith("/images/models")) return { data: [imageModelFixture()] };
  if (path.endsWith("/images/tasks") && method === "POST") {
    state.imageTaskCreated = true;
    return { data: { task_id: "e2e-image-task", status: "pending" } };
  }
  if (path.endsWith("/images/tasks/e2e-image-task")) return { data: imageJobFixture(state) };
  if (path.includes("/tasks") && !path.includes("/images")) return { data: { items: [], has_more: false } };
  return { data: [] };
}

function escapeRegExp(value: string) {
  return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}
