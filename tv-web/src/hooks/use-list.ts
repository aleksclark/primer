import { useCallback, useEffect, useState } from "react";
import { get, listQueryString, type ListQuery, type Page } from "@/api/client";

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

    setState((s) => ({ ...s, loading: true }));
    get<Page<T>>(`${path}?${listQueryString(query)}`, controller.signal)
      .then((data) => setState({ data, loading: false, error: null }))
      .catch((err: Error) => {
        if (err.name !== "AbortError") {
          setState({ data: null, loading: false, error: err.message });
        }
      });
    return () => controller.abort();
  }, [path, query.limit, query.offset, query.q, query.sort, query.dir, JSON.stringify(query.filter), tick]);

  return { ...state, refresh };
}
