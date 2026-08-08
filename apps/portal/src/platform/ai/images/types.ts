export interface PortalImageModelRecord {
  group_id?: string;
  group_name?: string;
  effective_user_multiplier?: number;
  billing_group_label?: string;
  model_code: string;
  capability_type: string;
  status: string;
  max_output_count?: number;
  edit_max_output_count?: number;
}

export interface PortalImageJobRecord {
  id: string;
  operation?: "generation" | "edit";
  group_id?: string;
  model_code: string;
  prompt: string;
  retry_prompt?: string;
  status: string;
  storage_policy: string;
  raw_image_retained: boolean;
  size?: string;
  quality?: string;
  style?: string;
  response_format?: string;
  requested_output_count?: number;
  caller_charge_usd: number;
  image_count: number;
  inline_count: number;
  url_count: number;
  revised_prompts?: string[];
  assets?: PortalImageTaskAssetRecord[];
  error_message?: string;
  created_at: number;
  completed_at?: number;
}

export interface PortalImageTaskAssetRecord {
  id?: string;
  index?: number;
  preview_url?: string;
  display_url: string;
  original_url?: string;
  content_type?: string;
  size_bytes?: number;
  preview_content_type?: string;
  preview_size_bytes?: number;
  width?: number;
  height?: number;
  expires_at?: number;
}

export interface PortalImageTaskCreateResponse {
  task_id: string;
  status: string;
}

export interface PortalImageApi<
  TModel extends PortalImageModelRecord = PortalImageModelRecord,
  TJob extends PortalImageJobRecord = PortalImageJobRecord
> {
  listModels: () => Promise<TModel[]>;
  listJobs: () => Promise<TJob[]>;
  createTask: (payload: {
    operation?: "generation" | "edit";
    model: string;
    group_id?: string;
    prompt: string;
    n?: number;
    images?: Array<{ image_url: string }>;
    mask?: { image_url: string };
    size?: string;
    response_format?: string;
    background?: string;
    input_fidelity?: string;
    moderation?: string;
    output_format?: string;
    output_compression?: number;
    user?: string;
  } | FormData) => Promise<PortalImageTaskCreateResponse>;
  getTask: (taskId: string) => Promise<TJob>;
  cancelTask: (taskId: string) => Promise<TJob>;
  deleteTask: (taskId: string) => Promise<{ deleted: boolean }>;
}
