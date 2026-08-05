import createClient from "openapi-fetch";
import type { paths } from "./schema";
import { authHeaders, reportUnauthorized } from "./auth";

/**
 * Typed API client generated at build time from the server's OpenAPI spec.
 * Run `npm run generate:client` (or `make client`) after changing the API.
 */
export const api = createClient<paths>({ baseUrl: "/api/v1" });

/** Standard paginated list envelope returned by every list endpoint. */
export interface Page<T> {
  items: T[];
  totalCount: number;
  limit: number;
  offset: number;
}

/** Standard list query parameters supported by every list endpoint. */
export interface ListQuery {
  limit?: number;
  offset?: number;
  q?: string;
  sort?: string;
  dir?: "asc" | "desc";
  filter?: string[];
}

/** Extract a human-readable error detail from a failed API response. */
export async function apiError(res: Response): Promise<Error> {
  if (res.status === 401) reportUnauthorized();
  const body = await res.json().catch(() => ({}));
  return new Error((body as { detail?: string }).detail ?? `HTTP ${res.status}`);
}

function mergeHeaders(extra?: HeadersInit): Headers {
  const h = new Headers(extra);
  for (const [k, v] of Object.entries(authHeaders())) {
    if (!h.has(k)) h.set(k, v);
  }
  return h;
}

/** get performs an authenticated JSON GET. */
export async function get<T>(path: string): Promise<T> {
  const res = await fetch(`/api/v1${path}`, {
    headers: mergeHeaders(),
  });
  if (!res.ok) throw await apiError(res);
  return res.json() as Promise<T>;
}

/** mutate performs a JSON write request against the API. */
export async function mutate(
  method: "POST" | "PATCH" | "DELETE",
  path: string,
  body?: unknown,
): Promise<unknown> {
  const headers = mergeHeaders(body ? { "Content-Type": "application/json" } : undefined);
  const res = await fetch(`/api/v1${path}`, {
    method,
    headers,
    body: body ? JSON.stringify(body) : undefined,
  });
  if (!res.ok) throw await apiError(res);
  if (res.status === 204) return null;
  return res.json();
}
