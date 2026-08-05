/**
 * Route-level document metadata for titles, descriptions, and canonical URLs.
 */

export interface RouteMeta {
  path: string;
  title: string;
  description: string;
}

export const SITE_NAME = "Primer";
export const DEFAULT_DESCRIPTION =
  "Primer — an instructional LMS for mastery. Investor pitch: family-first, school-ready, grades 5–8.";

/** Production origin override via Vite env; empty means relative canonical. */
export function getSiteOrigin(): string {
  const raw = import.meta.env.VITE_SITE_ORIGIN as string | undefined;
  if (!raw) return "";
  return raw.replace(/\/$/, "");
}

export function getCanonicalUrl(pathname: string): string {
  const origin = getSiteOrigin();
  const path = pathname.startsWith("/") ? pathname : `/${pathname}`;
  if (!origin) return path === "/" ? "/" : path;
  return `${origin}${path === "/" ? "/" : path}`;
}

/**
 * Staging and preview deployments should not be indexed.
 * Production indexing requires both VITE_SITE_ENV=production and VITE_ALLOW_INDEX=1.
 * Vite's MODE is "production" for every `vite build`, so MODE alone must not open indexing.
 */
export function shouldNoIndex(): boolean {
  const flag = import.meta.env.VITE_NOINDEX;
  if (flag === "1" || flag === "true") return true;
  if (flag === "0" || flag === "false") return false;

  const siteEnv = import.meta.env.VITE_SITE_ENV;
  const allow =
    import.meta.env.VITE_ALLOW_INDEX === "1" || import.meta.env.VITE_ALLOW_INDEX === "true";
  if (siteEnv === "production" && allow) return false;
  return true;
}

export const routeMeta: Record<string, RouteMeta> = {
  "/": {
    path: "/",
    title: "Primer investor pitch",
    description: DEFAULT_DESCRIPTION,
  },
  "/demo": {
    path: "/demo",
    title: "Product demo — Primer",
    description:
      "Live product surfaces and a deterministic target-experience script. Synthetic learner data only.",
  },
  "/market": {
    path: "/market",
    title: "Market model — Primer",
    description:
      "Vertical expansion layers with sourced arithmetic, overlap groups, and non-additive ceilings.",
  },
  "/evidence": {
    path: "/evidence",
    title: "Evidence register — Primer",
    description:
      "Research claims, seed-readiness scorecard, and evidence plan with explicit EVIDENCE NEEDED items.",
  },
  "/schools": {
    path: "/schools",
    title: "Schools path — Primer",
    description:
      "Family-first path to school pilots, integrations, and compliance readiness gates.",
  },
  "/company": {
    path: "/company",
    title: "Company — Primer",
    description:
      "Founder proof points, hiring plan, material risks, and investor contact process.",
  },
  "/diligence": {
    path: "/diligence",
    title: "Diligence — Primer",
    description:
      "Diligence index for market, evidence, product, schools, company, and source registers.",
  },
};

export function metaForPath(pathname: string): RouteMeta {
  return (
    routeMeta[pathname] ?? {
      path: pathname,
      title: "Primer",
      description: DEFAULT_DESCRIPTION,
    }
  );
}
