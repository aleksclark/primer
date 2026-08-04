import { analytics } from "@/lib/analytics";
import { contactMailto } from "@/lib/contact";
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

/** Site footer with contact and diligence source links. */
export function SiteFooter() {
  const mailto = contactMailto();
  const contactLinks = [
    { label: "Discuss the round", href: "/#contact" },
    { label: "Primary pitch", href: "/" },
    { label: "Email founder", href: mailto },
  ];

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
                  <TextLink
                    href={link.href}
                    onClick={() => {
                      if (link.href.includes("contact") || link.href.startsWith("mailto:")) {
                        analytics.ctaClick(link.label, link.href);
                        analytics.contactIntent(
                          link.href.startsWith("mailto:") ? "mailto" : "contact_anchor",
                        );
                      }
                    }}
                  >
                    {link.label}
                  </TextLink>
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
