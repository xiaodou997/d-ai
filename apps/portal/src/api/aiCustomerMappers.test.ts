import { describe, expect, it } from "vitest";

import type { components } from "./generated/dai";
import {
  toApiKeys,
  toChatModels,
  toChatSessions,
  toCurrentSubscription
} from "./aiCustomerMappers";

const apiKey = {
  id: "key-1",
  owner_type: "user",
  tenant_id: "tenant-1",
  group_id: "group-1",
  name: "Personal key",
  quota_used_micro_usd: 0,
  status: "active"
} satisfies components["schemas"]["ApiKeyDTO"];

describe("AI customer generated DTO mappers", () => {
  it("normalizes nullable API key collections and optional limits", () => {
    const result = toApiKeys({ items: [apiKey], total: 1 });

    expect(result.items).toEqual([
      expect.objectContaining({
        id: "key-1",
        quota_limit_micro_usd: undefined,
        limit_policy: undefined
      })
    ]);
    expect(toApiKeys({ items: null, total: 0 })).toEqual({ items: [], total: 0 });
  });

  it("normalizes nullable workspace lists and model formats", () => {
    expect(toChatModels({ items: null, total: 0 })).toEqual({ items: [], total: 0 });
    expect(
      toChatModels({
        items: [
          {
            group_id: "group-1",
            group_name: "Default",
            effective_user_multiplier: 1,
            billing_group_label: "standard",
            model_code: "gpt-test",
            capability_type: "chat",
            default_api_format: "openai",
            available_api_formats: null,
            supports_stream: true,
            status: "active"
          }
        ],
        total: 1
      })
    ).toMatchObject({ items: [{ model_code: "gpt-test", available_api_formats: [] }] });

    expect(toChatSessions({ items: null, total: 0 })).toEqual({ items: [], total: 0 });
  });

  it("keeps an absent current subscription as null", () => {
    expect(toCurrentSubscription(null)).toBeNull();
  });
});
