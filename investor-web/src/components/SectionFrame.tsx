import type { ReactNode } from "react";
import type { SectionManifestEntry, Source } from "@/data/types";
import { analytics } from "@/lib/analytics";
import { cn } from "@/lib/cn";
import { contactMailto } from "@/lib/contact";
import { CaveatNote } from "./CaveatNote";
import { PrimaryButton } from "./PrimaryButton";
import { SourceLinkList } from "./SourceLink";
import { StateBadge } from "./StateBadge";
import { SystemLabel } from "./SystemLabel";
import { TextLink } from "./TextLink";

function resolveCtaHref(href: string): string {
  if (href.startsWith("mailto:")) return contactMailto();
  return href;
}

function trackSectionCta(label: string, href: string) {
  analytics.ctaClick(label, href);
  if (href.startsWith("mailto:") || href.includes("contact")) {
    analytics.contactIntent(href.startsWith("mailto:") ? "mailto" : "contact_anchor");
  }
}

interface SectionFrameProps {
  section: SectionManifestEntry;
  /** 1-based index shown in mono metadata. */
  index?: number;
  total?: number;
  sources?: Source[];
  children?: ReactNode;
  className?: string;
  /** Override default heading level (thesis may use h1). */
  headingLevel?: 1 | 2;
}

/**
 * Ruled investor section shell: index, status, headline, body, caveats, CTAs.
 * Placeholder content is enough for Phase 1; narrative fills in Phase 2.
 */
export function SectionFrame({
  section,
  index,
  total,
  sources = [],
  children,
  className,
  headingLevel,
}: SectionFrameProps) {
  const isHero = section.id === "thesis";
  const HeadingTag = (headingLevel ?? (isHero ? 1 : 2)) === 1 ? "h1" : "h2";
  const headingClass = isHero ? "type-display" : "type-h1";

  return (
    <section
      id={section.id}
      className={cn("section-frame", className)}
      aria-labelledby={`${section.id}-heading`}
      data-section={section.id}
    >
      <div className="site-container">
        <header className="section-frame__header">
          <div className="section-frame__meta">
            {index != null ? (
              <span className="section-frame__index">
                {String(index).padStart(2, "0")}
                {total != null ? ` / ${String(total).padStart(2, "0")}` : null}
              </span>
            ) : null}
            {section.eyebrow ? <SystemLabel tone="text">{section.eyebrow}</SystemLabel> : null}
            <StateBadge status={section.status} />
          </div>

          <div className="section-frame__title-row">
            <HeadingTag id={`${section.id}-heading`} className={headingClass}>
              {section.headline}
            </HeadingTag>
            {section.proofLine ? (
              <SystemLabel tone="accent">{section.proofLine}</SystemLabel>
            ) : null}
          </div>
        </header>

        <div className="section-frame__body">
          <div className="section-frame__prose">
            {section.subhead ? <p className="type-h3">{section.subhead}</p> : null}
            <p className="type-body prose-measure">{section.body}</p>
          </div>

          {children ? <div className="section-frame__extras">{children}</div> : null}

          {(section.cta || section.secondaryCta) && (
            <div className="section-frame__actions">
              {section.cta ? (
                <PrimaryButton
                  href={resolveCtaHref(section.cta.href)}
                  onClick={() =>
                    trackSectionCta(section.cta!.label, resolveCtaHref(section.cta!.href))
                  }
                >
                  {section.cta.label}
                </PrimaryButton>
              ) : null}
              {section.secondaryCta ? (
                <TextLink
                  href={resolveCtaHref(section.secondaryCta.href)}
                  onClick={() =>
                    trackSectionCta(
                      section.secondaryCta!.label,
                      resolveCtaHref(section.secondaryCta!.href),
                    )
                  }
                >
                  {section.secondaryCta.label}
                </TextLink>
              ) : null}
            </div>
          )}

          {(section.inlineCaveat || section.publicationBlocker || sources.length > 0) && (
            <div className="section-frame__notes">
              {section.inlineCaveat ? <CaveatNote>{section.inlineCaveat}</CaveatNote> : null}
              {section.publicationBlocker ? (
                <CaveatNote blocker>{section.publicationBlocker}</CaveatNote>
              ) : null}
              {sources.length > 0 ? <SourceLinkList sources={sources} /> : null}
            </div>
          )}
        </div>
      </div>
    </section>
  );
}
