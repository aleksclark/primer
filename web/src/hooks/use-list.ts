import { useCallback, useEffect, useState } from "react";
import type { ListQuery, Page } from "@/api/client";

interface ListState<T> {
  data: Page<T> | null;
  loading: boolean;
  error: string | null;
}

/**
 * useList drives a paginated, searchable, sortable list endpoint. All list
 * endpoints share the same query contract, so one hook serves every resource.
 */
export function useList<T>(
  path: string,
  query: ListQuery,
): ListState<T> & { refresh: () => void } {
  const [state, setState] = useState<ListState<T>>({ data: null, loading: true, error: null });
  const [tick, setTick] = useState(0);

  const refresh = useCallback(() => setTick((t) => t + 1), []);

  useEffect(() => {
    const controller = new AbortController();
    const params = new URLSearchParams();
    if (query.limit != null) params.set("limit", String(query.limit));
    if (query.offset != null) params.set("offset", String(query.offset));
    if (query.q) params.set("q", query.q);
    if (query.sort) params.set("sort", query.sort);
    if (query.dir) params.set("dir", query.dir);
    for (const f of query.filter ?? []) params.append("filter", f);

    setState((s) => ({ ...s, loading: true }));
    fetch(`/api/v1${path}?${params}`, { signal: controller.signal })
      .then(async (res) => {
        if (!res.ok) {
          const body = await res.json().catch(() => ({}));
          throw new Error(body.detail ?? `HTTP ${res.status}`);
        }
        return res.json();
      })
      .then((data: Page<T>) => setState({ data, loading: false, error: null }))
      .catch((err: Error) => {
        if (err.name !== "AbortError") {
          setState({ data: null, loading: false, error: err.message });
        }
      });
    return () => controller.abort();
  }, [path, query.limit, query.offset, query.q, query.sort, query.dir, JSON.stringify(query.filter), tick]);

  return { ...state, refresh };
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
  if (!res.ok) {
    const detail = await res.json().catch(() => ({}));
    throw new Error(detail.detail ?? `HTTP ${res.status}`);
  }
  if (res.status === 204) return null;
  return res.json();
}
