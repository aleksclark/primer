import { useMemo } from "react";
import type { components } from "@/api/schema";
import { useList } from "@/hooks/use-list";
import type { Choice } from "@/components/resource-page";

type MediaItem = components["schemas"]["MediaItem"];

/** MEDIA_ITEM_LIMIT is the server's maximum page size for a list endpoint. */
const MEDIA_ITEM_LIMIT = 200;

/**
 * useMediaItems loads the library for the pickers and title lookups that the
 * availability and schedule pages need. Those pages join by media item ID, and
 * a UUID is useless to a parent, so every reference is resolved to a title.
 */
export function useMediaItems() {
  const query = useMemo(
    () => ({ limit: MEDIA_ITEM_LIMIT, sort: "title", dir: "asc" as const }),
    [],
  );
  const { data } = useList<MediaItem>("/media-items", query);

  return useMemo(() => {
    const items = data?.items ?? [];
    const byId = new Map(items.map((item) => [item.id, item]));
    const choices: Choice[] = items.map((item) => ({ value: item.id, label: item.title }));
    const title = (id?: string | null) => (id ? byId.get(id)?.title ?? id.slice(0, 8) : "");
    return { items, byId, choices, title };
  }, [data]);
}
