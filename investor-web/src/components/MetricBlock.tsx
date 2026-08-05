import type { ReactNode } from "react";
import { cn } from "@/lib/cn";
import { SystemLabel } from "./SystemLabel";

interface MetricBlockProps {
  label: string;
  value: ReactNode;
  delta?: ReactNode;
  hint?: ReactNode;
  className?: string;
}

/** Single metric cell in the ruled System C language. */
export function MetricBlock({ label, value, delta, hint, className }: MetricBlockProps) {
  return (
    <div className={cn("metric-block", className)}>
      <SystemLabel>{label}</SystemLabel>
      <div className="metric-block__value-row" style={{ display: "flex", alignItems: "baseline", gap: "var(--primer-space-3)" }}>
        <div className="metric-block__value">{value}</div>
        {delta != null ? <div className="metric-block__delta">{delta}</div> : null}
      </div>
      {hint != null ? <div className="metric-block__hint">{hint}</div> : null}
    </div>
  );
}

interface MetricRowProps {
  children: ReactNode;
  className?: string;
  /** Accessible summary for screen readers / future charts. */
  summary?: string;
}

/** Horizontal ruled metric strip. */
export function MetricRow({ children, className, summary }: MetricRowProps) {
  return (
    <div className={cn("metric-row", className)} role="group" aria-label={summary}>
      {children}
      {summary ? <p className="sr-only">{summary}</p> : null}
    </div>
  );
}
