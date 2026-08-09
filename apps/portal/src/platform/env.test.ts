import { describe, expect, it } from "vitest";

import { createStandardPortalEnv } from "./env";

describe("unified Portal environment", () => {
  it("uses one API base URL without service or SSO configuration", () => {
    const env = createStandardPortalEnv({
      env: { VITE_APP_VERSION: "54135ad84974" }
    });

    expect(env.apiBaseUrl).toBe("/");
    expect(env.appVersion).toBe("54135ad84974");
    expect(env).not.toHaveProperty("publicBaseUrl");
    expect(env).not.toHaveProperty("legalBaseUrl");
    expect(env).not.toHaveProperty("serviceClientIds");
    expect(env).not.toHaveProperty("xClientId");
    expect(env).not.toHaveProperty("ssoAuthorizeUrl");
  });
});
