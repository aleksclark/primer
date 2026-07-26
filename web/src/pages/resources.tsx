import type { components } from "@/api/schema";
import { ResourcePage, type ColumnDef, type FieldDef } from "@/components/resource-page";
import { Badge } from "@/components/ui/badge";

type Schemas = components["schemas"];

const shortId = (id?: string) => (id ? id.slice(0, 8) : "");
const shortDate = (v?: string | null) => (v ? new Date(v).toLocaleDateString() : "");

const idCol = <T extends { id: string }>(): ColumnDef<T> => ({
  key: "id",
  header: "ID",
  render: (row) => <code className="text-xs text-muted-foreground">{shortId(row.id)}</code>,
});

const createdCol = <T extends { createdAt: string }>(): ColumnDef<T> => ({
  key: "created_at",
  header: "Created",
  sortable: true,
  render: (row) => shortDate(row.createdAt),
});

export function StudentsPage() {
  type Row = Schemas["Student"];
  return (
    <ResourcePage<Row>
      title="Students"
      path="/students"
      columns={[
        idCol<Row>(),
        { key: "first_name", header: "First name", sortable: true, render: (r) => r.firstName },
        { key: "last_name", header: "Last name", sortable: true, render: (r) => r.lastName },
        { key: "grade_level", header: "Grade", sortable: true, render: (r) => r.gradeLevel ?? "" },
        { key: "notes", header: "Notes", render: (r) => r.notes },
        createdCol<Row>(),
      ]}
      fields={[
        { key: "firstName", label: "First name", required: true },
        { key: "lastName", label: "Last name", required: true },
        { key: "gradeLevel", label: "Grade level", type: "number" },
        { key: "notes", label: "Notes" },
      ]}
    />
  );
}

export function EducatorsPage() {
  type Row = Schemas["Educator"];
  return (
    <ResourcePage<Row>
      title="Educators"
      path="/educators"
      columns={[
        idCol<Row>(),
        { key: "name", header: "Name", sortable: true, render: (r) => r.name },
        { key: "email", header: "Email", sortable: true, render: (r) => r.email },
        { key: "role", header: "Role", sortable: true, render: (r) => <Badge variant="secondary">{r.role}</Badge> },
        createdCol<Row>(),
      ]}
      fields={[
        { key: "name", label: "Name", required: true },
        { key: "email", label: "Email", required: true },
        { key: "role", label: "Role", type: "select", options: ["parent", "admin", "tutor"] },
      ]}
    />
  );
}

export function SubjectsPage() {
  type Row = Schemas["Subject"];
  return (
    <ResourcePage<Row>
      title="Subjects"
      path="/subjects"
      columns={[
        idCol<Row>(),
        { key: "code", header: "Code", sortable: true, render: (r) => <code>{r.code}</code> },
        { key: "name", header: "Name", sortable: true, render: (r) => r.name },
        { key: "description", header: "Description", render: (r) => r.description },
        createdCol<Row>(),
      ]}
      fields={[
        { key: "code", label: "Code", required: true },
        { key: "name", label: "Name", required: true },
        { key: "description", label: "Description" },
      ]}
    />
  );
}

export function StandardsPage() {
  type Row = Schemas["Standard"];
  return (
    <ResourcePage<Row>
      title="Standards"
      path="/standards"
      columns={[
        { key: "code", header: "Code", sortable: true, render: (r) => <code>{r.code}</code> },
        { key: "source", header: "Source", sortable: true, render: (r) => <Badge variant="outline">{r.source}</Badge> },
        { key: "grade_level", header: "Grade", sortable: true, render: (r) => r.gradeLevel ?? "" },
        { key: "domain", header: "Domain", sortable: true, render: (r) => r.domain },
        { key: "description", header: "Description", render: (r) => r.description },
        { key: "tcapWeight", header: "TCAP", render: (r) => r.tcapWeight },
      ]}
      fields={[
        { key: "code", label: "Code", required: true },
        { key: "source", label: "Source", type: "select", options: ["tennessee", "common_core", "custom"], required: true },
        { key: "subjectId", label: "Subject ID" },
        { key: "parentId", label: "Parent standard ID" },
        { key: "gradeLevel", label: "Grade level", type: "number" },
        { key: "domain", label: "Domain" },
        { key: "cluster", label: "Cluster" },
        { key: "description", label: "Description" },
        { key: "tcapWeight", label: "TCAP weight", type: "select", options: ["low", "medium", "high"] },
      ]}
    />
  );
}

export function CurriculaPage() {
  type Row = Schemas["Curriculum"];
  return (
    <ResourcePage<Row>
      title="Curricula"
      path="/curricula"
      columns={[
        idCol<Row>(),
        { key: "name", header: "Name", sortable: true, render: (r) => r.name },
        { key: "approach", header: "Approach", sortable: true, render: (r) => <Badge variant="secondary">{r.approach}</Badge> },
        { key: "grade_level", header: "Grade", sortable: true, render: (r) => r.gradeLevel ?? "" },
        { key: "description", header: "Description", render: (r) => r.description },
        createdCol<Row>(),
      ]}
      fields={[
        { key: "name", label: "Name", required: true },
        {
          key: "approach",
          label: "Approach",
          type: "select",
          options: ["mastery_based", "spiral", "classical", "unit_study", "custom"],
          required: true,
        },
        { key: "gradeLevel", label: "Grade level", type: "number" },
        { key: "description", label: "Description" },
      ]}
    />
  );
}

export function CurriculumStandardsPage() {
  type Row = Schemas["CurriculumStandard"];
  return (
    <ResourcePage<Row>
      title="Curriculum Standards"
      path="/curriculum-standards"
      columns={[
        idCol<Row>(),
        { key: "curriculumId", header: "Curriculum", render: (r) => shortId(r.curriculumId) },
        { key: "standardId", header: "Standard", render: (r) => shortId(r.standardId) },
        { key: "unit", header: "Unit", sortable: true, render: (r) => r.unit },
        { key: "position", header: "Position", sortable: true, render: (r) => r.position },
      ]}
      fields={[
        { key: "curriculumId", label: "Curriculum ID", required: true, createOnly: true },
        { key: "standardId", label: "Standard ID", required: true, createOnly: true },
        { key: "unit", label: "Unit" },
        { key: "position", label: "Position", type: "number" },
        { key: "notes", label: "Notes" },
      ]}
    />
  );
}

export function EnrollmentsPage() {
  type Row = Schemas["Enrollment"];
  return (
    <ResourcePage<Row>
      title="Enrollments"
      path="/enrollments"
      columns={[
        idCol<Row>(),
        { key: "studentId", header: "Student", render: (r) => shortId(r.studentId) },
        { key: "curriculumId", header: "Curriculum", render: (r) => shortId(r.curriculumId) },
        { key: "status", header: "Status", sortable: true, render: (r) => <Badge variant="secondary">{r.status}</Badge> },
        { key: "started_on", header: "Started", sortable: true, render: (r) => shortDate(r.startedOn) },
        { key: "ended_on", header: "Ended", sortable: true, render: (r) => shortDate(r.endedOn) },
      ]}
      fields={[
        { key: "studentId", label: "Student ID", required: true, createOnly: true },
        { key: "curriculumId", label: "Curriculum ID", required: true, createOnly: true },
        { key: "status", label: "Status", type: "select", options: ["active", "paused", "completed", "withdrawn"] },
      ]}
    />
  );
}

export function MasteryRecordsPage() {
  type Row = Schemas["MasteryRecord"];
  return (
    <ResourcePage<Row>
      title="Mastery Records"
      path="/mastery-records"
      columns={[
        idCol<Row>(),
        { key: "studentId", header: "Student", render: (r) => shortId(r.studentId) },
        { key: "standardId", header: "Standard", render: (r) => shortId(r.standardId) },
        { key: "status", header: "Status", sortable: true, render: (r) => <Badge variant="secondary">{r.status}</Badge> },
        {
          key: "confidence",
          header: "Confidence",
          sortable: true,
          render: (r) => `${Math.round(r.confidence * 100)}%`,
        },
        {
          key: "next_reinforcement_at",
          header: "Next reinforcement",
          sortable: true,
          render: (r) => shortDate(r.nextReinforcementAt),
        },
      ]}
      fields={[
        { key: "studentId", label: "Student ID", required: true, createOnly: true },
        { key: "standardId", label: "Standard ID", required: true, createOnly: true },
        {
          key: "status",
          label: "Status",
          type: "select",
          options: ["not_introduced", "in_progress", "approaching", "mastered"],
        },
        { key: "confidence", label: "Confidence (0-1)", type: "number" },
      ]}
    />
  );
}

export function MasteryEvidencePage() {
  type Row = Schemas["MasteryEvidence"];
  return (
    <ResourcePage<Row>
      title="Mastery Evidence"
      path="/mastery-evidence"
      columns={[
        idCol<Row>(),
        { key: "masteryRecordId", header: "Record", render: (r) => shortId(r.masteryRecordId) },
        { key: "kind", header: "Kind", sortable: true, render: (r) => <Badge variant="outline">{r.kind}</Badge> },
        { key: "occurred_on", header: "Date", sortable: true, render: (r) => shortDate(r.occurredOn) },
        { key: "context", header: "Context", render: (r) => r.context },
      ]}
      fields={[
        { key: "masteryRecordId", label: "Mastery record ID", required: true, createOnly: true },
        { key: "kind", label: "Kind", type: "select", options: ["continuous", "formal", "project", "portfolio"], required: true },
        { key: "context", label: "Context" },
        { key: "sourceRef", label: "Source reference" },
      ]}
    />
  );
}

export function AssessmentsPage() {
  type Row = Schemas["Assessment"];
  return (
    <ResourcePage<Row>
      title="Assessments"
      path="/assessments"
      columns={[
        idCol<Row>(),
        { key: "title", header: "Title", sortable: true, render: (r) => r.title },
        { key: "kind", header: "Kind", sortable: true, render: (r) => <Badge variant="secondary">{r.kind}</Badge> },
        { key: "grade_level", header: "Grade", sortable: true, render: (r) => r.gradeLevel ?? "" },
        createdCol<Row>(),
      ]}
      fields={[
        { key: "title", label: "Title", required: true },
        {
          key: "kind",
          label: "Kind",
          type: "select",
          options: ["continuous", "quick_check", "comprehensive", "tcap_practice", "quiz", "project_rubric"],
          required: true,
        },
        { key: "subjectId", label: "Subject ID" },
        { key: "curriculumId", label: "Curriculum ID" },
        { key: "gradeLevel", label: "Grade level", type: "number" },
        { key: "description", label: "Description" },
      ]}
    />
  );
}

export function AssessmentItemsPage() {
  type Row = Schemas["AssessmentItem"];
  return (
    <ResourcePage<Row>
      title="Assessment Items"
      path="/assessment-items"
      columns={[
        idCol<Row>(),
        { key: "assessmentId", header: "Assessment", render: (r) => shortId(r.assessmentId) },
        { key: "position", header: "#", sortable: true, render: (r) => r.position },
        { key: "item_type", header: "Type", sortable: true, render: (r) => <Badge variant="outline">{r.itemType}</Badge> },
        { key: "difficulty", header: "Difficulty", sortable: true, render: (r) => r.difficulty },
        { key: "stem", header: "Stem", render: (r) => r.stem },
        { key: "points", header: "Points", sortable: true, render: (r) => r.points },
      ]}
      fields={[
        { key: "assessmentId", label: "Assessment ID", required: true, createOnly: true },
        { key: "standardId", label: "Standard ID" },
        {
          key: "itemType",
          label: "Item type",
          type: "select",
          options: ["mc", "multi_select", "equation_editor", "constructed_response", "matching", "short_answer", "true_false"],
          required: true,
        },
        { key: "difficulty", label: "Difficulty", type: "select", options: ["approaching", "on_track", "mastered"] },
        { key: "position", label: "Position", type: "number" },
        { key: "stem", label: "Stem", required: true },
        { key: "rationale", label: "Rationale" },
        { key: "points", label: "Points", type: "number" },
      ]}
    />
  );
}

export function AssessmentItemOptionsPage() {
  type Row = Schemas["AssessmentItemOption"];
  return (
    <ResourcePage<Row>
      title="Item Options"
      path="/assessment-item-options"
      columns={[
        idCol<Row>(),
        { key: "itemId", header: "Item", render: (r) => shortId(r.itemId) },
        { key: "position", header: "#", sortable: true, render: (r) => r.position },
        { key: "text", header: "Text", render: (r) => r.text },
        {
          key: "correct",
          header: "Correct",
          sortable: true,
          render: (r) => (r.correct ? <Badge>correct</Badge> : <Badge variant="outline">incorrect</Badge>),
        },
      ]}
      fields={[
        { key: "itemId", label: "Item ID", required: true, createOnly: true },
        { key: "position", label: "Position", type: "number" },
        { key: "text", label: "Text", required: true },
        { key: "correct", label: "Correct", type: "checkbox" },
        { key: "feedback", label: "Feedback" },
      ]}
    />
  );
}

export function AssessmentAttemptsPage() {
  type Row = Schemas["AssessmentAttempt"];
  return (
    <ResourcePage<Row>
      title="Attempts"
      path="/assessment-attempts"
      columns={[
        idCol<Row>(),
        { key: "assessmentId", header: "Assessment", render: (r) => shortId(r.assessmentId) },
        { key: "studentId", header: "Student", render: (r) => shortId(r.studentId) },
        { key: "status", header: "Status", sortable: true, render: (r) => <Badge variant="secondary">{r.status}</Badge> },
        {
          key: "score",
          header: "Score",
          sortable: true,
          render: (r) => (r.score != null ? `${r.score}${r.maxScore != null ? ` / ${r.maxScore}` : ""}` : ""),
        },
        { key: "started_at", header: "Started", sortable: true, render: (r) => shortDate(r.startedAt) },
      ]}
      fields={[
        { key: "assessmentId", label: "Assessment ID", required: true, createOnly: true },
        { key: "studentId", label: "Student ID", required: true, createOnly: true },
        { key: "status", label: "Status", type: "select", options: ["in_progress", "submitted", "scored"] },
        { key: "score", label: "Score", type: "number" },
        { key: "maxScore", label: "Max score", type: "number" },
      ]}
    />
  );
}

export function ItemResponsesPage() {
  type Row = Schemas["ItemResponse"];
  return (
    <ResourcePage<Row>
      title="Item Responses"
      path="/item-responses"
      columns={[
        idCol<Row>(),
        { key: "attemptId", header: "Attempt", render: (r) => shortId(r.attemptId) },
        { key: "itemId", header: "Item", render: (r) => shortId(r.itemId) },
        {
          key: "is_correct",
          header: "Correct",
          sortable: true,
          render: (r) =>
            r.isCorrect == null ? "" : r.isCorrect ? <Badge>yes</Badge> : <Badge variant="destructive">no</Badge>,
        },
        { key: "points_awarded", header: "Points", sortable: true, render: (r) => r.pointsAwarded ?? "" },
        { key: "feedback", header: "Feedback", render: (r) => r.feedback },
      ]}
      fields={[
        { key: "attemptId", label: "Attempt ID", required: true, createOnly: true },
        { key: "itemId", label: "Item ID", required: true, createOnly: true },
        { key: "isCorrect", label: "Correct", type: "checkbox" },
        { key: "pointsAwarded", label: "Points awarded", type: "number" },
        { key: "feedback", label: "Feedback" },
      ]}
    />
  );
}

export const fieldDefsSanity: FieldDef[] = [];
