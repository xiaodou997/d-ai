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

export interface ServiceRuntime {
  service: "urm" | "ai";
  baseUrl: string;
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
