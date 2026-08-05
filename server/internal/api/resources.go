package api

import (
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/aleksclark/primer/server/internal/domain"
	"github.com/aleksclark/primer/server/internal/repo"
	"github.com/aleksclark/primer/server/internal/tutor"
)

// Create/update request bodies. The db tags map fields to columns; update
// bodies use pointer fields so that omitted fields are left unchanged.

// EducatorCreate is the body for creating an educator.
type EducatorCreate struct {
	Email string `json:"email" db:"email" format:"email" minLength:"3"`
	Name  string `json:"name" db:"name" minLength:"1"`
	Role  string `json:"role,omitempty" db:"role" enum:"parent,admin,tutor" default:"parent" required:"false"`
}

// EducatorUpdate is the body for updating an educator.
type EducatorUpdate struct {
	Email *string `json:"email,omitempty" db:"email" format:"email" required:"false"`
	Name  *string `json:"name,omitempty" db:"name" required:"false"`
	Role  *string `json:"role,omitempty" db:"role" enum:"parent,admin,tutor" required:"false"`
}

// StudentCreate is the body for creating a student.
type StudentCreate struct {
	FirstName  string     `json:"firstName" db:"first_name" minLength:"1"`
	LastName   string     `json:"lastName" db:"last_name" minLength:"1"`
	Birthdate  *time.Time `json:"birthdate,omitempty" db:"birthdate" required:"false"`
	GradeLevel *int       `json:"gradeLevel,omitempty" db:"grade_level" minimum:"0" maximum:"12" required:"false"`
	Notes      *string    `json:"notes,omitempty" db:"notes" required:"false"`
}

// StudentUpdate is the body for updating a student.
type StudentUpdate struct {
	FirstName  *string    `json:"firstName,omitempty" db:"first_name" required:"false"`
	LastName   *string    `json:"lastName,omitempty" db:"last_name" required:"false"`
	Birthdate  *time.Time `json:"birthdate,omitempty" db:"birthdate" required:"false"`
	GradeLevel *int       `json:"gradeLevel,omitempty" db:"grade_level" minimum:"0" maximum:"12" required:"false"`
	Notes      *string    `json:"notes,omitempty" db:"notes" required:"false"`
}

// SubjectCreate is the body for creating a subject.
type SubjectCreate struct {
	Code        string  `json:"code" db:"code" minLength:"1"`
	Name        string  `json:"name" db:"name" minLength:"1"`
	Description *string `json:"description,omitempty" db:"description" required:"false"`
}

// SubjectUpdate is the body for updating a subject.
type SubjectUpdate struct {
	Code        *string `json:"code,omitempty" db:"code" required:"false"`
	Name        *string `json:"name,omitempty" db:"name" required:"false"`
	Description *string `json:"description,omitempty" db:"description" required:"false"`
}

// StandardCreate is the body for creating a standard.
type StandardCreate struct {
	Source      string          `json:"source" db:"source" enum:"tennessee,common_core,custom"`
	SubjectID   *string         `json:"subjectId,omitempty" db:"subject_id" format:"uuid" required:"false"`
	ParentID    *string         `json:"parentId,omitempty" db:"parent_id" format:"uuid" required:"false"`
	Code        string          `json:"code" db:"code" minLength:"1"`
	GradeLevel  *int            `json:"gradeLevel,omitempty" db:"grade_level" required:"false"`
	Domain      *string         `json:"domain,omitempty" db:"domain" required:"false"`
	Cluster     *string         `json:"cluster,omitempty" db:"cluster" required:"false"`
	Description *string         `json:"description,omitempty" db:"description" required:"false"`
	TCAPWeight  *string         `json:"tcapWeight,omitempty" db:"tcap_weight" enum:"low,medium,high" required:"false"`
	Metadata    *map[string]any `json:"metadata,omitempty" db:"metadata" required:"false"`
}

// StandardUpdate is the body for updating a standard.
type StandardUpdate struct {
	Source      *string         `json:"source,omitempty" db:"source" enum:"tennessee,common_core,custom" required:"false"`
	SubjectID   *string         `json:"subjectId,omitempty" db:"subject_id" format:"uuid" required:"false"`
	ParentID    *string         `json:"parentId,omitempty" db:"parent_id" format:"uuid" required:"false"`
	Code        *string         `json:"code,omitempty" db:"code" required:"false"`
	GradeLevel  *int            `json:"gradeLevel,omitempty" db:"grade_level" required:"false"`
	Domain      *string         `json:"domain,omitempty" db:"domain" required:"false"`
	Cluster     *string         `json:"cluster,omitempty" db:"cluster" required:"false"`
	Description *string         `json:"description,omitempty" db:"description" required:"false"`
	TCAPWeight  *string         `json:"tcapWeight,omitempty" db:"tcap_weight" enum:"low,medium,high" required:"false"`
	Metadata    *map[string]any `json:"metadata,omitempty" db:"metadata" required:"false"`
}

// CurriculumCreate is the body for creating a curriculum.
type CurriculumCreate struct {
	Slug        string          `json:"slug" db:"slug" minLength:"1"`
	Name        string          `json:"name" db:"name" minLength:"1"`
	Description *string         `json:"description,omitempty" db:"description" required:"false"`
	Approach    string          `json:"approach" db:"approach" enum:"mastery_based,spiral,classical,unit_study,custom"`
	SubjectCode *string         `json:"subjectCode,omitempty" db:"subject_code" required:"false"`
	Status      *string         `json:"status,omitempty" db:"status" enum:"draft,published,retired" required:"false"`
	GradeLevel  *int            `json:"gradeLevel,omitempty" db:"grade_level" required:"false"`
	Metadata    *map[string]any `json:"metadata,omitempty" db:"metadata" required:"false"`
}

// CurriculumUpdate is the body for updating a curriculum.
type CurriculumUpdate struct {
	Slug        *string         `json:"slug,omitempty" db:"slug" required:"false"`
	Name        *string         `json:"name,omitempty" db:"name" required:"false"`
	Description *string         `json:"description,omitempty" db:"description" required:"false"`
	Approach    *string         `json:"approach,omitempty" db:"approach" enum:"mastery_based,spiral,classical,unit_study,custom" required:"false"`
	SubjectCode *string         `json:"subjectCode,omitempty" db:"subject_code" required:"false"`
	Status      *string         `json:"status,omitempty" db:"status" enum:"draft,published,retired" required:"false"`
	GradeLevel  *int            `json:"gradeLevel,omitempty" db:"grade_level" required:"false"`
	Metadata    *map[string]any `json:"metadata,omitempty" db:"metadata" required:"false"`
}

// CurriculumStandardCreate places a standard in a curriculum.
type CurriculumStandardCreate struct {
	CurriculumID string  `json:"curriculumId" db:"curriculum_id" format:"uuid"`
	StandardID   string  `json:"standardId" db:"standard_id" format:"uuid"`
	Unit         *string `json:"unit,omitempty" db:"unit" required:"false"`
	Position     *int    `json:"position,omitempty" db:"position" required:"false"`
	Notes        *string `json:"notes,omitempty" db:"notes" required:"false"`
}

// CurriculumStandardUpdate updates a curriculum-standard placement.
type CurriculumStandardUpdate struct {
	Unit     *string `json:"unit,omitempty" db:"unit" required:"false"`
	Position *int    `json:"position,omitempty" db:"position" required:"false"`
	Notes    *string `json:"notes,omitempty" db:"notes" required:"false"`
}

// EnrollmentCreate enrolls a student in a curriculum.
type EnrollmentCreate struct {
	StudentID            string     `json:"studentId" db:"student_id" format:"uuid"`
	CurriculumID         string     `json:"curriculumId" db:"curriculum_id" format:"uuid"`
	CurriculumRevisionID *string    `json:"curriculumRevisionId,omitempty" db:"curriculum_revision_id" format:"uuid" required:"false"`
	Status               *string    `json:"status,omitempty" db:"status" enum:"active,paused,completed,withdrawn" required:"false"`
	Priority             *int       `json:"priority,omitempty" db:"priority" required:"false"`
	StartedOn            *time.Time `json:"startedOn,omitempty" db:"started_on" required:"false"`
}

// EnrollmentUpdate updates an enrollment.
type EnrollmentUpdate struct {
	Status               *string    `json:"status,omitempty" db:"status" enum:"active,paused,completed,withdrawn" required:"false"`
	Priority             *int       `json:"priority,omitempty" db:"priority" required:"false"`
	CurriculumRevisionID *string    `json:"curriculumRevisionId,omitempty" db:"curriculum_revision_id" format:"uuid" required:"false"`
	StartedOn            *time.Time `json:"startedOn,omitempty" db:"started_on" required:"false"`
	EndedOn              *time.Time `json:"endedOn,omitempty" db:"ended_on" required:"false"`
}

// MasteryRecordCreate creates a mastery record for a (student, standard).
type MasteryRecordCreate struct {
	StudentID           string     `json:"studentId" db:"student_id" format:"uuid"`
	StandardID          string     `json:"standardId" db:"standard_id" format:"uuid"`
	Status              *string    `json:"status,omitempty" db:"status" enum:"not_introduced,in_progress,approaching,mastered" required:"false"`
	Confidence          *float64   `json:"confidence,omitempty" db:"confidence" minimum:"0" maximum:"1" required:"false"`
	DecayRate           *float64   `json:"decayRate,omitempty" db:"decay_rate" minimum:"0" maximum:"1" required:"false"`
	LastAssessedAt      *time.Time `json:"lastAssessedAt,omitempty" db:"last_assessed_at" required:"false"`
	LastReinforcedAt    *time.Time `json:"lastReinforcedAt,omitempty" db:"last_reinforced_at" required:"false"`
	NextReinforcementAt *time.Time `json:"nextReinforcementAt,omitempty" db:"next_reinforcement_at" required:"false"`
}

// MasteryRecordUpdate updates a mastery record.
type MasteryRecordUpdate struct {
	Status              *string    `json:"status,omitempty" db:"status" enum:"not_introduced,in_progress,approaching,mastered" required:"false"`
	Confidence          *float64   `json:"confidence,omitempty" db:"confidence" minimum:"0" maximum:"1" required:"false"`
	DecayRate           *float64   `json:"decayRate,omitempty" db:"decay_rate" minimum:"0" maximum:"1" required:"false"`
	LastAssessedAt      *time.Time `json:"lastAssessedAt,omitempty" db:"last_assessed_at" required:"false"`
	LastReinforcedAt    *time.Time `json:"lastReinforcedAt,omitempty" db:"last_reinforced_at" required:"false"`
	NextReinforcementAt *time.Time `json:"nextReinforcementAt,omitempty" db:"next_reinforcement_at" required:"false"`
}

// MasteryEvidenceCreate attaches evidence to a mastery record.
type MasteryEvidenceCreate struct {
	MasteryRecordID string          `json:"masteryRecordId" db:"mastery_record_id" format:"uuid"`
	Kind            string          `json:"kind" db:"kind" enum:"continuous,formal,project,portfolio"`
	EvidenceClass   *string         `json:"evidenceClass,omitempty" db:"evidence_class" enum:"procedural_continuous,conceptual_response,parent_attestation,formal_assessment,portfolio" required:"false"`
	Provenance      *map[string]any `json:"provenance,omitempty" db:"provenance" required:"false"`
	PolicyVersion   *int            `json:"policyVersion,omitempty" db:"policy_version" required:"false"`
	OccurredOn      *time.Time      `json:"occurredOn,omitempty" db:"occurred_on" required:"false"`
	Context         *string         `json:"context,omitempty" db:"context" required:"false"`
	SourceRef       *string         `json:"sourceRef,omitempty" db:"source_ref" required:"false"`
}

// MasteryEvidenceUpdate updates evidence.
type MasteryEvidenceUpdate struct {
	Kind          *string         `json:"kind,omitempty" db:"kind" enum:"continuous,formal,project,portfolio" required:"false"`
	EvidenceClass *string         `json:"evidenceClass,omitempty" db:"evidence_class" enum:"procedural_continuous,conceptual_response,parent_attestation,formal_assessment,portfolio" required:"false"`
	Provenance    *map[string]any `json:"provenance,omitempty" db:"provenance" required:"false"`
	PolicyVersion *int            `json:"policyVersion,omitempty" db:"policy_version" required:"false"`
	OccurredOn    *time.Time      `json:"occurredOn,omitempty" db:"occurred_on" required:"false"`
	Context       *string         `json:"context,omitempty" db:"context" required:"false"`
	SourceRef     *string         `json:"sourceRef,omitempty" db:"source_ref" required:"false"`
}

// AssessmentCreate creates an assessment definition.
type AssessmentCreate struct {
	Title        string          `json:"title" db:"title" minLength:"1"`
	Description  *string         `json:"description,omitempty" db:"description" required:"false"`
	Kind         string          `json:"kind" db:"kind" enum:"continuous,quick_check,comprehensive,tcap_practice,quiz,project_rubric"`
	SubjectID    *string         `json:"subjectId,omitempty" db:"subject_id" format:"uuid" required:"false"`
	CurriculumID *string         `json:"curriculumId,omitempty" db:"curriculum_id" format:"uuid" required:"false"`
	GradeLevel   *int            `json:"gradeLevel,omitempty" db:"grade_level" required:"false"`
	Metadata     *map[string]any `json:"metadata,omitempty" db:"metadata" required:"false"`
}

// AssessmentUpdate updates an assessment definition.
type AssessmentUpdate struct {
	Title        *string         `json:"title,omitempty" db:"title" required:"false"`
	Description  *string         `json:"description,omitempty" db:"description" required:"false"`
	Kind         *string         `json:"kind,omitempty" db:"kind" enum:"continuous,quick_check,comprehensive,tcap_practice,quiz,project_rubric" required:"false"`
	SubjectID    *string         `json:"subjectId,omitempty" db:"subject_id" format:"uuid" required:"false"`
	CurriculumID *string         `json:"curriculumId,omitempty" db:"curriculum_id" format:"uuid" required:"false"`
	GradeLevel   *int            `json:"gradeLevel,omitempty" db:"grade_level" required:"false"`
	Metadata     *map[string]any `json:"metadata,omitempty" db:"metadata" required:"false"`
}

// AssessmentItemCreate creates an assessment item.
type AssessmentItemCreate struct {
	AssessmentID string          `json:"assessmentId" db:"assessment_id" format:"uuid"`
	StandardID   *string         `json:"standardId,omitempty" db:"standard_id" format:"uuid" required:"false"`
	Position     *int            `json:"position,omitempty" db:"position" required:"false"`
	ItemType     string          `json:"itemType" db:"item_type" enum:"mc,multi_select,equation_editor,constructed_response,matching,short_answer,true_false"`
	Difficulty   *string         `json:"difficulty,omitempty" db:"difficulty" enum:"approaching,on_track,mastered" required:"false"`
	Stem         string          `json:"stem" db:"stem" minLength:"1"`
	Rationale    *string         `json:"rationale,omitempty" db:"rationale" required:"false"`
	Points       *float64        `json:"points,omitempty" db:"points" minimum:"0" required:"false"`
	Metadata     *map[string]any `json:"metadata,omitempty" db:"metadata" required:"false"`
}

// AssessmentItemUpdate updates an assessment item.
type AssessmentItemUpdate struct {
	StandardID *string         `json:"standardId,omitempty" db:"standard_id" format:"uuid" required:"false"`
	Position   *int            `json:"position,omitempty" db:"position" required:"false"`
	ItemType   *string         `json:"itemType,omitempty" db:"item_type" enum:"mc,multi_select,equation_editor,constructed_response,matching,short_answer,true_false" required:"false"`
	Difficulty *string         `json:"difficulty,omitempty" db:"difficulty" enum:"approaching,on_track,mastered" required:"false"`
	Stem       *string         `json:"stem,omitempty" db:"stem" required:"false"`
	Rationale  *string         `json:"rationale,omitempty" db:"rationale" required:"false"`
	Points     *float64        `json:"points,omitempty" db:"points" minimum:"0" required:"false"`
	Metadata   *map[string]any `json:"metadata,omitempty" db:"metadata" required:"false"`
}

// AssessmentItemOptionCreate creates an option for a choice-type item.
type AssessmentItemOptionCreate struct {
	ItemID   string  `json:"itemId" db:"item_id" format:"uuid"`
	Position *int    `json:"position,omitempty" db:"position" required:"false"`
	Text     string  `json:"text" db:"text" minLength:"1"`
	Correct  *bool   `json:"correct,omitempty" db:"correct" required:"false"`
	Feedback *string `json:"feedback,omitempty" db:"feedback" required:"false"`
}

// AssessmentItemOptionUpdate updates an item option.
type AssessmentItemOptionUpdate struct {
	Position *int    `json:"position,omitempty" db:"position" required:"false"`
	Text     *string `json:"text,omitempty" db:"text" required:"false"`
	Correct  *bool   `json:"correct,omitempty" db:"correct" required:"false"`
	Feedback *string `json:"feedback,omitempty" db:"feedback" required:"false"`
}

// AssessmentAttemptCreate starts an attempt.
type AssessmentAttemptCreate struct {
	AssessmentID string   `json:"assessmentId" db:"assessment_id" format:"uuid"`
	StudentID    string   `json:"studentId" db:"student_id" format:"uuid"`
	Status       *string  `json:"status,omitempty" db:"status" enum:"in_progress,submitted,scored" required:"false"`
	MaxScore     *float64 `json:"maxScore,omitempty" db:"max_score" minimum:"0" required:"false"`
}

// AssessmentAttemptUpdate updates an attempt (submit / score).
type AssessmentAttemptUpdate struct {
	Status      *string    `json:"status,omitempty" db:"status" enum:"in_progress,submitted,scored" required:"false"`
	Score       *float64   `json:"score,omitempty" db:"score" minimum:"0" required:"false"`
	MaxScore    *float64   `json:"maxScore,omitempty" db:"max_score" minimum:"0" required:"false"`
	SubmittedAt *time.Time `json:"submittedAt,omitempty" db:"submitted_at" required:"false"`
	ScoredAt    *time.Time `json:"scoredAt,omitempty" db:"scored_at" required:"false"`
}

// ItemResponseCreate records a response to an item within an attempt.
type ItemResponseCreate struct {
	AttemptID     string          `json:"attemptId" db:"attempt_id" format:"uuid"`
	ItemID        string          `json:"itemId" db:"item_id" format:"uuid"`
	Response      *map[string]any `json:"response,omitempty" db:"response" required:"false"`
	IsCorrect     *bool           `json:"isCorrect,omitempty" db:"is_correct" required:"false"`
	PointsAwarded *float64        `json:"pointsAwarded,omitempty" db:"points_awarded" minimum:"0" required:"false"`
	Feedback      *string         `json:"feedback,omitempty" db:"feedback" required:"false"`
}

// ItemResponseUpdate updates a recorded response (e.g. scoring).
type ItemResponseUpdate struct {
	Response      *map[string]any `json:"response,omitempty" db:"response" required:"false"`
	IsCorrect     *bool           `json:"isCorrect,omitempty" db:"is_correct" required:"false"`
	PointsAwarded *float64        `json:"pointsAwarded,omitempty" db:"points_awarded" minimum:"0" required:"false"`
	Feedback      *string         `json:"feedback,omitempty" db:"feedback" required:"false"`
}

// registerAll wires every resource's CRUD endpoints into the API.
func registerAll(h huma.API, q repo.Querier, opts Options) {
	opts = normalizeOptions(opts)
	RegisterCRUD[domain.Educator, EducatorCreate, EducatorUpdate](h, q, repo.Educators, "educator", "educators", "/educators")
	RegisterCRUD[domain.Student, StudentCreate, StudentUpdate](h, q, repo.Students, "student", "students", "/students")
	RegisterCRUD[domain.Subject, SubjectCreate, SubjectUpdate](h, q, repo.Subjects, "subject", "subjects", "/subjects")
	RegisterCRUD[domain.Standard, StandardCreate, StandardUpdate](h, q, repo.Standards, "standard", "standards", "/standards")
	RegisterCRUD[domain.Curriculum, CurriculumCreate, CurriculumUpdate](h, q, repo.Curricula, "curriculum", "curricula", "/curricula")
	RegisterCRUD[domain.CurriculumStandard, CurriculumStandardCreate, CurriculumStandardUpdate](h, q, repo.CurriculumStandards, "curriculum-standard", "curriculum-standards", "/curriculum-standards")
	RegisterCRUD[domain.Enrollment, EnrollmentCreate, EnrollmentUpdate](h, q, repo.Enrollments, "enrollment", "enrollments", "/enrollments")
	RegisterCRUD[domain.MasteryRecord, MasteryRecordCreate, MasteryRecordUpdate](h, q, repo.MasteryRecords, "mastery-record", "mastery-records", "/mastery-records")
	RegisterCRUD[domain.MasteryEvidence, MasteryEvidenceCreate, MasteryEvidenceUpdate](h, q, repo.MasteryEvidences, "mastery-evidence", "mastery-evidences", "/mastery-evidence")
	RegisterCRUD[domain.Assessment, AssessmentCreate, AssessmentUpdate](h, q, repo.Assessments, "assessment", "assessments", "/assessments")
	RegisterCRUD[domain.AssessmentItem, AssessmentItemCreate, AssessmentItemUpdate](h, q, repo.AssessmentItems, "assessment-item", "assessment-items", "/assessment-items")
	RegisterCRUD[domain.AssessmentItemOption, AssessmentItemOptionCreate, AssessmentItemOptionUpdate](h, q, repo.AssessmentItemOptions, "assessment-item-option", "assessment-item-options", "/assessment-item-options")
	RegisterCRUD[domain.AssessmentAttempt, AssessmentAttemptCreate, AssessmentAttemptUpdate](h, q, repo.AssessmentAttempts, "assessment-attempt", "assessment-attempts", "/assessment-attempts")
	RegisterCRUD[domain.ItemResponse, ItemResponseCreate, ItemResponseUpdate](h, q, repo.ItemResponses, "item-response", "item-responses", "/item-responses")
	registerInstructionLogs(h, q, opts)
	registerParentAuth(h, q)
	registerParentLearning(h, q, opts)
	registerParentCourse(h, q)
	registerStudentAPI(h, q, opts)
}

func normalizeOptions(opts Options) Options {
	if opts.Tutor == nil {
		svc, err := tutor.NewFromConfig(tutor.DefaultConfig())
		if err != nil {
			// DefaultConfig must always succeed; panic would hide wiring bugs in tests.
			opts.Tutor = tutor.NewFake()
			opts.TutorProviderName = "fake"
			opts.TutorEnabled = true
			return opts
		}
		opts.Tutor = svc
		if opts.TutorProviderName == "" {
			opts.TutorProviderName = svc.ProviderName()
		}
		opts.TutorEnabled = svc.Enabled()
		return opts
	}
	if opts.TutorProviderName == "" {
		if ps, ok := opts.Tutor.(*tutor.PolicyService); ok {
			opts.TutorProviderName = ps.ProviderName()
			opts.TutorEnabled = ps.Enabled()
		} else {
			opts.TutorProviderName = "fake"
			opts.TutorEnabled = true
		}
	}
	return opts
}
