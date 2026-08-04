import type { ReactNode } from "react";
import { analytics } from "@/lib/analytics";
import { cn } from "@/lib/cn";
import { contactMailto } from "@/lib/contact";
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

function resolveHref(href: string): string {
  return href.startsWith("mailto:") ? contactMailto() : href;
}

function trackCta(label: string, href: string) {
  analytics.ctaClick(label, href);
  if (href.startsWith("mailto:") || href.includes("contact")) {
    analytics.contactIntent(href.startsWith("mailto:") ? "mailto" : "contact_anchor");
  }
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
  const primary = resolveHref(primaryHref);
  const secondary = secondaryHref ? resolveHref(secondaryHref) : undefined;

  return (
    <aside className={cn("investor-cta", className)} aria-label="Investor call to action">
      <SystemLabel tone="accent">{eyebrow}</SystemLabel>
      <h2 className="type-h2" style={{ margin: 0 }}>
        {headline}
      </h2>
      {body != null ? <div className="type-body prose-measure text-muted">{body}</div> : null}
      <div className="investor-cta__actions">
        <PrimaryButton href={primary} onClick={() => trackCta(primaryLabel, primary)}>
          {primaryLabel}
        </PrimaryButton>
        {secondaryLabel && secondary ? (
          <TextLink href={secondary} onClick={() => trackCta(secondaryLabel, secondary)}>
            {secondaryLabel}
          </TextLink>
        ) : null}
      </div>
    </aside>
  );
}
