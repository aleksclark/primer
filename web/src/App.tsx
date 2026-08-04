import { useCallback, useEffect, useState } from "react";
import { NavLink, Route, Routes, Navigate } from "react-router-dom";
import {
  Activity,
  BookOpen,
  Clapperboard,
  ClipboardCheck,
  ClipboardList,
  GraduationCap,
  Layers,
  LayoutDashboard,
  Library,
  ListChecks,
  ListOrdered,
  MessageSquare,
  MonitorSmartphone,
  Moon,
  Sun,
  Target,
  TrendingUp,
  UserCheck,
  Users,
} from "lucide-react";
import { cn } from "@/lib/utils";
import { ParentTokenGate } from "@/components/parent-token-gate";
import {
  AssessmentAttemptsPage,
  AssessmentItemOptionsPage,
  AssessmentItemsPage,
  AssessmentsPage,
  CurriculaPage,
  CurriculumStandardsPage,
  EducatorsPage,
  EnrollmentsPage,
  InstructionLogsPage,
  ItemResponsesPage,
  MasteryEvidencePage,
  MasteryRecordsPage,
  StandardsPage,
  StudentsPage,
  SubjectsPage,
} from "@/pages/resources";
import {
  LearningAssignmentsPage,
  LearningOverviewPage,
  LearningSessionsPage,
  StudentDevicesPage,
} from "@/pages/student-client";

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
    section: "People",
    links: [
      { to: "/students", label: "Students", icon: Users, page: <StudentsPage /> },
      { to: "/educators", label: "Educators", icon: UserCheck, page: <EducatorsPage /> },
    ],
  },
  {
    section: "Curriculum",
    links: [
      { to: "/subjects", label: "Subjects", icon: Library, page: <SubjectsPage /> },
      { to: "/standards", label: "Standards", icon: Target, page: <StandardsPage /> },
      { to: "/curricula", label: "Curricula", icon: BookOpen, page: <CurriculaPage /> },
      { to: "/curriculum-standards", label: "Sequencing", icon: ListOrdered, page: <CurriculumStandardsPage /> },
      { to: "/enrollments", label: "Enrollments", icon: GraduationCap, page: <EnrollmentsPage /> },
    ],
  },
  {
    section: "Mastery",
    links: [
      { to: "/mastery-records", label: "Mastery Records", icon: TrendingUp, page: <MasteryRecordsPage /> },
      { to: "/mastery-evidence", label: "Evidence", icon: ClipboardCheck, page: <MasteryEvidencePage /> },
    ],
  },
  {
    section: "Instruction",
    links: [
      { to: "/instruction-logs", label: "Instruction Logs", icon: Clapperboard, page: <InstructionLogsPage /> },
    ],
  },
  {
    section: "Student client",
    links: [
      {
        to: "/student-devices",
        label: "Devices",
        icon: MonitorSmartphone,
        page: (
          <ParentTokenGate>
            <StudentDevicesPage />
          </ParentTokenGate>
        ),
      },
      {
        to: "/learning-assignments",
        label: "Assignments",
        icon: ClipboardList,
        page: (
          <ParentTokenGate>
            <LearningAssignmentsPage />
          </ParentTokenGate>
        ),
      },
      {
        to: "/learning-sessions",
        label: "Sessions",
        icon: Activity,
        page: (
          <ParentTokenGate>
            <LearningSessionsPage />
          </ParentTokenGate>
        ),
      },
      {
        to: "/learning-overview",
        label: "Overview",
        icon: LayoutDashboard,
        page: (
          <ParentTokenGate>
            <LearningOverviewPage />
          </ParentTokenGate>
        ),
      },
    ],
  },
  {
    section: "Assessment",
    links: [
      { to: "/assessments", label: "Assessments", icon: ClipboardList, page: <AssessmentsPage /> },
      { to: "/assessment-items", label: "Items", icon: ListChecks, page: <AssessmentItemsPage /> },
      { to: "/assessment-item-options", label: "Options", icon: Layers, page: <AssessmentItemOptionsPage /> },
      { to: "/assessment-attempts", label: "Attempts", icon: GraduationCap, page: <AssessmentAttemptsPage /> },
      { to: "/item-responses", label: "Responses", icon: MessageSquare, page: <ItemResponsesPage /> },
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
              Primer<span className="font-bold">LMS</span>
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

        <div className="border-t border-border px-4 py-3">
          <p className="type-label text-muted-foreground">System C · Admin</p>
        </div>
      </aside>

      <main className="min-w-0 flex-1 overflow-auto p-6 md:p-8">
        <Routes>
          <Route path="/" element={<Navigate to="/students" replace />} />
          {nav.flatMap((group) =>
            group.links.map((link) => <Route key={link.to} path={link.to} element={link.page} />),
          )}
        </Routes>
      </main>
    </div>
  );
}
