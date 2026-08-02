import createClient from "openapi-fetch";
import type { paths } from "./schema";
import { authHeaders, reportUnauthorized } from "./auth";

/**
 * Typed API client generated at build time from the TV server's OpenAPI spec.
 * Run `npm run generate:client` (or `make tv-client`) after changing the API.
 *
 * The middleware attaches the admin key to every request and notices when the
 * server rejects it, so a stale key surfaces as the key-entry screen rather
 * than as a per-page error.
 */
export const api = createClient<paths>({ baseUrl: "/api/v1" });

api.use({
  onRequest({ request }) {
    for (const [name, value] of Object.entries(authHeaders())) {
      request.headers.set(name, value);
    }
    return request;
  },
  onResponse({ response }) {
    if (response.status === 401) reportUnauthorized();
    return response;
  },
});

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

/** mutate performs a JSON write request against the API. */
export async function mutate(
  method: "POST" | "PATCH" | "DELETE",
  path: string,
  body?: unknown,
): Promise<unknown> {
  const res = await fetch(`/api/v1${path}`, {
    method,
    headers: {
      ...authHeaders(),
      ...(body ? { "Content-Type": "application/json" } : {}),
    },
    body: body ? JSON.stringify(body) : undefined,
  });
  if (!res.ok) throw await apiError(res);
  if (res.status === 204) return null;
  return res.json();
}

/** get performs an authenticated JSON read against the API. */
export async function get<T>(path: string, signal?: AbortSignal): Promise<T> {
  const res = await fetch(`/api/v1${path}`, { headers: authHeaders(), signal });
  if (!res.ok) throw await apiError(res);
  return res.json() as Promise<T>;
}

/** listQueryString renders a ListQuery as URL search parameters. */
export function listQueryString(query: ListQuery): string {
  const params = new URLSearchParams();
  if (query.limit != null) params.set("limit", String(query.limit));
  if (query.offset != null) params.set("offset", String(query.offset));
  if (query.q) params.set("q", query.q);
  if (query.sort) params.set("sort", query.sort);
  if (query.dir) params.set("dir", query.dir);
  for (const f of query.filter ?? []) params.append("filter", f);
  return params.toString();
}

/** imageURL is the TV-server-proxied artwork URL for a media item. */
export function imageURL(mediaItemId: string, type = "Primary"): string {
  return `/api/v1/images/${mediaItemId}/${type}`;
}
