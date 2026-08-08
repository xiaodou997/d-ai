export interface PortalApiKeyLimitPolicy {
  id?: string;
  scope_type?: string;
  scope_id?: string;
  concurrency_limit?: number | null;
  status?: "active" | "disabled" | null;
}

export interface PortalApiKeyRecord {
  id: string;
  owner_type: string;
  tenant_id: string;
  user_id?: string | null;
  group_id: string;
  last_four?: string | null;
  name: string;
  quota_limit_micro_usd?: number | null;
  quota_used_micro_usd: number;
  status: string;
  expires_at?: number | null;
  last_used_at?: number | null;
  limit_policy?: PortalApiKeyLimitPolicy | null;
  created_by?: string | null;
  created_at?: number | null;
  updated_at?: number | null;
}

export interface PortalApiKeyGroupRecord {
  id: string;
  name: string;
  effective_user_multiplier?: number;
  status?: string;
}

export interface PortalApiKeyWriteInput {
  name: string;
  group_id: string;
  quota_limit_micro_usd?: number | null;
  status?: string;
  expires_at?: number | null;
  limit_policy?: PortalApiKeyLimitPolicy | null;
}

export interface PortalApiKeyCreatedResult<TKey extends PortalApiKeyRecord = PortalApiKeyRecord> {
  plaintext_key: string;
  key?: TKey;
}

export interface PortalApiKeyRevealResult {
  plaintext_key: string;
}

export interface PortalApiKeyApi<TKey extends PortalApiKeyRecord = PortalApiKeyRecord> {
  listApiKeys: () => Promise<{ items: TKey[]; total?: number }>;
  createApiKey: (payload: PortalApiKeyWriteInput) => Promise<PortalApiKeyCreatedResult<TKey>>;
  updateApiKey: (apiKeyId: string, payload: PortalApiKeyWriteInput) => Promise<TKey>;
  updateApiKeyStatus: (apiKeyId: string, status: string) => Promise<TKey>;
  revealApiKey: (apiKeyId: string) => Promise<PortalApiKeyRevealResult>;
  rotateApiKey: (apiKeyId: string) => Promise<PortalApiKeyCreatedResult<TKey>>;
  deleteApiKey: (apiKeyId: string) => Promise<unknown>;
  listGroups?: () => Promise<{ items: PortalApiKeyGroupRecord[]; total?: number }>;
}
