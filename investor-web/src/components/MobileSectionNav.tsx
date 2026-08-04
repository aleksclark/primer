import { useEffect, useRef } from "react";
import type { SectionManifestEntry } from "@/data/types";
import { SystemLabel } from "./SystemLabel";

interface DiligenceLink {
  id: string;
  label: string;
  href: string;
}

interface MobileSectionNavProps {
  id: string;
  open: boolean;
  onClose: () => void;
  sections: SectionManifestEntry[];
  activeId: string | null;
  /** When true, section links are in-page anchors; otherwise prefix with /. */
  homeAnchors?: boolean;
  /** When set, show diligence route links instead of home sections. */
  diligenceLinks?: DiligenceLink[];
}

/** Full-screen section list for narrow viewports. */
export function MobileSectionNav({
  id,
  open,
  onClose,
  sections,
  activeId,
  homeAnchors = true,
  diligenceLinks,
}: MobileSectionNavProps) {
  const closeRef = useRef<HTMLButtonElement>(null);

  useEffect(() => {
    if (!open) return;
    closeRef.current?.focus();
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    document.addEventListener("keydown", onKey);
    const prev = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    return () => {
      document.removeEventListener("keydown", onKey);
      document.body.style.overflow = prev;
    };
  }, [open, onClose]);

  return (
    <div
      id={id}
      className="mobile-section-nav"
      data-open={open ? "true" : "false"}
      hidden={!open}
      role="dialog"
      aria-modal="true"
      aria-label="Section navigation"
    >
      <div className="mobile-section-nav__header">
        <SystemLabel tone="text">{diligenceLinks ? "Diligence" : "Sections"}</SystemLabel>
        <button
          ref={closeRef}
          type="button"
          className="btn btn--secondary"
          onClick={onClose}
        >
          Close
        </button>
      </div>
      <nav aria-label={diligenceLinks ? "Diligence pages" : "Mobile sections"}>
        <ul className="mobile-section-nav__list">
          {diligenceLinks
            ? diligenceLinks.map((link, i) => (
                <li key={link.id}>
                  <a href={link.href} onClick={onClose}>
                    <span>{link.label}</span>
                    <span className="text-muted">{String(i + 1).padStart(2, "0")}</span>
                  </a>
                </li>
              ))
            : sections.map((section, i) => (
                <li key={section.id}>
                  <a
                    href={homeAnchors ? `#${section.id}` : `/#${section.id}`}
                    data-active={activeId === section.id ? "true" : undefined}
                    onClick={onClose}
                  >
                    <span>{section.navLabel}</span>
                    <span className="text-muted">{String(i + 1).padStart(2, "0")}</span>
                  </a>
                </li>
              ))}
          <li>
            <a href={homeAnchors ? "#contact" : "/#contact"} onClick={onClose}>
              <span>Contact</span>
              <span className="text-muted">→</span>
            </a>
          </li>
          {!diligenceLinks ? (
            <li>
              <a href="/diligence" onClick={onClose}>
                <span>Diligence index</span>
                <span className="text-muted">→</span>
              </a>
            </li>
          ) : (
            <li>
              <a href="/" onClick={onClose}>
                <span>Back to pitch</span>
                <span className="text-muted">→</span>
              </a>
            </li>
          )}
        </ul>
      </nav>
    </div>
  );
}
