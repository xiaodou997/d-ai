import { flushPromises, mount } from "@vue/test-utils";
import ElementPlus, { ElMessageBox } from "element-plus";
import { createMemoryHistory, createRouter } from "vue-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import JwtKeysWorkspace from "./JwtKeysWorkspace.vue";

const api = vi.hoisted(() => ({
  listJwtKeys: vi.fn(),
  rotateJwtKey: vi.fn()
}));

vi.mock("@/api/platformAdmin", () => ({ platformAdminApi: api }));

const key = {
  id: 1,
  kid: "kid-2026-08",
  status: "active",
  createdTime: 1_700_000_000_000,
  graceUntil: undefined,
  retiredTime: undefined
};

describe("JwtKeysWorkspace", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    api.listJwtKeys.mockResolvedValue({ keys: [key], total: 1 });
    api.rotateJwtKey.mockResolvedValue({ message: "rotated" });
  });

  afterEach(() => {
    vi.restoreAllMocks();
    document.body.innerHTML = "";
  });

  it("loads and labels the active signing key", async () => {
    const { wrapper } = await mountWorkspace();

    expect(api.listJwtKeys).toHaveBeenCalledOnce();
    expect(wrapper.text()).toContain("kid-2026-08");
    expect(wrapper.text()).toContain("签发中");
    wrapper.unmount();
  });

  it("rotates the key only after confirmation and reloads the list", async () => {
    vi.spyOn(ElMessageBox, "confirm").mockResolvedValue("confirm" as never);
    const { wrapper } = await mountWorkspace();
    const rotateButton = wrapper.findAll("button").find((button) => button.text().includes("轮换密钥"));
    expect(rotateButton).toBeDefined();

    await rotateButton!.trigger("click");
    await flushPromises();
    expect(api.rotateJwtKey).toHaveBeenCalledOnce();
    expect(api.listJwtKeys).toHaveBeenCalledTimes(2);
    wrapper.unmount();
  });
});

async function mountWorkspace() {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: "/admin/identity/jwt", component: JwtKeysWorkspace }]
  });
  await router.push("/admin/identity/jwt");
  await router.isReady();
  const wrapper = mount(JwtKeysWorkspace, {
    attachTo: document.body,
    global: { plugins: [router, ElementPlus] }
  });
  await flushPromises();
  return { router, wrapper };
}
