/**
 * Privacy-safe analytics scaffold.
 *
 * Default transport is a no-op. A host page or deploy wrapper may assign
 * `window.__PRIMER_ANALYTICS__` to forward events to a privacy-respecting
 * backend. No third-party keys are bundled.
 *
 * Do not send PII, student data, or free-text form bodies.
 */

export type AnalyticsEventName =
  | "cta_click"
  | "contact_intent"
  | "demo_start"
  | "demo_step"
  | "demo_complete"
  | "market_explorer_use"
  | "package_compare"
  | "diligence_route_view"
  | "source_click"
  | "section_view";

export interface AnalyticsPayload {
  /** Stable event name from the allow-list. */
  name: AnalyticsEventName;
  /** Optional low-cardinality props (ids, routes, labels). */
  props?: Record<string, string | number | boolean | undefined>;
}

export type AnalyticsTransport = (event: AnalyticsPayload) => void;

declare global {
  interface Window {
    __PRIMER_ANALYTICS__?: AnalyticsTransport;
  }
}

const isBrowser = typeof window !== "undefined";

function resolveTransport(): AnalyticsTransport {
  if (!isBrowser) return () => {};
  const injected = window.__PRIMER_ANALYTICS__;
  if (typeof injected === "function") return injected;
  // Optional debug sink for local verification — never loads remote scripts.
  if (import.meta.env.DEV && import.meta.env.VITE_ANALYTICS_DEBUG === "1") {
    return (event) => {
      console.debug("[analytics]", event.name, event.props ?? {});
    };
  }
  return () => {};
}

/** Fire a privacy-safe analytics event (no-op by default). */
export function track(name: AnalyticsEventName, props?: AnalyticsPayload["props"]): void {
  try {
    const clean: Record<string, string | number | boolean> = {};
    if (props) {
      for (const [k, v] of Object.entries(props)) {
        if (v === undefined) continue;
        // Hard guard: drop anything that looks like an email or long free text.
        if (typeof v === "string") {
          if (v.length > 120) continue;
          if (/@/.test(v) && /\./.test(v)) continue;
        }
        clean[k] = v;
      }
    }
    resolveTransport()({ name, props: clean });
  } catch {
    // Analytics must never break the pitch site.
  }
}

/** Convenience helpers for common call sites. */
export const analytics = {
  ctaClick: (label: string, href: string) =>
    track("cta_click", { label, href: href.slice(0, 120) }),
  contactIntent: (method: string) => track("contact_intent", { method }),
  demoStart: () => track("demo_start"),
  demoStep: (step: number, stepId: string) => track("demo_step", { step, stepId }),
  demoComplete: () => track("demo_complete"),
  marketExplorerUse: (action: string, layerId?: string) =>
    track("market_explorer_use", { action, layerId }),
  packageCompare: (tierId: string) => track("package_compare", { tierId }),
  diligenceRouteView: (path: string) => track("diligence_route_view", { path }),
  sourceClick: (sourceId: string) => track("source_click", { sourceId }),
  sectionView: (sectionId: string) => track("section_view", { sectionId }),
};

export default analytics;
