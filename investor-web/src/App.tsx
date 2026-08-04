import type { ReactNode } from "react";
import { BrowserRouter, Navigate, Route, Routes } from "react-router-dom";
import { RouteAnnouncer } from "@/components/RouteAnnouncer";
import { SiteFooter } from "@/components/SiteFooter";
import { SiteHeader } from "@/components/SiteHeader";
import { SkipLink } from "@/components/SkipLink";
import { sections } from "@/data";
import { CompanyPage } from "@/pages/CompanyPage";
import { DemoPage } from "@/pages/DemoPage";
import { DiligenceIndexPage } from "@/pages/DiligenceIndexPage";
import { EvidencePage } from "@/pages/EvidencePage";
import { HomePage } from "@/pages/HomePage";
import { MarketPage } from "@/pages/MarketPage";
import { SchoolsPage } from "@/pages/SchoolsPage";

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
          <Route path="/demo" element={<DemoPage />} />
          <Route path="/market" element={<MarketPage />} />
          <Route path="/evidence" element={<EvidencePage />} />
          <Route path="/schools" element={<SchoolsPage />} />
          <Route path="/company" element={<CompanyPage />} />
          <Route path="/diligence" element={<DiligenceIndexPage />} />
          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
      </AppShell>
    </BrowserRouter>
  );
}
