export interface AdminTenantLimitForm {
  concurrency_limit: number | null;
  status: "active" | "disabled";
}

export interface AdminTenantUpstreamPolicyDraft {
  access_granted: boolean;
  tenant_multiplier_override: number | null;
}
