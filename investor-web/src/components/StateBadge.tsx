import { toDisplayStatus, type StatusLabel } from "@/data/types";
import { cn } from "@/lib/cn";

const statusClass: Record<StatusLabel, string> = {
  LIVE: "state-badge--live",
  IN_DEVELOPMENT: "state-badge--in-development",
  PLANNED: "state-badge--planned",
  HYPOTHESIS: "state-badge--hypothesis",
  EVIDENCE_NEEDED: "state-badge--evidence-needed",
};

interface StateBadgeProps {
  status: StatusLabel;
  className?: string;
  /** When false, omit the status dot (status remains in text). */
  showDot?: boolean;
}

/**
 * Publication-state badge. Color is never the only cue — label text is always present.
 */
export function StateBadge({ status, className, showDot = true }: StateBadgeProps) {
  const label = toDisplayStatus(status);
  return (
    <span
      className={cn("state-badge", statusClass[status], className)}
      data-status={status}
      title={`Status: ${label}`}
    >
      {showDot ? <span className="state-badge__dot" aria-hidden="true" /> : null}
      <span>{label}</span>
    </span>
  );
}
