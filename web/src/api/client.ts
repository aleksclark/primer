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
