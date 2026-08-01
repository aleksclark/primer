import type { components } from "@/api/schema";
import { ResourcePage } from "@/components/resource-page";
import { Badge } from "@/components/ui/badge";
import {
  badgeCol,
  codeCol,
  createdCol,
  dateCol,
  gradeCol,
  idCol,
  shortIdCol,
  textCol,
} from "@/lib/columns";

type Schemas = components["schemas"];

export function StudentsPage() {
  type Row = Schemas["Student"];
  return (
    <ResourcePage<Row>
      title="Students"
      path="/students"
      columns={[
        idCol<Row>(),
        textCol<Row>("first_name", "First name", (r) => r.firstName, { sortable: true }),
        textCol<Row>("last_name", "Last name", (r) => r.lastName, { sortable: true }),
        gradeCol<Row>(),
        textCol<Row>("notes", "Notes", (r) => r.notes),
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
        textCol<Row>("name", "Name", (r) => r.name, { sortable: true }),
        textCol<Row>("email", "Email", (r) => r.email, { sortable: true }),
        badgeCol<Row>("role", "Role", (r) => r.role, { sortable: true }),
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
        codeCol<Row>("code", "Code", (r) => r.code, { sortable: true }),
        textCol<Row>("name", "Name", (r) => r.name, { sortable: true }),
        textCol<Row>("description", "Description", (r) => r.description),
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
        codeCol<Row>("code", "Code", (r) => r.code, { sortable: true }),
        badgeCol<Row>("source", "Source", (r) => r.source, { sortable: true, variant: "outline" }),
        gradeCol<Row>(),
        textCol<Row>("domain", "Domain", (r) => r.domain, { sortable: true }),
        textCol<Row>("description", "Description", (r) => r.description),
        textCol<Row>("tcap_weight", "TCAP", (r) => r.tcapWeight),
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
        textCol<Row>("name", "Name", (r) => r.name, { sortable: true }),
        badgeCol<Row>("approach", "Approach", (r) => r.approach, { sortable: true }),
        gradeCol<Row>(),
        textCol<Row>("description", "Description", (r) => r.description),
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
        shortIdCol<Row>("curriculum_id", "Curriculum", (r) => r.curriculumId),
        shortIdCol<Row>("standard_id", "Standard", (r) => r.standardId),
        textCol<Row>("unit", "Unit", (r) => r.unit, { sortable: true }),
        textCol<Row>("position", "Position", (r) => r.position, { sortable: true }),
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
        shortIdCol<Row>("student_id", "Student", (r) => r.studentId),
        shortIdCol<Row>("curriculum_id", "Curriculum", (r) => r.curriculumId),
        badgeCol<Row>("status", "Status", (r) => r.status, { sortable: true }),
        dateCol<Row>("started_on", "Started", (r) => r.startedOn, { sortable: true }),
        dateCol<Row>("ended_on", "Ended", (r) => r.endedOn, { sortable: true }),
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
        shortIdCol<Row>("student_id", "Student", (r) => r.studentId),
        shortIdCol<Row>("standard_id", "Standard", (r) => r.standardId),
        badgeCol<Row>("status", "Status", (r) => r.status, { sortable: true }),
        {
          key: "confidence",
          header: "Confidence",
          sortable: true,
          render: (r) => `${Math.round(r.confidence * 100)}%`,
        },
        dateCol<Row>("next_reinforcement_at", "Next reinforcement", (r) => r.nextReinforcementAt, {
          sortable: true,
        }),
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
        shortIdCol<Row>("mastery_record_id", "Record", (r) => r.masteryRecordId),
        badgeCol<Row>("kind", "Kind", (r) => r.kind, { sortable: true, variant: "outline" }),
        dateCol<Row>("occurred_on", "Date", (r) => r.occurredOn, { sortable: true }),
        textCol<Row>("context", "Context", (r) => r.context),
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
        textCol<Row>("title", "Title", (r) => r.title, { sortable: true }),
        badgeCol<Row>("kind", "Kind", (r) => r.kind, { sortable: true }),
        gradeCol<Row>(),
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
        shortIdCol<Row>("assessment_id", "Assessment", (r) => r.assessmentId),
        textCol<Row>("position", "#", (r) => r.position, { sortable: true }),
        badgeCol<Row>("item_type", "Type", (r) => r.itemType, { sortable: true, variant: "outline" }),
        textCol<Row>("difficulty", "Difficulty", (r) => r.difficulty, { sortable: true }),
        textCol<Row>("stem", "Stem", (r) => r.stem),
        textCol<Row>("points", "Points", (r) => r.points, { sortable: true }),
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
        shortIdCol<Row>("item_id", "Item", (r) => r.itemId),
        textCol<Row>("position", "#", (r) => r.position, { sortable: true }),
        textCol<Row>("text", "Text", (r) => r.text),
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
        shortIdCol<Row>("assessment_id", "Assessment", (r) => r.assessmentId),
        shortIdCol<Row>("student_id", "Student", (r) => r.studentId),
        badgeCol<Row>("status", "Status", (r) => r.status, { sortable: true }),
        {
          key: "score",
          header: "Score",
          sortable: true,
          render: (r) => (r.score != null ? `${r.score}${r.maxScore != null ? ` / ${r.maxScore}` : ""}` : ""),
        },
        dateCol<Row>("started_at", "Started", (r) => r.startedAt, { sortable: true }),
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
        shortIdCol<Row>("attempt_id", "Attempt", (r) => r.attemptId),
        shortIdCol<Row>("item_id", "Item", (r) => r.itemId),
        {
          key: "is_correct",
          header: "Correct",
          sortable: true,
          render: (r) =>
            r.isCorrect == null ? "" : r.isCorrect ? <Badge>yes</Badge> : <Badge variant="destructive">no</Badge>,
        },
        textCol<Row>("points_awarded", "Points", (r) => r.pointsAwarded ?? "", { sortable: true }),
        textCol<Row>("feedback", "Feedback", (r) => r.feedback),
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
