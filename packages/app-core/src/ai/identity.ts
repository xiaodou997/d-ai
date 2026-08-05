import type { components } from "@dai/api-client/ai";

export type IdentityIncludedUser = components["schemas"]["IdentityIncludedUserDTO"];
export type IdentityIncludedTenant = components["schemas"]["IdentityIncludedTenantDTO"];
export type IdentityIncluded = components["schemas"]["IdentityIncludedDTO"];

export const EMPTY_IDENTITY_INCLUDED: IdentityIncluded = Object.freeze({
  users: Object.freeze({}),
  tenants: Object.freeze({})
});

export function normalizeIdentityIncluded(value: unknown): IdentityIncluded {
  if (!value || typeof value !== "object") return EMPTY_IDENTITY_INCLUDED;
  const candidate = value as { users?: unknown; tenants?: unknown };
  return {
    users: normalizeRecord<IdentityIncludedUser>(candidate.users),
    tenants: normalizeRecord<IdentityIncludedTenant>(candidate.tenants)
  };
}

export function resolveIdentityTenantLabel(
  tenantId: string | null | undefined,
  included?: IdentityIncluded | null
): string {
  if (!tenantId) return "—";
  return included?.tenants?.[tenantId]?.tenant_name || tenantId;
}

export function resolveIdentityTenantMeta(
  tenantId: string | null | undefined,
  included?: IdentityIncluded | null
): string {
  if (!tenantId) return "";
  return resolveIdentityTenantLabel(tenantId, included) !== tenantId ? tenantId : "";
}

export function resolveIdentityUserLabel(
  userId: string | null | undefined,
  included?: IdentityIncluded | null
): string {
  if (!userId) return "—";
  const user = included?.users?.[userId];
  return user?.nickname || user?.username || user?.email || userId;
}

export function resolveIdentityUserMeta(
  userId: string | null | undefined,
  included?: IdentityIncluded | null
): string {
  if (!userId) return "";
  return resolveIdentityUserLabel(userId, included) !== userId ? userId : "";
}

function normalizeRecord<T extends object>(value: unknown): Record<string, T> {
  if (!value || typeof value !== "object" || Array.isArray(value)) return {};
  return Object.fromEntries(
    Object.entries(value).filter((entry): entry is [string, T] => Boolean(entry[1]) && typeof entry[1] === "object")
  );
}
