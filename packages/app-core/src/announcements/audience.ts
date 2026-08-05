import type {
  AnnouncementAudienceKind,
  AnnouncementAudienceRule,
  AnnouncementAudienceSelection
} from "./types";

export type AnnouncementAudiencePreset = "all" | "admins" | "tenant_users" | "end_users" | "selected";

export function audiencePresetToSelections(
  preset: AnnouncementAudiencePreset,
  tenantIds: readonly string[] = [],
  selectedKinds: readonly Exclude<AnnouncementAudienceKind, "admin">[] = []
): AnnouncementAudienceSelection[] {
  if (preset === "all") {
    return ["admin", "tenant_user", "end_user"].map((kind) => ({
      kind: kind as AnnouncementAudienceKind,
      scope: "all"
    }));
  }
  if (preset === "admins") return [{ kind: "admin", scope: "all" }];
  if (preset === "tenant_users") return [{ kind: "tenant_user", scope: "all" }];
  if (preset === "end_users") return [{ kind: "end_user", scope: "all" }];

  const uniqueTenantIds = [...new Set(tenantIds.filter(Boolean))];
  return selectedKinds.map((kind) => ({ kind, scope: "tenant", tenantIds: uniqueTenantIds }));
}

export function audienceRulesToEditorState(rules: readonly AnnouncementAudienceRule[]): {
  preset: AnnouncementAudiencePreset;
  tenantIds: string[];
  selectedKinds: Array<"tenant_user" | "end_user">;
} {
  const allKinds = new Set(rules.filter((rule) => rule.scope === "all").map((rule) => rule.kind));
  if (allKinds.size === 3) return { preset: "all", tenantIds: [], selectedKinds: [] };
  if (allKinds.size === 1 && allKinds.has("admin")) return { preset: "admins", tenantIds: [], selectedKinds: [] };
  if (allKinds.size === 1 && allKinds.has("tenant_user")) return { preset: "tenant_users", tenantIds: [], selectedKinds: [] };
  if (allKinds.size === 1 && allKinds.has("end_user")) return { preset: "end_users", tenantIds: [], selectedKinds: [] };

  return {
    preset: "selected",
    tenantIds: [...new Set(rules.map((rule) => rule.tenantId).filter((value): value is string => Boolean(value)))],
    selectedKinds: [...new Set(
      rules
        .map((rule) => rule.kind)
        .filter((kind): kind is "tenant_user" | "end_user" => kind === "tenant_user" || kind === "end_user")
    )]
  };
}
