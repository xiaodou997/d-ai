import { describe, expect, it } from "vitest";

import {
  createStandardPortalEnv,
  enabledPortalServices,
  portalModuleForClientID
} from "./env";

function createEnv(env: Record<string, string> = {}) {
  return createStandardPortalEnv({ portal: "admin", env });
}

describe("portal service modules", () => {
  it("uses the registered default client IDs", () => {
    const env = createEnv();

    expect(env.serviceClientIds).toEqual({
      urm: "urm",
      ai: "uni-ai-api",
      proxy: "uni-api-proxy"
    });
    expect(portalModuleForClientID(env, "uni-ai-api")?.label).toBe("智能服务");
    expect(portalModuleForClientID(env, "uni-api-proxy")?.label).toBe("接口代理");
  });

  it("resolves modules through configured client IDs", () => {
    const env = createEnv({
      VITE_URM_CLIENT_ID: "identity-service",
      VITE_AI_CLIENT_ID: "custom-ai",
      VITE_PROXY_CLIENT_ID: "custom-proxy"
    });

    expect(portalModuleForClientID(env, "identity-service")?.service).toBe("urm");
    expect(portalModuleForClientID(env, "custom-ai")?.service).toBe("ai");
    expect(portalModuleForClientID(env, "custom-proxy")?.service).toBe("proxy");
    expect(portalModuleForClientID(env, "uni-ai-api")).toBeUndefined();
  });

  it("does not match unknown or blank client IDs", () => {
    const env = createEnv();

    expect(portalModuleForClientID(env, "report-service")).toBeUndefined();
    expect(portalModuleForClientID(env, "   ")).toBeUndefined();
  });

  it("always enables URM and filters business modules by capabilities", () => {
    const env = createEnv();

    expect(enabledPortalServices(env, [])).toEqual(["urm"]);
    expect(enabledPortalServices(env, ["uni-api-proxy", "unknown-service"])).toEqual([
      "urm",
      "proxy"
    ]);
  });
});
