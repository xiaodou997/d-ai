import { flushPromises, mount } from "@vue/test-utils";
import { createMemoryHistory, createRouter } from "vue-router";
import { beforeEach, describe, expect, it, vi } from "vitest";

import ActivationWorkspace from "./ActivationWorkspace.vue";
import { buildActivationUrl } from "@/platform/auth/activation";

const getPasswordPolicy = vi.hoisted(() => vi.fn());
const activateAccount = vi.hoisted(() => vi.fn());

vi.mock("@/api/platformPublic", () => ({
  platformPublicApi: { getPasswordPolicy, activateAccount }
}));

describe("account activation", () => {
  beforeEach(() => {
    getPasswordPolicy.mockReset();
    activateAccount.mockReset();
    getPasswordPolicy.mockResolvedValue({
      minLength: 12,
      maxBytes: 72,
      requiredCharacterClasses: 3,
      description: "至少 12 个字符，至少包含三类字符"
    });
  });

  it("keeps the activation credential out of the HTTP query string", () => {
    const url = new URL(buildActivationUrl("dai_act_secret"));

    expect(url.search).toBe("");
    expect(url.hash).toBe("#token=dai_act_secret");
  });

  it("loads the token into the form and removes it from the URL", async () => {
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [
        { path: "/activate", component: ActivationWorkspace },
        { path: "/login", component: { template: "<div>Login</div>" } }
      ]
    });
    await router.push("/activate?campaign=admin#token=dai_act_secret");
    await router.isReady();

    const wrapper = mount(ActivationWorkspace, { global: { plugins: [router] } });
    await flushPromises();

    expect((wrapper.get('input[autocomplete="one-time-code"]').element as HTMLInputElement).value)
      .toBe("dai_act_secret");
    expect(router.currentRoute.value.query).toEqual({ campaign: "admin" });
    expect(router.currentRoute.value.hash).toBe("");
  });
});
