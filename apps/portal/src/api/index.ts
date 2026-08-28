export interface ProblemDetail {
  status: number;
  title: string;
  detail?: string;
  code?: string;
  meta?: Record<string, unknown>;
  errors?: Array<{
    field?: string;
    message?: string;
  }> | null;
}

export interface RequestAdapter {
  <T>(input: {
    method: string;
    path: string;
    query?: Record<string, string | number | boolean | undefined | null>;
    headers?: Record<string, string | undefined>;
    body?: unknown;
    baseUrl?: string;
    signal?: AbortSignal;
  }): Promise<T>;
}

import type { operations } from "./generated/dai";

type JSONResponseContent<Response> = Response extends { content: infer Content }
  ? Content extends { "application/json": infer Body }
    ? Body
    : never
  : never;

/** JSON success body inferred from one generated OpenAPI operation. */
export type OperationResponse<Operation extends keyof operations> =
  200 extends keyof operations[Operation]["responses"]
    ? JSONResponseContent<operations[Operation]["responses"][200]>
    : 201 extends keyof operations[Operation]["responses"]
      ? JSONResponseContent<operations[Operation]["responses"][201]>
      : 204 extends keyof operations[Operation]["responses"]
        ? undefined
        : never;

/** JSON request body inferred from one generated OpenAPI operation. */
export type OperationBody<Operation extends keyof operations> = operations[Operation] extends {
  requestBody: { content: { "application/json": infer Body } };
}
  ? Body
  : never;

export type OperationRequest<Operation extends keyof operations> = {
  method: string;
  path: string;
  query?: Record<string, string | number | boolean | undefined | null>;
  headers?: Record<string, string | undefined>;
  baseUrl?: string;
  signal?: AbortSignal;
} & (OperationBody<Operation> extends never ? { body?: never } : { body?: OperationBody<Operation> });

/**
 * Binds the runtime fetch adapter to generated operation response/body types.
 * The operation ID is compile-time only; path construction remains explicit so
 * callers can preserve existing URL encoding and facade-level view models.
 */
export function createTypedOperationRequest(adapter: RequestAdapter) {
  return function request<Operation extends keyof operations>(input: OperationRequest<Operation>) {
    return adapter<OperationResponse<Operation>>(input);
  };
}

export function joinUrl(baseUrl: string, path: string): string {
  const prefix = baseUrl.endsWith("/") ? baseUrl.slice(0, -1) : baseUrl;
  const suffix = path.startsWith("/") ? path : `/${path}`;
  return `${prefix}${suffix}`;
}

export function withQuery(
  path: string,
  query?: Record<string, string | number | boolean | undefined | null>
): string {
  if (!query) return path;

  const params = new URLSearchParams();
  for (const [key, value] of Object.entries(query)) {
    if (value === undefined || value === null || value === "") continue;
    params.set(key, String(value));
  }
  const search = params.toString();
  return search ? `${path}?${search}` : path;
}
