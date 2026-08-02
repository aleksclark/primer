// Package domain holds the core entity types persisted by the LMS.
//
// JSON tags are camelCase so entities surface cleanly through the
// Huma-generated OpenAPI spec and the TypeScript client generated from it.
// The db tags map columns for pgx struct scanning.
package domain

import "time"

// Educator is a parent or administrator who manages the system.
type Educator struct {
	ID        string    `json:"id" db:"id" format:"uuid"`
	Email     string    `json:"email" db:"email" format:"email"`
	Name      string    `json:"name" db:"name"`
	Role      string    `json:"role" db:"role" enum:"parent,admin,tutor"`
	CreatedAt time.Time `json:"createdAt" db:"created_at"`
	UpdatedAt time.Time `json:"updatedAt" db:"updated_at"`
}

// Student is a learner tracked by the system.
type Student struct {
	ID         string     `json:"id" db:"id" format:"uuid"`
	FirstName  string     `json:"firstName" db:"first_name"`
	LastName   string     `json:"lastName" db:"last_name"`
	Birthdate  *time.Time `json:"birthdate,omitempty" db:"birthdate"`
	GradeLevel *int       `json:"gradeLevel,omitempty" db:"grade_level"`
	Notes      string     `json:"notes" db:"notes"`
	CreatedAt  time.Time  `json:"createdAt" db:"created_at"`
	UpdatedAt  time.Time  `json:"updatedAt" db:"updated_at"`
}

// Subject is a top-level domain such as math or ela.
type Subject struct {
	ID          string    `json:"id" db:"id" format:"uuid"`
	Code        string    `json:"code" db:"code"`
	Name        string    `json:"name" db:"name"`
	Description string    `json:"description" db:"description"`
	CreatedAt   time.Time `json:"createdAt" db:"created_at"`
	UpdatedAt   time.Time `json:"updatedAt" db:"updated_at"`
}

// Standard is an individual learning standard, hierarchical and multi-source.
type Standard struct {
	ID          string         `json:"id" db:"id" format:"uuid"`
	Source      string         `json:"source" db:"source" enum:"tennessee,common_core,custom"`
	SubjectID   *string        `json:"subjectId,omitempty" db:"subject_id" format:"uuid"`
	ParentID    *string        `json:"parentId,omitempty" db:"parent_id" format:"uuid"`
	Code        string         `json:"code" db:"code"`
	GradeLevel  *int           `json:"gradeLevel,omitempty" db:"grade_level"`
	Domain      string         `json:"domain" db:"domain"`
	Cluster     string         `json:"cluster" db:"cluster"`
	Description string         `json:"description" db:"description"`
	TCAPWeight  string         `json:"tcapWeight" db:"tcap_weight"`
	Metadata    map[string]any `json:"metadata,omitempty" db:"metadata"`
	CreatedAt   time.Time      `json:"createdAt" db:"created_at"`
	UpdatedAt   time.Time      `json:"updatedAt" db:"updated_at"`
}

// Curriculum is a curriculum approach: a coherent way of organizing standards.
type Curriculum struct {
	ID          string         `json:"id" db:"id" format:"uuid"`
	Name        string         `json:"name" db:"name"`
	Description string         `json:"description" db:"description"`
	Approach    string         `json:"approach" db:"approach" enum:"mastery_based,spiral,classical,unit_study,custom"`
	GradeLevel  *int           `json:"gradeLevel,omitempty" db:"grade_level"`
	Metadata    map[string]any `json:"metadata,omitempty" db:"metadata"`
	CreatedAt   time.Time      `json:"createdAt" db:"created_at"`
	UpdatedAt   time.Time      `json:"updatedAt" db:"updated_at"`
}

// CurriculumStandard places a standard within a curriculum's sequence.
type CurriculumStandard struct {
	ID           string    `json:"id" db:"id" format:"uuid"`
	CurriculumID string    `json:"curriculumId" db:"curriculum_id" format:"uuid"`
	StandardID   string    `json:"standardId" db:"standard_id" format:"uuid"`
	Unit         string    `json:"unit" db:"unit"`
	Position     int       `json:"position" db:"position"`
	Notes        string    `json:"notes" db:"notes"`
	CreatedAt    time.Time `json:"createdAt" db:"created_at"`
	UpdatedAt    time.Time `json:"updatedAt" db:"updated_at"`
}

// Enrollment ties a student to a curriculum.
type Enrollment struct {
	ID           string     `json:"id" db:"id" format:"uuid"`
	StudentID    string     `json:"studentId" db:"student_id" format:"uuid"`
	CurriculumID string     `json:"curriculumId" db:"curriculum_id" format:"uuid"`
	Status       string     `json:"status" db:"status" enum:"active,paused,completed,withdrawn"`
	StartedOn    time.Time  `json:"startedOn" db:"started_on"`
	EndedOn      *time.Time `json:"endedOn,omitempty" db:"ended_on"`
	CreatedAt    time.Time  `json:"createdAt" db:"created_at"`
	UpdatedAt    time.Time  `json:"updatedAt" db:"updated_at"`
}

// MasteryRecord captures a student's mastery of a single standard.
type MasteryRecord struct {
	ID                  string     `json:"id" db:"id" format:"uuid"`
	StudentID           string     `json:"studentId" db:"student_id" format:"uuid"`
	StandardID          string     `json:"standardId" db:"standard_id" format:"uuid"`
	Status              string     `json:"status" db:"status" enum:"not_introduced,in_progress,approaching,mastered"`
	Confidence          float64    `json:"confidence" db:"confidence" minimum:"0" maximum:"1"`
	DecayRate           float64    `json:"decayRate" db:"decay_rate"`
	LastAssessedAt      *time.Time `json:"lastAssessedAt,omitempty" db:"last_assessed_at"`
	LastReinforcedAt    *time.Time `json:"lastReinforcedAt,omitempty" db:"last_reinforced_at"`
	NextReinforcementAt *time.Time `json:"nextReinforcementAt,omitempty" db:"next_reinforcement_at"`
	CreatedAt           time.Time  `json:"createdAt" db:"created_at"`
	UpdatedAt           time.Time  `json:"updatedAt" db:"updated_at"`
}

// MasteryEvidence is a single piece of evidence backing a mastery record.
type MasteryEvidence struct {
	ID              string    `json:"id" db:"id" format:"uuid"`
	MasteryRecordID string    `json:"masteryRecordId" db:"mastery_record_id" format:"uuid"`
	Kind            string    `json:"kind" db:"kind" enum:"continuous,formal,project,portfolio"`
	OccurredOn      time.Time `json:"occurredOn" db:"occurred_on"`
	Context         string    `json:"context" db:"context"`
	SourceRef       string    `json:"sourceRef" db:"source_ref"`
	CreatedAt       time.Time `json:"createdAt" db:"created_at"`
	UpdatedAt       time.Time `json:"updatedAt" db:"updated_at"`
}

// Instruction log sources. Machine producers carry a stable SourceRef so a
// retry cannot count the same event twice; hand-entered rows leave it empty.
const (
	InstructionSourceTV     = "tv"
	InstructionSourceManual = "manual"
)

// Instruction log classes. Only educational and mixed viewing is instructional
// time; pure entertainment is not logged here at all.
const (
	InstructionClassEducational = "educational"
	InstructionClassMixed       = "mixed"
)

// InstructionLog is instructional time earned outside the LMS proper — today,
// educational viewing pushed here by the TV server. Subject tags and standard
// codes travel with the log so the hours land under the right subject and can
// later be joined to mastery evidence by standard code.
type InstructionLog struct {
	ID             string    `json:"id" db:"id" format:"uuid"`
	Source         string    `json:"source" db:"source" enum:"tv,manual"`
	SourceRef      string    `json:"sourceRef" db:"source_ref" doc:"The producer's stable identifier for the event; empty for hand-entered rows."`
	StudentID      *string   `json:"studentId,omitempty" db:"student_id" format:"uuid"`
	MediaTitle     string    `json:"mediaTitle" db:"media_title"`
	Class          string    `json:"class" db:"class" enum:"educational,mixed"`
	SubjectTags    []string  `json:"subjectTags" db:"subject_tags"`
	StandardCodes  []string  `json:"standardCodes" db:"standard_codes"`
	WatchedSeconds int       `json:"watchedSeconds" db:"watched_seconds"`
	OccurredOn     time.Time `json:"occurredOn" db:"occurred_on"`
	Notes          string    `json:"notes" db:"notes"`
	CreatedAt      time.Time `json:"createdAt" db:"created_at"`
	UpdatedAt      time.Time `json:"updatedAt" db:"updated_at"`
}

// Assessment is a definition of an assessment of any supported kind.
type Assessment struct {
	ID           string         `json:"id" db:"id" format:"uuid"`
	Title        string         `json:"title" db:"title"`
	Description  string         `json:"description" db:"description"`
	Kind         string         `json:"kind" db:"kind" enum:"continuous,quick_check,comprehensive,tcap_practice,quiz,project_rubric"`
	SubjectID    *string        `json:"subjectId,omitempty" db:"subject_id" format:"uuid"`
	CurriculumID *string        `json:"curriculumId,omitempty" db:"curriculum_id" format:"uuid"`
	GradeLevel   *int           `json:"gradeLevel,omitempty" db:"grade_level"`
	Metadata     map[string]any `json:"metadata,omitempty" db:"metadata"`
	CreatedAt    time.Time      `json:"createdAt" db:"created_at"`
	UpdatedAt    time.Time      `json:"updatedAt" db:"updated_at"`
}

// AssessmentItem is a single question within an assessment.
type AssessmentItem struct {
	ID           string         `json:"id" db:"id" format:"uuid"`
	AssessmentID string         `json:"assessmentId" db:"assessment_id" format:"uuid"`
	StandardID   *string        `json:"standardId,omitempty" db:"standard_id" format:"uuid"`
	Position     int            `json:"position" db:"position"`
	ItemType     string         `json:"itemType" db:"item_type" enum:"mc,multi_select,equation_editor,constructed_response,matching,short_answer,true_false"`
	Difficulty   string         `json:"difficulty" db:"difficulty" enum:"approaching,on_track,mastered"`
	Stem         string         `json:"stem" db:"stem"`
	Rationale    string         `json:"rationale" db:"rationale"`
	Points       float64        `json:"points" db:"points"`
	Metadata     map[string]any `json:"metadata,omitempty" db:"metadata"`
	CreatedAt    time.Time      `json:"createdAt" db:"created_at"`
	UpdatedAt    time.Time      `json:"updatedAt" db:"updated_at"`
}

// AssessmentItemOption is a selectable option for choice-type items.
type AssessmentItemOption struct {
	ID        string    `json:"id" db:"id" format:"uuid"`
	ItemID    string    `json:"itemId" db:"item_id" format:"uuid"`
	Position  int       `json:"position" db:"position"`
	Text      string    `json:"text" db:"text"`
	Correct   bool      `json:"correct" db:"correct"`
	Feedback  string    `json:"feedback" db:"feedback"`
	CreatedAt time.Time `json:"createdAt" db:"created_at"`
	UpdatedAt time.Time `json:"updatedAt" db:"updated_at"`
}

// AssessmentAttempt is a student's attempt at an assessment.
type AssessmentAttempt struct {
	ID           string     `json:"id" db:"id" format:"uuid"`
	AssessmentID string     `json:"assessmentId" db:"assessment_id" format:"uuid"`
	StudentID    string     `json:"studentId" db:"student_id" format:"uuid"`
	Status       string     `json:"status" db:"status" enum:"in_progress,submitted,scored"`
	Score        *float64   `json:"score,omitempty" db:"score"`
	MaxScore     *float64   `json:"maxScore,omitempty" db:"max_score"`
	StartedAt    time.Time  `json:"startedAt" db:"started_at"`
	SubmittedAt  *time.Time `json:"submittedAt,omitempty" db:"submitted_at"`
	ScoredAt     *time.Time `json:"scoredAt,omitempty" db:"scored_at"`
	CreatedAt    time.Time  `json:"createdAt" db:"created_at"`
	UpdatedAt    time.Time  `json:"updatedAt" db:"updated_at"`
}

// ItemResponse is a student's response to a single item within an attempt.
type ItemResponse struct {
	ID            string         `json:"id" db:"id" format:"uuid"`
	AttemptID     string         `json:"attemptId" db:"attempt_id" format:"uuid"`
	ItemID        string         `json:"itemId" db:"item_id" format:"uuid"`
	Response      map[string]any `json:"response,omitempty" db:"response"`
	IsCorrect     *bool          `json:"isCorrect,omitempty" db:"is_correct"`
	PointsAwarded *float64       `json:"pointsAwarded,omitempty" db:"points_awarded"`
	Feedback      string         `json:"feedback" db:"feedback"`
	CreatedAt     time.Time      `json:"createdAt" db:"created_at"`
	UpdatedAt     time.Time      `json:"updatedAt" db:"updated_at"`
}
