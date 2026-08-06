import type { PortalChatStreamInput } from "./chat/types";
import { appendPortalQuery } from "./utils";
import type { RequestRecoveryResult } from "../http";
import { joinUrl } from "@/api";

export interface CreatePortalRuntimeTransportOptions {
  baseUrl: () => string;
  getAccessToken: () => string;
  onUnauthorized?: () => Promise<RequestRecoveryResult> | RequestRecoveryResult;
  runtimeBasePath?: string;
}

export interface PortalRuntimeTransport {
  request: <T>(
    method: string,
    path: string,
    body?: unknown,
    query?: Record<string, string | number | undefined>
  ) => Promise<T>;
  formRequest: <T>(method: string, path: string, body: FormData) => Promise<T>;
  streamChatMessage: (payload: PortalChatStreamInput) => Promise<void>;
}

export function createPortalRuntimeTransport(options: CreatePortalRuntimeTransportOptions): PortalRuntimeTransport {
  const runtimeBasePath = options.runtimeBasePath ?? "/runtime/v1";

  function runtimeHeaders(extra: Record<string, string> = {}): Record<string, string> {
    const token = options.getAccessToken();
    return {
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
      ...extra
    };
  }

  async function request<T>(
    method: string,
    path: string,
    body?: unknown,
    query?: Record<string, string | number | undefined>
  ): Promise<T> {
    const response = await executeWithRecovery(() =>
      fetch(joinUrl(options.baseUrl(), appendPortalQuery(path, query)), {
        method,
        headers: runtimeHeaders(body !== undefined ? { "Content-Type": "application/json" } : {}),
        body: body !== undefined ? JSON.stringify(body) : undefined
      })
    );
    if (!response) {
      return suspendForNavigation<T>();
    }
    return parseRuntimeEnvelope<T>(response);
  }

  async function formRequest<T>(method: string, path: string, body: FormData): Promise<T> {
    const response = await executeWithRecovery(() =>
      fetch(joinUrl(options.baseUrl(), path), {
        method,
        headers: runtimeHeaders(),
        body
      })
    );
    if (!response) {
      return suspendForNavigation<T>();
    }
    return parseRuntimeEnvelope<T>(response);
  }

  async function streamChatMessage(opts: PortalChatStreamInput): Promise<void> {
    const response = await executeWithRecovery(() =>
      fetch(
        joinUrl(
          options.baseUrl(),
          `${runtimeBasePath}/chat/sessions/${encodeURIComponent(opts.sessionId)}/messages:stream`
        ),
        {
          method: "POST",
          headers: runtimeHeaders({ "Content-Type": "application/json", Accept: "text/event-stream" }),
          body: JSON.stringify({
            model: opts.model || "",
            messages: opts.messages
          }),
          signal: opts.signal
        }
      )
    );
    if (!response) {
      return suspendForNavigation<void>();
    }

    if (!response.ok) {
      const text = await response.text().catch(() => "");
      let message = `请求失败 (${response.status})`;
      try {
        const data = JSON.parse(text);
        message = data?.error?.message || data?.message || text || message;
      } catch {
        if (text) message = text;
      }
      throw new Error(message);
    }
    if (!response.body) {
      throw new Error("当前浏览器不支持流式响应");
    }

    const reader = response.body.getReader();
    const decoder = new TextDecoder();
    let buffer = "";
    let eventType = "";

    for (;;) {
      const { value, done } = await reader.read();
      if (done) break;
      buffer += decoder.decode(value, { stream: true });
      const events = buffer.split("\n\n");
      buffer = events.pop() || "";

      for (const event of events) {
        const lines = event.split("\n");
        for (const line of lines) {
          if (line.startsWith("event:")) {
            eventType = line.slice(6).trim();
            opts.onEvent?.(eventType);
            continue;
          }
          if (!line.startsWith("data:")) continue;
          const data = line.slice(5).trim();
          if (!data || data === "[DONE]") continue;
          try {
            const parsed = JSON.parse(data);
            const delta = protocolDelta(parsed, eventType);
            if (delta) opts.onDelta(delta);
          } catch {
            // 忽略无法解析的事件帧
          }
        }
        eventType = "";
      }
    }
  }

  return {
    request,
    formRequest,
    streamChatMessage
  };

  async function executeWithRecovery(execute: () => Promise<Response>): Promise<Response | null> {
    let response = await execute();

    if (response.status === 401 && options.onUnauthorized) {
      const result = await options.onUnauthorized();
      if (result === "handled") {
        return null;
      }
      if (result === true || result === "retry") {
        response = await execute();
      }
    }

    return response;
  }
}

async function parseRuntimeEnvelope<T>(response: Response): Promise<T> {
  const text = await response.text();
  let parsed: unknown = null;
  try {
    parsed = text ? JSON.parse(text) : null;
  } catch {
    parsed = null;
  }
  if (!response.ok) {
    const message =
      (parsed as { message?: string; error?: { message?: string } } | null)?.message ||
      (parsed as { error?: { message?: string } } | null)?.error?.message ||
      `请求失败 (${response.status})`;
    throw new Error(message);
  }
  const envelope = parsed as { code?: number; data?: T } | null;
  if (envelope && typeof envelope === "object" && "data" in envelope) {
    return envelope.data as T;
  }
  return parsed as T;
}

function protocolDelta(payload: Record<string, unknown>, eventType: string): string {
  if (!payload || typeof payload !== "object") return "";
  const choice = (payload as { choices?: Array<{ delta?: { content?: string }; text?: string }> }).choices?.[0];
  if (choice?.delta?.content) return choice.delta.content;
  if (choice?.text) return choice.text;
  const delta = (payload as { delta?: unknown }).delta;
  if (typeof delta === "string") return delta;
  if (typeof (payload as { text?: unknown }).text === "string" && eventType?.includes("delta")) {
    return (payload as { text: string }).text;
  }
  if (delta && typeof delta === "object" && typeof (delta as { text?: unknown }).text === "string") {
    return (delta as { text: string }).text;
  }
  const parts = (payload as { candidates?: Array<{ content?: { parts?: Array<{ text?: string }> } }> }).candidates?.[0]
    ?.content?.parts;
  if (Array.isArray(parts)) return parts.map((part) => part.text || "").join("");
  return "";
}

function suspendForNavigation<T>(): Promise<T> {
  return new Promise<T>(() => undefined);
}

export { appendPortalQuery, portalStatusOptions, type PortalSelectOption } from "./utils";
