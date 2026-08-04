import type { ReactNode } from "react";
import { cn } from "@/lib/cn";
import { PrimaryButton } from "./PrimaryButton";
import { SystemLabel } from "./SystemLabel";
import { TextLink } from "./TextLink";

interface InvestorCTAProps {
  eyebrow?: string;
  headline: string;
  body?: ReactNode;
  primaryLabel?: string;
  primaryHref?: string;
  secondaryLabel?: string;
  secondaryHref?: string;
  className?: string;
}

/** End-of-page / section call-to-action block for the round conversation. */
export function InvestorCTA({
  eyebrow = "Next step",
  headline,
  body,
  primaryLabel = "Discuss the round",
  primaryHref = "#contact",
  secondaryLabel,
  secondaryHref,
  className,
}: InvestorCTAProps) {
  return (
    <aside className={cn("investor-cta", className)} aria-label="Investor call to action">
      <SystemLabel tone="accent">{eyebrow}</SystemLabel>
      <h2 className="type-h2" style={{ margin: 0 }}>
        {headline}
      </h2>
      {body != null ? <div className="type-body prose-measure text-muted">{body}</div> : null}
      <div className="investor-cta__actions">
        <PrimaryButton href={primaryHref}>{primaryLabel}</PrimaryButton>
        {secondaryLabel && secondaryHref ? (
          <TextLink href={secondaryHref}>{secondaryLabel}</TextLink>
        ) : null}
      </div>
    </aside>
  );
}
