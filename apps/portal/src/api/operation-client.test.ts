import { describe, expect, it } from "vitest";

import { createTypedOperationRequest, type RequestAdapter } from ".";

describe("typed OpenAPI operation request", () => {
  it("infers generated response and forwards the operation request", async () => {
    const adapter: RequestAdapter = async <T>() => ({
      minLength: 12,
      maxBytes: 128,
      requiredCharacterClasses: 3,
      description: "strong password"
    } as T);
    const request = createTypedOperationRequest(adapter);

    const response = await request<"auth-password-policy">({
      method: "GET",
      path: "/api/auth/password-policy",
      baseUrl: "https://portal.example.com"
    });

    expect(response.minLength).toBe(12);
  });
});
