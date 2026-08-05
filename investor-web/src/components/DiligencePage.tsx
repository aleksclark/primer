import type { ReactNode } from "react";
import type { Source } from "@/data/types";
import { CaveatNote } from "./CaveatNote";
import { PrimaryButton } from "./PrimaryButton";
import { SourceLinkList } from "./SourceLink";
import { SystemLabel } from "./SystemLabel";
import { TextLink } from "./TextLink";

export interface DiligenceNavItem {
  id: string;
  label: string;
}

interface DiligencePageProps {
  /** Short route label shown above the title. */
  eyebrow: string;
  title: string;
  summary: string;
  /** ISO date or human-readable update stamp. */
  updated: string;
  /** In-page section anchors rendered as a ruled index. */
  nav: DiligenceNavItem[];
  sources?: Source[];
  /** Optional caveat rendered under the summary. */
  caveat?: string;
  /** Prefer print styles for market/evidence/diligence. */
  printable?: boolean;
  children: ReactNode;
  /** Secondary action under the header. */
  primaryAction?: { label: string; href: string };
  secondaryAction?: { label: string; href: string };
}

/**
 * Data-room style shell for deep diligence routes.
 * Title, updated date, executive summary, ruled section nav, sources.
 */
export function DiligencePage({
  eyebrow,
  title,
  summary,
  updated,
  nav,
  sources = [],
  caveat,
  printable = false,
  children,
  primaryAction,
  secondaryAction,
}: DiligencePageProps) {
  return (
    <div
      className="diligence-page route-page"
      data-printable={printable ? "true" : undefined}
    >
      <div className="site-container">
        <header className="diligence-page__header route-page__header">
          <div className="diligence-page__meta">
            <SystemLabel tone="accent">{eyebrow}</SystemLabel>
            <SystemLabel>Updated {updated}</SystemLabel>
          </div>
          <h1 className="type-h1" style={{ margin: 0 }}>
            {title}
          </h1>
          <p className="type-body prose-measure text-muted" style={{ margin: 0 }}>
            {summary}
          </p>
          {caveat ? <CaveatNote>{caveat}</CaveatNote> : null}
          <div className="diligence-page__actions">
            {primaryAction ? (
              <PrimaryButton href={primaryAction.href}>{primaryAction.label}</PrimaryButton>
            ) : (
              <PrimaryButton href="/">Back to pitch</PrimaryButton>
            )}
            {secondaryAction ? (
              <TextLink href={secondaryAction.href}>{secondaryAction.label}</TextLink>
            ) : (
              <TextLink href="/#contact">Discuss the round</TextLink>
            )}
            {printable ? (
              <PrimaryButton
                variant="quiet"
                onClick={() => window.print()}
                className="diligence-page__print-btn"
              >
                Print summary
              </PrimaryButton>
            ) : null}
          </div>
        </header>

        {nav.length > 0 ? (
          <nav className="diligence-toc" aria-label={`${title} sections`}>
            <SystemLabel>Sections</SystemLabel>
            <ol className="diligence-toc__list">
              {nav.map((item, i) => (
                <li key={item.id}>
                  <a href={`#${item.id}`}>
                    <span className="diligence-toc__index">
                      {String(i + 1).padStart(2, "0")}
                    </span>
                    <span>{item.label}</span>
                  </a>
                </li>
              ))}
            </ol>
          </nav>
        ) : null}

        <div className="diligence-page__body">{children}</div>

        {sources.length > 0 ? (
          <footer className="diligence-page__sources" id="sources">
            <SystemLabel tone="accent">Source register</SystemLabel>
            <SourceLinkList sources={sources} />
          </footer>
        ) : null}
      </div>
    </div>
  );
}

interface DiligenceSectionProps {
  id: string;
  title: string;
  eyebrow?: string;
  children: ReactNode;
}

/** Ruled section block inside a diligence page. */
export function DiligenceSection({ id, title, eyebrow, children }: DiligenceSectionProps) {
  return (
    <section
      id={id}
      className="diligence-section"
      aria-labelledby={`${id}-heading`}
    >
      <header className="diligence-section__header">
        {eyebrow ? <SystemLabel tone="text">{eyebrow}</SystemLabel> : null}
        <h2 id={`${id}-heading`} className="type-h2" style={{ margin: 0 }}>
          {title}
        </h2>
      </header>
      <div className="diligence-section__body">{children}</div>
    </section>
  );
}
