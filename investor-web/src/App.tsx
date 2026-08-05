import { lazy, Suspense, type ReactNode } from "react";
import { BrowserRouter, Navigate, Route, Routes } from "react-router-dom";
import { RouteAnnouncer } from "@/components/RouteAnnouncer";
import { SiteFooter } from "@/components/SiteFooter";
import { SiteHeader } from "@/components/SiteHeader";
import { SkipLink } from "@/components/SkipLink";
import { SystemLabel } from "@/components/SystemLabel";
import { sections } from "@/data";
import { HomePage } from "@/pages/HomePage";

const DemoPage = lazy(() =>
  import("@/pages/DemoPage").then((m) => ({ default: m.DemoPage })),
);
const MarketPage = lazy(() =>
  import("@/pages/MarketPage").then((m) => ({ default: m.MarketPage })),
);
const EvidencePage = lazy(() =>
  import("@/pages/EvidencePage").then((m) => ({ default: m.EvidencePage })),
);
const SchoolsPage = lazy(() =>
  import("@/pages/SchoolsPage").then((m) => ({ default: m.SchoolsPage })),
);
const CompanyPage = lazy(() =>
  import("@/pages/CompanyPage").then((m) => ({ default: m.CompanyPage })),
);
const DiligenceIndexPage = lazy(() =>
  import("@/pages/DiligenceIndexPage").then((m) => ({ default: m.DiligenceIndexPage })),
);

function RouteFallback() {
  return (
    <div className="site-container" style={{ padding: "var(--primer-space-8) 0" }}>
      <SystemLabel>Loading section…</SystemLabel>
    </div>
  );
}

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
        <Suspense fallback={<RouteFallback />}>
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
        </Suspense>
      </AppShell>
    </BrowserRouter>
  );
}
