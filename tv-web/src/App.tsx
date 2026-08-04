import { useCallback, useEffect, useState } from "react";
import { NavLink, Route, Routes, Navigate } from "react-router-dom";
import {
  BarChart3,
  CalendarClock,
  CalendarRange,
  GraduationCap,
  Library,
  LogOut,
  Moon,
  Sun,
  Tv,
} from "lucide-react";
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

type Theme = "dark" | "light";

const THEME_KEY = "primer-theme";

function readTheme(): Theme {
  if (typeof document === "undefined") return "dark";
  const attr = document.documentElement.getAttribute("data-theme");
  return attr === "light" ? "light" : "dark";
}

function applyTheme(theme: Theme) {
  document.documentElement.setAttribute("data-theme", theme);
  try {
    localStorage.setItem(THEME_KEY, theme);
  } catch {
    // Ignore storage failures (private mode, etc.).
  }
}

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
  const [theme, setTheme] = useState<Theme>(() => readTheme());

  useEffect(() => {
    applyTheme(theme);
  }, [theme]);

  const toggleTheme = useCallback(() => {
    setTheme((current) => (current === "dark" ? "light" : "dark"));
  }, []);

  return (
    <AdminKeyGate>
      <div className="flex min-h-screen bg-background text-foreground">
        <aside className="flex w-64 shrink-0 flex-col border-r border-border bg-background">
          <div className="flex h-16 items-center justify-between gap-3 border-b border-rule-strong px-4">
            <div className="flex min-w-0 items-center gap-3">
              <img
                src="/brand/logo-mark.svg"
                alt=""
                width={28}
                height={28}
                className="logo-mark-dark h-7 w-7 shrink-0"
              />
              <img
                src="/brand/logo-mark-light.svg"
                alt=""
                width={28}
                height={28}
                className="logo-mark-light h-7 w-7 shrink-0"
              />
              <span className="type-h3 truncate">
                Primer<span className="font-bold">TV</span>
              </span>
            </div>
            <button
              type="button"
              onClick={toggleTheme}
              className={cn(
                "type-label inline-flex h-8 w-8 shrink-0 items-center justify-center",
                "border border-border text-muted-foreground transition-colors",
                "hover:border-muted-foreground hover:text-foreground",
                "focus-visible:outline focus-visible:outline-[length:var(--primer-focus-width)]",
                "focus-visible:outline-offset-[var(--primer-focus-offset)] focus-visible:outline-primary",
              )}
              title={theme === "dark" ? "Switch to light theme" : "Switch to dark theme"}
              aria-label={theme === "dark" ? "Switch to light theme" : "Switch to dark theme"}
            >
              {theme === "dark" ? <Sun className="h-3.5 w-3.5" /> : <Moon className="h-3.5 w-3.5" />}
            </button>
          </div>

          <nav className="flex-1 overflow-y-auto py-2" aria-label="Primary">
            {nav.map((group) => (
              <div key={group.section} className="py-2">
                <p className="type-label px-4 py-2 text-muted-foreground">{group.section}</p>
                <div>
                  {group.links.map((link) => (
                    <NavLink
                      key={link.to}
                      to={link.to}
                      className={({ isActive }) =>
                        cn(
                          "flex items-center gap-2.5 border-t border-border px-4 py-3 text-sm transition-colors",
                          "last:border-b",
                          isActive
                            ? "border-l-2 border-l-primary pl-[14px] text-foreground"
                            : "border-l-2 border-l-transparent pl-[14px] text-muted-foreground hover:text-foreground",
                        )
                      }
                    >
                      <link.icon className="h-4 w-4 shrink-0" aria-hidden />
                      <span className="truncate">{link.label}</span>
                    </NavLink>
                  ))}
                </div>
              </div>
            ))}
          </nav>

          <div className="space-y-3 border-t border-border px-4 py-3">
            <Button variant="ghost" size="sm" className="w-full justify-start" onClick={clearAdminKey}>
              <LogOut /> Forget admin key
            </Button>
            <p className="type-label text-muted-foreground">System C · TV Admin</p>
          </div>
        </aside>

        <main className="min-w-0 flex-1 overflow-auto p-6 md:p-8">
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
