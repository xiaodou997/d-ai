export interface AdminTenantLimitForm {
  concurrency_limit: number | null;
  status: "active" | "disabled";
}

export interface AdminTenantPolicySubject {
  tenantId: string;
  tenantName: string;
}

export interface AdminTenantUpstreamPolicyDraft {
  access_granted: boolean;
  tenant_multiplier_override: number | null;
}
