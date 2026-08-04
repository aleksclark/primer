import { useId, useMemo, useState } from "react";
import { useLocation } from "react-router-dom";
import type { SectionManifestEntry } from "@/data/types";
import { useActiveSection } from "@/hooks/useActiveSection";
import { useScrolled } from "@/hooks/useScrolled";
import { analytics } from "@/lib/analytics";
import { BrandLogo } from "./BrandLogo";
import { MobileSectionNav } from "./MobileSectionNav";
import { PrimaryButton } from "./PrimaryButton";
import { ThemeToggle } from "./ThemeToggle";

interface SiteHeaderProps {
  sections: SectionManifestEntry[];
}

const diligenceNav = [
  { id: "demo", label: "Demo", href: "/demo" },
  { id: "market", label: "Market", href: "/market" },
  { id: "evidence", label: "Evidence", href: "/evidence" },
  { id: "schools", label: "Schools", href: "/schools" },
  { id: "company", label: "Company", href: "/company" },
  { id: "diligence", label: "Diligence", href: "/diligence" },
];

/** Sticky anchored primary nav with compact scrolled state. */
export function SiteHeader({ sections }: SiteHeaderProps) {
  const scrolled = useScrolled();
  const location = useLocation();
  const onHome = location.pathname === "/";
  const sectionIds = useMemo(() => sections.map((s) => s.id), [sections]);
  const active = useActiveSection(onHome ? sectionIds : []);
  const [mobileOpen, setMobileOpen] = useState(false);
  const menuId = useId();

  const sectionHref = (id: string) => (onHome ? `#${id}` : `/#${id}`);
  const contactHref = onHome ? "#contact" : "/#contact";
  const brandHref = onHome ? "#thesis" : "/";

  return (
    <>
      <header className="site-header" data-scrolled={scrolled ? "true" : "false"}>
        <div className="site-container site-header__inner">
          <a className="site-header__brand" href={brandHref} aria-label="Primer — back to thesis">
            <BrandLogo />
          </a>

          <nav className="site-header__nav" aria-label="Primary">
            {onHome
              ? sections.map((section) => (
                  <a
                    key={section.id}
                    href={sectionHref(section.id)}
                    data-active={active === section.id ? "true" : undefined}
                    aria-current={active === section.id ? "true" : undefined}
                  >
                    {section.navLabel}
                  </a>
                ))
              : diligenceNav.map((item) => (
                  <a
                    key={item.id}
                    href={item.href}
                    data-active={location.pathname === item.href ? "true" : undefined}
                    aria-current={location.pathname === item.href ? "page" : undefined}
                  >
                    {item.label}
                  </a>
                ))}
          </nav>

          <div className="site-header__actions">
            <ThemeToggle />
            <PrimaryButton
              href={contactHref}
              className="site-header__cta"
              onClick={() => {
                analytics.ctaClick("Discuss the round", contactHref);
                analytics.contactIntent("header_cta");
              }}
            >
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
        homeAnchors={onHome}
        diligenceLinks={!onHome ? diligenceNav : undefined}
      />
    </>
  );
}
