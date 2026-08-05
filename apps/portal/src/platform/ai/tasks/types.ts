export type PortalTaskStatus = "pending" | "running" | "completed" | "failed" | "cancelled";
export type PortalTaskType = "images.generation" | "images.edit" | "chat.completions";
export type PortalTaskSource = "portal" | "api_key" | "app_key";
export type PortalTaskOwnerScope = "tenant" | "user";

export interface PortalTaskOwner {
  scope: PortalTaskOwnerScope;
  user_id?: string;
}

export interface PortalTaskPermissions {
  read_only: boolean;
  can_cancel: boolean;
  can_delete: boolean;
}

export interface PortalTaskRecord {
  id: string;
  type: PortalTaskType;
  source: PortalTaskSource;
  status: PortalTaskStatus;
  model?: string;
  owner: PortalTaskOwner;
  permissions: PortalTaskPermissions;
  request_id?: string;
  attempt: number;
  error?: { code: string; message: string };
  usage?: { cost_credits: number };
  result_available: boolean;
  result_summary?: { image_count?: number; choice_count?: number };
  result?: unknown;
  created_at: number;
  started_at?: number;
  completed_at?: number;
}

export interface PortalTaskPage {
  items: PortalTaskRecord[];
  has_more: boolean;
}

export interface PortalTaskQuery {
  owner_scope?: "" | PortalTaskOwnerScope;
  user_id?: string;
  status?: "" | PortalTaskStatus;
  type?: "" | PortalTaskType;
  limit?: number;
  starting_after?: string;
}

export interface PortalTaskApi {
  listTasks: (query: PortalTaskQuery) => Promise<PortalTaskPage>;
  getTask: (taskId: string) => Promise<PortalTaskRecord>;
  cancelTask: (taskId: string) => Promise<PortalTaskRecord>;
  deleteTask: (taskId: string) => Promise<{ deleted: boolean }>;
}

export type PortalTaskPortalMode = "tenant" | "user";
