export interface PortalChatModelRecord {
  group_id: string;
  group_name?: string;
  effective_user_multiplier?: number;
  billing_group_label?: string;
  model_code: string;
  capability_type: string;
  default_api_format: string;
  available_api_formats: string[];
  supports_stream: boolean;
  status: string;
}

export interface PortalChatSessionRecord {
  id: string;
  title: string;
  model_code: string;
  group_id?: string;
  group_name?: string;
  effective_user_multiplier?: number;
  billing_group_label?: string;
  provider_api_format: string;
  selected_route_id: string;
  status: string;
  created_at: number;
  updated_at: number;
}

export interface PortalChatMessageRecord {
  id: string;
  role: string;
  content: string;
  protocol?: string;
  route_id?: string;
  usage?: Record<string, unknown>;
  error?: Record<string, unknown>;
  created_at: number;
}

export interface PortalChatUiMessage {
  id: string;
  role: string;
  content: string;
  protocol?: string;
  streamStatus?: string;
}

export interface PortalChatSessionDetail<
  TSession extends PortalChatSessionRecord = PortalChatSessionRecord,
  TMessage extends PortalChatMessageRecord = PortalChatMessageRecord
> {
  session: TSession;
  messages: TMessage[];
}

export interface PortalChatCreateSessionInput {
  model_code: string;
  group_id: string;
  title?: string;
}

export interface PortalChatStreamInput {
  sessionId: string;
  model?: string;
  messages: Array<{ role: string; content: string }>;
  signal?: AbortSignal;
  onDelta: (delta: string) => void;
  onEvent?: (eventType: string) => void;
}

export interface PortalChatApi<
  TModel extends PortalChatModelRecord = PortalChatModelRecord,
  TSession extends PortalChatSessionRecord = PortalChatSessionRecord,
  TMessage extends PortalChatMessageRecord = PortalChatMessageRecord
> {
  listModels: () => Promise<TModel[]>;
  listSessions: () => Promise<TSession[]>;
  createSession: (payload: PortalChatCreateSessionInput) => Promise<TSession>;
  getSession: (sessionId: string) => Promise<PortalChatSessionDetail<TSession, TMessage>>;
  deleteSession: (sessionId: string) => Promise<unknown>;
  streamMessage: (payload: PortalChatStreamInput) => Promise<void>;
}
