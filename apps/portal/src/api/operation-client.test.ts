import { describe, expect, it } from "vitest";

import { createTypedOperationRequest, type RequestAdapter } from ".";
import type { PublicRegistrationPayload } from "./types/platformPublic";
import type { LiteLLMModelsOutputBody } from "./types/ai";

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

  it("keeps generated request bodies assignable to the facade domain alias", () => {
    const payload: PublicRegistrationPayload = {
      username: "demo",
      password: "Strong-password-123",
      termsVersion: "2026-07-19",
      privacyVersion: "2026-07-19"
    };
    expect(payload.email).toBeUndefined();
  });

  it("exposes generated LiteLLM response fields without any", () => {
    const result: LiteLLMModelsOutputBody = { items: [], total: 0 };
    expect(result.items).toEqual([]);
  });
});
