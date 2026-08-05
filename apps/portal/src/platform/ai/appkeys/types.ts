export type AppCapability = "chat" | "image_generation" | "image_edit";

// 使用侧脱敏视图:服务端不再返回应用底层模型。
export interface PortalVisibleAgentRecord {
  id: string;
  name: string;
  description?: string;
	capability: AppCapability;
	prompt_strategy: "none" | "caller_variables" | "bound_prompt_exact";
  publisher_label?: string;
  variables: string[];
	prompt_names: string[];
}

// 应用密钥：每把密钥只能绑定一个已发布应用（最小权限），不支持直绑裸模型。
export interface PortalAppKeyRecord {
  id: string;
  name: string;
  last_four: string;
  status: "active" | "disabled";
  agent_id: string;
  agent_name?: string;
  expires_at?: number;
}

export interface PortalAppKeyListOutput<TAppKey extends PortalAppKeyRecord = PortalAppKeyRecord> {
  items: TAppKey[];
  total: number;
}

export interface PortalVisibleAgentListOutput<TAgent extends PortalVisibleAgentRecord = PortalVisibleAgentRecord> {
  items: TAgent[];
  total: number;
}

export interface PortalAppKeyWriteInput {
  name: string;
  status?: "active" | "disabled";
  agent_id: string;
  expires_at?: number | null;
}

export interface PortalAppKeyCreatedOutput<TAppKey extends PortalAppKeyRecord = PortalAppKeyRecord> {
  plaintext_key: string;
  key: TAppKey;
}

export interface PortalAppKeyRevealOutput {
  plaintext_key: string;
}

export interface PortalAppKeyApi<
  TAppKey extends PortalAppKeyRecord = PortalAppKeyRecord,
  TAgent extends PortalVisibleAgentRecord = PortalVisibleAgentRecord
> {
  listAppKeys: () => Promise<PortalAppKeyListOutput<TAppKey>>;
  listVisibleAgents: () => Promise<PortalVisibleAgentListOutput<TAgent>>;
  createAppKey: (payload: PortalAppKeyWriteInput) => Promise<PortalAppKeyCreatedOutput<TAppKey>>;
  updateAppKey: (appKeyId: string, payload: Partial<PortalAppKeyWriteInput>) => Promise<TAppKey>;
  deleteAppKey: (appKeyId: string) => Promise<{ deleted: boolean }>;
  revealAppKey: (appKeyId: string) => Promise<PortalAppKeyRevealOutput>;
  rotateAppKey: (appKeyId: string) => Promise<PortalAppKeyCreatedOutput<TAppKey>>;
}
