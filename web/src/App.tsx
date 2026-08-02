import { NavLink, Route, Routes, Navigate } from "react-router-dom";
import {
  BookOpen,
  Clapperboard,
  ClipboardCheck,
  ClipboardList,
  GraduationCap,
  Layers,
  Library,
  ListChecks,
  ListOrdered,
  MessageSquare,
  Target,
  TrendingUp,
  UserCheck,
  Users,
} from "lucide-react";
import { cn } from "@/lib/utils";
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
  return (
    <div className="flex min-h-screen">
      <aside className="w-60 shrink-0 border-r bg-muted/30">
        <div className="flex h-14 items-center border-b px-4">
          <span className="text-lg font-semibold tracking-tight">Primer LMS</span>
        </div>
        <nav className="space-y-4 p-3">
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
      </aside>
      <main className="flex-1 p-6">
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
