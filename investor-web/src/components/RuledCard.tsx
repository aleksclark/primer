import type { ReactNode } from "react";
import { cn } from "@/lib/cn";

interface RuledCardProps {
  children: ReactNode;
  footer?: ReactNode;
  selected?: boolean;
  attention?: boolean;
  className?: string;
  as?: "div" | "article" | "li";
}

/** Square ruled card — surface contrast and rules, never shadows. */
export function RuledCard({
  children,
  footer,
  selected,
  attention,
  className,
  as: Tag = "div",
}: RuledCardProps) {
  return (
    <Tag
      className={cn(
        "ruled-card",
        selected && "ruled-card--selected",
        attention && "ruled-card--attention",
        className,
      )}
    >
      <div className="ruled-card__body">{children}</div>
      {footer != null ? <div className="ruled-card__footer">{footer}</div> : null}
    </Tag>
  );
}

interface RuledGridProps {
  children: ReactNode;
  columns?: 1 | 2 | 3;
  className?: string;
}

/** Ruled multi-column card grid. */
export function RuledGrid({ children, columns = 2, className }: RuledGridProps) {
  return (
    <div
      className={cn(
        "ruled-grid",
        columns === 2 && "ruled-grid--2",
        columns === 3 && "ruled-grid--3",
        className,
      )}
    >
      {children}
    </div>
  );
}
