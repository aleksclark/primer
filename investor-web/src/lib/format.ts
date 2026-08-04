import type { MetricCurrentValue } from "@/data/types";

/** Format a learner count for investor-facing equations. */
export function formatLearners(n: number): string {
  if (!Number.isFinite(n)) return "—";
  const abs = Math.abs(n);
  if (abs >= 1_000_000) {
    const millions = n / 1_000_000;
    const rounded =
      Math.abs(millions - Math.round(millions)) < 1e-9
        ? Math.round(millions).toString()
        : millions.toFixed(2).replace(/\.?0+$/, "");
    return `${rounded}M`;
  }
  if (abs >= 10_000) return n.toLocaleString("en-US");
  return n.toLocaleString("en-US");
}

/** Currency with full separators, no cents for whole dollars. */
export function formatUsd(n: number, opts?: { compact?: boolean }): string {
  if (!Number.isFinite(n)) return "—";
  if (opts?.compact) {
    const abs = Math.abs(n);
    if (abs >= 1_000_000_000) {
      const billions = n / 1_000_000_000;
      const text =
        Math.abs(billions - Math.round(billions * 100) / 100) < 1e-9
          ? billions.toFixed(2).replace(/\.?0+$/, "")
          : billions.toFixed(2);
      return `$${text}B`;
    }
    if (abs >= 1_000_000) {
      const millions = n / 1_000_000;
      const text =
        Math.abs(millions - Math.round(millions * 10) / 10) < 1e-9 &&
        millions % 1 !== 0
          ? millions.toFixed(1)
          : Math.abs(millions - Math.round(millions)) < 1e-9
            ? Math.round(millions).toString()
            : millions.toFixed(1).replace(/\.0$/, "");
      return `$${text}M`;
    }
    if (abs >= 1_000) {
      return `$${Math.round(n / 1_000)}k`;
    }
  }
  return new Intl.NumberFormat("en-US", {
    style: "currency",
    currency: "USD",
    maximumFractionDigits: Number.isInteger(n) ? 0 : 2,
  }).format(n);
}

/** Compact ceiling display ($2.04B style). */
export function formatCeiling(n: number): string {
  return formatUsd(n, { compact: true });
}

/** Scorecard current/floor/target display. Never renders 0 for unknown. */
export function formatMetricValue(value: MetricCurrentValue, unit: string): string {
  if (value === "NOT_YET_MEASURED") return "NOT YET MEASURED";
  if (unit === "percent") return `${value}%`;
  if (unit === "ratio") return `${value}×`;
  if (unit === "standard_deviations") return `${value} SD`;
  if (unit === "months") return `${value} mo`;
  if (unit === "minutes_per_account") return `${value} min`;
  if (unit === "minutes_per_learner_month") return `${value} min`;
  if (unit === "pilots" || unit === "tests" || unit === "learners") {
    return value.toLocaleString("en-US");
  }
  if (unit.startsWith("usd")) return formatUsd(value);
  return String(value);
}

/** Human label for observation kind. */
export function formatObservationKind(kind: "observed" | "modeled" | "derived"): string {
  return kind;
}
