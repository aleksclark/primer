import { NavLink, Route, Routes, Navigate } from "react-router-dom";
import { BarChart3, CalendarClock, CalendarRange, GraduationCap, Library, LogOut, Tv } from "lucide-react";
import { cn } from "@/lib/utils";
import { clearAdminKey } from "@/api/auth";
import { AdminKeyGate } from "@/components/admin-key-gate";
import { Button } from "@/components/ui/button";
import { AvailabilityPage } from "@/pages/availability";
import { DevicesPage } from "@/pages/devices";
import { LibraryPage } from "@/pages/library";
import { MetricsPage } from "@/pages/metrics";
import { PrimerPage } from "@/pages/primer";
import { SchedulePage } from "@/pages/schedule";

const nav = [
  {
    section: "Content",
    links: [
      { to: "/library", label: "Library", icon: Library, page: <LibraryPage /> },
      { to: "/availability", label: "Availability", icon: CalendarClock, page: <AvailabilityPage /> },
      { to: "/schedule", label: "Schedule", icon: CalendarRange, page: <SchedulePage /> },
    ],
  },
  {
    section: "Clients",
    links: [{ to: "/devices", label: "Devices", icon: Tv, page: <DevicesPage /> }],
  },
  {
    section: "Reporting",
    links: [
      { to: "/metrics", label: "Viewing", icon: BarChart3, page: <MetricsPage /> },
      { to: "/primer-reports", label: "Primer Reports", icon: GraduationCap, page: <PrimerPage /> },
    ],
  },
];

export default function App() {
  return (
    <AdminKeyGate>
      <div className="flex min-h-screen">
        <aside className="flex w-60 shrink-0 flex-col border-r bg-muted/30">
          <div className="flex h-14 items-center border-b px-4">
            <span className="text-lg font-semibold tracking-tight">Primer TV</span>
          </div>
          <nav className="flex-1 space-y-4 p-3">
            {nav.map((group) => (
              <div key={group.section}>
                <p className="px-2 pb-1 text-xs font-medium uppercase tracking-wide text-muted-foreground">
                  {group.section}
                </p>
                <div className="space-y-0.5">
                  {group.links.map((link) => (
                    <NavLink
                      key={link.to}
                      to={link.to}
                      className={({ isActive }) =>
                        cn(
                          "flex items-center gap-2 rounded-md px-2 py-1.5 text-sm transition-colors",
                          isActive
                            ? "bg-primary text-primary-foreground"
                            : "text-muted-foreground hover:bg-accent hover:text-accent-foreground",
                        )
                      }
                    >
                      <link.icon className="h-4 w-4" />
                      {link.label}
                    </NavLink>
                  ))}
                </div>
              </div>
            ))}
          </nav>
          <div className="border-t p-3">
            <Button variant="ghost" size="sm" className="w-full justify-start" onClick={clearAdminKey}>
              <LogOut /> Forget admin key
            </Button>
          </div>
        </aside>
        <main className="flex-1 p-6">
          <Routes>
            <Route path="/" element={<Navigate to="/library" replace />} />
            {nav.flatMap((group) =>
              group.links.map((link) => <Route key={link.to} path={link.to} element={link.page} />),
            )}
          </Routes>
        </main>
      </div>
    </AdminKeyGate>
  );
}
