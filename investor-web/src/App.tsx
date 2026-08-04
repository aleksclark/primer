import type { ReactNode } from "react";
import { BrowserRouter, Navigate, Route, Routes } from "react-router-dom";
import { RouteAnnouncer } from "@/components/RouteAnnouncer";
import { SiteFooter } from "@/components/SiteFooter";
import { SiteHeader } from "@/components/SiteHeader";
import { SkipLink } from "@/components/SkipLink";
import { sections } from "@/data";
import { HomePage } from "@/pages/HomePage";
import { PlaceholderPage } from "@/pages/PlaceholderPage";

function AppShell({ children }: { children: ReactNode }) {
  return (
    <div className="site-shell">
      <SkipLink />
      <SiteHeader sections={sections} />
      <main id="main-content" className="site-main" tabIndex={-1}>
        {children}
      </main>
      <SiteFooter />
    </div>
  );
}

export function App() {
  return (
    <BrowserRouter>
      <RouteAnnouncer />
      <AppShell>
        <Routes>
          <Route path="/" element={<HomePage />} />
          <Route
            path="/demo"
            element={
              <PlaceholderPage
                eyebrow="Product demo"
                title="Current product surfaces"
                description="Phase 4 will host truthful product surfaces, a tutor-flow prototype, and a product-state legend. Synthetic artifacts only — no student data."
              />
            }
          />
          <Route
            path="/market"
            element={
              <PlaceholderPage
                eyebrow="Market model"
                title="Assumptions and expansion layers"
                description="Interactive market calculator and non-summing expansion ladder land in Phase 3. Structured layer data is already validated in the data package."
              />
            }
          />
          <Route
            path="/evidence"
            element={
              <PlaceholderPage
                eyebrow="Evidence"
                title="Research, claims, and source register"
                description="Diligence-depth evidence pages and the full source register arrive in later phases. Core claims already carry citations on the primary page."
              />
            }
          />
          <Route
            path="/schools"
            element={
              <PlaceholderPage
                eyebrow="Schools"
                title="Supplementary to primary LMS path"
                description="LTI/Clever entry, SIS/SSO roadmap, and privacy/accessibility notes will expand here for institutional diligence."
              />
            }
          />
          <Route
            path="/company"
            element={
              <PlaceholderPage
                eyebrow="Company"
                title="Founder story and team plan"
                description="Extended founder timeline, hiring plan, and advisor notes expand here. Core founder claims are on the primary page."
              />
            }
          />
          <Route
            path="/diligence"
            element={
              <PlaceholderPage
                eyebrow="Diligence"
                title="Product state, risks, and data room"
                description="Risk register, compliance roadmap, and gated data-room link come after the public narrative is correct. No premature portal."
              />
            }
          />
          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
      </AppShell>
    </BrowserRouter>
  );
}
