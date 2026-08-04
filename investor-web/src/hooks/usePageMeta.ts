import { useEffect } from "react";
import { useLocation } from "react-router-dom";
import {
  getCanonicalUrl,
  metaForPath,
  shouldNoIndex,
} from "@/lib/siteMeta";

function upsertMeta(attr: "name" | "property", key: string, content: string): void {
  const selector = `meta[${attr}="${key}"]`;
  let el = document.head.querySelector(selector) as HTMLMetaElement | null;
  if (!el) {
    el = document.createElement("meta");
    el.setAttribute(attr, key);
    document.head.appendChild(el);
  }
  el.content = content;
}

function upsertLink(rel: string, href: string): void {
  let el = document.head.querySelector(`link[rel="${rel}"]`) as HTMLLinkElement | null;
  if (!el) {
    el = document.createElement("link");
    el.rel = rel;
    document.head.appendChild(el);
  }
  el.href = href;
}

/**
 * Keeps document title, description, canonical, and robots in sync with the route.
 * Staging builds default to noindex unless VITE_ALLOW_INDEX=1.
 */
export function usePageMeta(): void {
  const location = useLocation();

  useEffect(() => {
    const meta = metaForPath(location.pathname);
    document.title = meta.title;
    upsertMeta("name", "description", meta.description);
    upsertMeta("property", "og:title", meta.title);
    upsertMeta("property", "og:description", meta.description);
    upsertMeta("property", "og:type", "website");
    upsertMeta("name", "twitter:card", "summary");
    upsertMeta("name", "twitter:title", meta.title);
    upsertMeta("name", "twitter:description", meta.description);

    const canonical = getCanonicalUrl(location.pathname);
    upsertLink("canonical", canonical);
    if (canonical.startsWith("http")) {
      upsertMeta("property", "og:url", canonical);
    }

    if (shouldNoIndex()) {
      upsertMeta("name", "robots", "noindex, nofollow");
    } else {
      upsertMeta("name", "robots", "index, follow");
    }
  }, [location.pathname]);
}
