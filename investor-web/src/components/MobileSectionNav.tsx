import { useEffect, useRef } from "react";
import type { SectionManifestEntry } from "@/data/types";
import { SystemLabel } from "./SystemLabel";

interface MobileSectionNavProps {
  id: string;
  open: boolean;
  onClose: () => void;
  sections: SectionManifestEntry[];
  activeId: string | null;
}

/** Full-screen section list for narrow viewports. */
export function MobileSectionNav({
  id,
  open,
  onClose,
  sections,
  activeId,
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
        <SystemLabel tone="text">Sections</SystemLabel>
        <button
          ref={closeRef}
          type="button"
          className="btn btn--secondary"
          onClick={onClose}
        >
          Close
        </button>
      </div>
      <nav aria-label="Mobile sections">
        <ul className="mobile-section-nav__list">
          {sections.map((section, i) => (
            <li key={section.id}>
              <a
                href={`#${section.id}`}
                data-active={activeId === section.id ? "true" : undefined}
                onClick={onClose}
              >
                <span>{section.navLabel}</span>
                <span className="text-muted">{String(i + 1).padStart(2, "0")}</span>
              </a>
            </li>
          ))}
        </ul>
      </nav>
    </div>
  );
}
