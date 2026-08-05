export type PortalAppScope = "tenant" | "user";
export type PortalAppType = "chat" | "image_generation" | "image_edit";
export type PortalPromptStrategy = "none" | "caller_variables" | "bound_prompt_exact";
export type PortalAppTemplateId =
  | "standard_chat"
  | "keyword_selector"
  | "dynamic_prompt_composition"
  | "text_to_image"
  | "image_to_image";

export interface PortalAppRuntimeConfig {
  chat?: {
    creativity: "precise" | "balanced" | "creative";
    allow_attachments: boolean;
  };
  image?: {
    resolution: string;
    aspect_ratio: string;
    default_output_count: number;
    max_output_count: number;
    allow_output_count_override: boolean;
  };
}

export interface PortalAppPromptRecord {
  owner_type: "tenant" | "user";
  owner_tenant_id?: string;
  owner_user_id?: string;
  id: string;
  name: string;
  description: string;
  status: "active" | "disabled";
  template_text: string;
  variables: string[];
  created_by?: string;
  updated_by?: string;
  created_at?: number;
  updated_at?: number;
}

export interface PortalAppPromptDetailRecord<TPrompt extends PortalAppPromptRecord = PortalAppPromptRecord> {
  prompt: TPrompt;
}

export interface PortalAppPromptWriteInput {
  name?: string;
  description?: string;
  status?: "active" | "disabled";
  template_text?: string;
}

export interface PortalAppPromptBindingRecord {
  prompt_id: string;
  prompt_name: string;
  variables: string[];
  binding_role: "primary" | "fragment";
  display_order: number;
}

export interface PortalAppRecord {
  owner_type: "tenant" | "user";
  owner_tenant_id?: string;
  owner_user_id?: string;
  id: string;
  name: string;
  description: string;
  status: "active" | "disabled";
  capability: PortalAppType;
  prompt_strategy: PortalPromptStrategy;
  prompt_bindings: PortalAppPromptBindingRecord[];
  group_id: string;
  model_code: string;
  runtime_config: PortalAppRuntimeConfig;
  published_by_tenant?: boolean;
  created_by?: string;
  updated_by?: string;
  created_at?: number;
  updated_at?: number;
}

export interface PortalAppTemplate {
  id: PortalAppTemplateId;
  name: string;
  description: string;
  defaultCapability: PortalAppType;
  allowedCapabilities: PortalAppType[];
  promptStrategy: PortalPromptStrategy;
  minPromptBindings: number;
  maxPromptBindings: number;
}

export interface PortalAppTemplateRecord {
  id: PortalAppTemplateId;
  name: string;
  description: string;
  default_capability: PortalAppType;
  allowed_capabilities: PortalAppType[];
  prompt_strategy: PortalPromptStrategy;
  min_prompt_bindings: number;
  max_prompt_bindings: number;
}

export interface PortalAppModelRecord {
  group_id?: string;
  group_name?: string;
  effective_user_multiplier?: number;
  billing_group_label?: string;
  model_code: string;
  capability_type: string;
  status?: string;
  max_output_count?: number;
  edit_max_output_count?: number;
}

export interface PortalAppWriteInput {
  template_id?: PortalAppTemplateId;
  name: string;
  description: string;
  status: "active" | "disabled";
  capability: PortalAppType;
  prompt_strategy: PortalPromptStrategy;
  prompt_ids: string[];
  group_id: string;
  model_code: string;
  runtime_config: PortalAppRuntimeConfig;
}

export interface PortalAppPreviewAttachment {
  type?: "image" | "file";
  url: string;
  name?: string;
  mime_type?: string;
}

export interface PortalAppPreviewRequest {
  input?: string;
  variables?: Record<string, string>;
  attachments?: PortalAppPreviewAttachment[];
  images?: Array<string | PortalAppPreviewAttachment>;
  response_format?: "url" | "b64_json";
  n?: number;
}

export interface PortalAppPreviewImageResult {
  url?: string;
  b64_json?: string;
}

export interface PortalAppPreviewResponse {
  type: "chat" | "image";
  text?: string;
  images?: PortalAppPreviewImageResult[];
  usage?: Record<string, unknown>;
  request_id?: string;
}

export interface PortalAppPromptApi<
  TPrompt extends PortalAppPromptRecord = PortalAppPromptRecord,
  TDetail extends PortalAppPromptDetailRecord<TPrompt> = PortalAppPromptDetailRecord<TPrompt>
> {
  listPrompts: () => Promise<{ items: TPrompt[]; total?: number }>;
  getPrompt: (promptId: string) => Promise<TDetail>;
  createPrompt: (payload: Required<Pick<PortalAppPromptWriteInput, "name" | "template_text">> & PortalAppPromptWriteInput) => Promise<TDetail>;
  updatePrompt: (promptId: string, payload: PortalAppPromptWriteInput) => Promise<TDetail>;
  deletePrompt: (promptId: string) => Promise<unknown>;
}

export interface PortalAppApi<
  TPrompt extends PortalAppPromptRecord = PortalAppPromptRecord,
  TDetail extends PortalAppPromptDetailRecord<TPrompt> = PortalAppPromptDetailRecord<TPrompt>,
  TApp extends PortalAppRecord = PortalAppRecord,
  TModel extends PortalAppModelRecord = PortalAppModelRecord
> {
  listTemplates: () => Promise<{ items: PortalAppTemplateRecord[] }>;
  listApps: () => Promise<{ items: TApp[]; total?: number }>;
  listPrompts: () => Promise<{ items: TPrompt[]; total?: number }>;
  listModels?: () => Promise<TModel[]>;
  getPrompt?: (promptId: string) => Promise<TDetail>;
  createApp: (payload: PortalAppWriteInput) => Promise<TApp>;
  updateApp: (appId: string, payload: PortalAppWriteInput) => Promise<TApp>;
  deleteApp: (appId: string) => Promise<unknown>;
  setPublication?: (appId: string, published: boolean) => Promise<unknown>;
  previewApp?: (appId: string, payload: PortalAppPreviewRequest) => Promise<PortalAppPreviewResponse>;
}
