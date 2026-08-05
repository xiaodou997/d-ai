import { describe, expect, it } from "vitest";

import {
  createStandardPortalEnv,
  enabledPortalServices,
  portalModuleForClientID
} from "./env";

function createEnv(env: Record<string, string> = {}) {
  return createStandardPortalEnv({ env });
}

describe("unified Portal service modules", () => {
  it("uses the two registered service client IDs", () => {
    const env = createEnv();

    expect(env.portal).toBe("unified");
    expect(env.serviceClientIds).toEqual({
      urm: "dai-portal",
      ai: "dai-portal"
    });
    expect(portalModuleForClientID(env, "dai-portal")?.label).toBe("用户中心");
  });

  it("resolves configured client IDs", () => {
    const env = createEnv({
      VITE_URM_CLIENT_ID: "identity-service",
      VITE_AI_CLIENT_ID: "custom-ai"
    });

    expect(portalModuleForClientID(env, "identity-service")?.service).toBe("urm");
    expect(portalModuleForClientID(env, "custom-ai")?.service).toBe("ai");
    expect(portalModuleForClientID(env, "dai-portal")).toBeUndefined();
  });

  it("filters AI by capabilities while URM is always enabled", () => {
    const env = createEnv({ VITE_AI_CLIENT_ID: "custom-ai" });

    expect(enabledPortalServices(env, [])).toEqual(["urm"]);
    expect(enabledPortalServices(env, ["custom-ai", "unknown-service"])).toEqual([
      "urm",
      "ai"
    ]);
  });
});
