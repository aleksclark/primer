import createClient from "openapi-fetch";
import type { paths } from "./schema";

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
  const body = await res.json().catch(() => ({}));
  return new Error((body as { detail?: string }).detail ?? `HTTP ${res.status}`);
}

/** mutate performs a JSON write request against the API. */
export async function mutate(
  method: "POST" | "PATCH" | "DELETE",
  path: string,
  body?: unknown,
): Promise<unknown> {
  const res = await fetch(`/api/v1${path}`, {
    method,
    headers: body ? { "Content-Type": "application/json" } : undefined,
    body: body ? JSON.stringify(body) : undefined,
  });
  if (!res.ok) throw await apiError(res);
  if (res.status === 204) return null;
  return res.json();
}
