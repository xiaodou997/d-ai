import { describe, expect, it } from "vitest";

import {
  EMPTY_IDENTITY_INCLUDED,
  normalizeIdentityIncluded,
  resolveIdentityTenantLabel,
  resolveIdentityTenantMeta,
  resolveIdentityUserLabel,
  resolveIdentityUserMeta
} from "../identity";

describe("portal identity enrichment", () => {
  const included = normalizeIdentityIncluded({
    users: {
      nickname: { user_id: "u-1", tenant_id: "t-1", username: "alice", nickname: "Alice" },
      username: { user_id: "u-2", tenant_id: "t-1", username: "bob", email: "bob@example.com" },
      email: { user_id: "u-3", tenant_id: "t-1", username: "", email: "only@example.com" }
    },
    tenants: {
      "t-1": { tenant_id: "t-1", tenant_name: "North Campus" }
    }
  });

  it("uses nickname, username, email, then ID priority", () => {
    expect(resolveIdentityUserLabel("nickname", included)).toBe("Alice");
    expect(resolveIdentityUserLabel("username", included)).toBe("bob");
    expect(resolveIdentityUserLabel("email", included)).toBe("only@example.com");
    expect(resolveIdentityUserLabel("unknown", included)).toBe("unknown");
  });

  it("keeps IDs as metadata only when enrichment succeeds", () => {
    expect(resolveIdentityUserMeta("nickname", included)).toBe("nickname");
    expect(resolveIdentityUserMeta("unknown", included)).toBe("");
    expect(resolveIdentityTenantLabel("t-1", included)).toBe("North Campus");
    expect(resolveIdentityTenantMeta("t-1", included)).toBe("t-1");
  });

  it("normalizes missing and partially enriched responses without blocking", () => {
    expect(normalizeIdentityIncluded(null)).toBe(EMPTY_IDENTITY_INCLUDED);
    expect(normalizeIdentityIncluded({ users: { "u-1": included.users.nickname } })).toEqual({
      users: { "u-1": included.users.nickname },
      tenants: {}
    });
    expect(resolveIdentityUserLabel("external-user", normalizeIdentityIncluded({}))).toBe("external-user");
  });
});
