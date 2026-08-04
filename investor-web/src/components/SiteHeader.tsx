import { useId, useMemo, useState } from "react";
import type { SectionManifestEntry } from "@/data/types";
import { useActiveSection } from "@/hooks/useActiveSection";
import { useScrolled } from "@/hooks/useScrolled";
import { BrandLogo } from "./BrandLogo";
import { MobileSectionNav } from "./MobileSectionNav";
import { PrimaryButton } from "./PrimaryButton";
import { ThemeToggle } from "./ThemeToggle";

interface SiteHeaderProps {
  sections: SectionManifestEntry[];
}

/** Sticky anchored primary nav with compact scrolled state. */
export function SiteHeader({ sections }: SiteHeaderProps) {
  const scrolled = useScrolled();
  const sectionIds = useMemo(() => sections.map((s) => s.id), [sections]);
  const active = useActiveSection(sectionIds);
  const [mobileOpen, setMobileOpen] = useState(false);
  const menuId = useId();

  return (
    <>
      <header className="site-header" data-scrolled={scrolled ? "true" : "false"}>
        <div className="site-container site-header__inner">
          <a className="site-header__brand" href="#thesis" aria-label="Primer — back to thesis">
            <BrandLogo />
          </a>

          <nav className="site-header__nav" aria-label="Primary">
            {sections.map((section) => (
              <a
                key={section.id}
                href={`#${section.id}`}
                data-active={active === section.id ? "true" : undefined}
                aria-current={active === section.id ? "true" : undefined}
              >
                {section.navLabel}
              </a>
            ))}
          </nav>

          <div className="site-header__actions">
            <ThemeToggle />
            <PrimaryButton href="#contact" className="site-header__cta">
              Discuss the round
            </PrimaryButton>
            <button
              type="button"
              className="site-header__menu-toggle"
              aria-expanded={mobileOpen}
              aria-controls={menuId}
              onClick={() => setMobileOpen(true)}
            >
              Menu
            </button>
          </div>
        </div>
      </header>

      <MobileSectionNav
        id={menuId}
        open={mobileOpen}
        onClose={() => setMobileOpen(false)}
        sections={sections}
        activeId={active}
      />
    </>
  );
}
