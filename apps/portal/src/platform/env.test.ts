import { describe, expect, it } from "vitest";

import { createStandardPortalEnv } from "./env";

describe("unified Portal environment", () => {
  it("uses one API base URL without service or SSO configuration", () => {
    const env = createStandardPortalEnv({
      env: { VITE_API_BASE_URL: "http://dai.test", VITE_PUBLIC_BASE_URL: "http://public.test" }
    });

    expect(env.apiBaseUrl).toBe("http://dai.test");
    expect(env.publicBaseUrl).toBe("http://public.test");
    expect(env).not.toHaveProperty("serviceClientIds");
    expect(env).not.toHaveProperty("xClientId");
    expect(env).not.toHaveProperty("ssoAuthorizeUrl");
  });
});
