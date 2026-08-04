import { BrandLogo } from "./BrandLogo";
import { SystemLabel } from "./SystemLabel";
import { TextLink } from "./TextLink";

const diligenceLinks = [
  { label: "Product demo", href: "/demo" },
  { label: "Market model", href: "/market" },
  { label: "Evidence register", href: "/evidence" },
  { label: "Schools path", href: "/schools" },
  { label: "Company", href: "/company" },
  { label: "Diligence", href: "/diligence" },
];

const contactLinks = [
  { label: "Discuss the round", href: "#contact" },
  { label: "Email founder", href: "mailto:aleks@primer.local" },
];

/** Site footer with contact and diligence source links. */
export function SiteFooter() {
  return (
    <footer className="site-footer">
      <div className="site-container">
        <div className="site-footer__inner">
          <div className="site-footer__brand">
            <BrandLogo />
            <p className="type-small text-muted prose-measure" style={{ margin: 0 }}>
              An instructional LMS for mastery. Family-first, school-ready. Grades 5–8.
            </p>
          </div>

          <div className="site-footer__col">
            <SystemLabel>Diligence</SystemLabel>
            <ul className="site-footer__links">
              {diligenceLinks.map((link) => (
                <li key={link.href}>
                  <TextLink href={link.href}>{link.label}</TextLink>
                </li>
              ))}
            </ul>
          </div>

          <div className="site-footer__col">
            <SystemLabel>Contact</SystemLabel>
            <ul className="site-footer__links">
              {contactLinks.map((link) => (
                <li key={link.href}>
                  <TextLink href={link.href}>{link.label}</TextLink>
                </li>
              ))}
            </ul>
          </div>
        </div>

        <div className="site-footer__meta">
          <SystemLabel>System C · Record</SystemLabel>
          <span className="type-small">
            Status labels: Live · In development · Planned · Hypothesis · Evidence needed
          </span>
        </div>
      </div>
    </footer>
  );
}
