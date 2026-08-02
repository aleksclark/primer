import { useCallback, useEffect, useState } from "react";
import { get } from "@/api/client";
import type { components } from "@/api/schema";

export type Programme = components["schemas"]["Programme"];
type GridResponse = components["schemas"]["GridResponse"];

interface GridState {
  data: GridResponse | null;
  loading: boolean;
  error: string | null;
}

/**
 * useScheduleGrid loads a span of the programmed grid.
 *
 * The grid is not a paginated resource list: the server resolves each entry
 * against its media item's runtime to work out when the slot ends, which is
 * what makes overlap visible. So this reads `/schedule-grid` rather than
 * driving `ResourcePage` over `/schedule-entries`.
 */
export function useScheduleGrid(from: string, days: number): GridState & { refresh: () => void } {
  const [state, setState] = useState<GridState>({ data: null, loading: true, error: null });
  const [tick, setTick] = useState(0);

  const refresh = useCallback(() => setTick((t) => t + 1), []);

  useEffect(() => {
    const controller = new AbortController();
    setState((s) => ({ ...s, loading: true }));
    get<GridResponse>(`/schedule-grid?from=${from}&days=${days}`, controller.signal)
      .then((data) => setState({ data, loading: false, error: null }))
      .catch((err: Error) => {
        if (err.name !== "AbortError") {
          setState({ data: null, loading: false, error: err.message });
        }
      });
    return () => controller.abort();
  }, [from, days, tick]);

  return { ...state, refresh };
}
