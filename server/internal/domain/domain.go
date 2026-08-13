// Package domain holds the core entity types persisted by the LMS.
//
// JSON tags are camelCase so entities surface cleanly through the
// Huma-generated OpenAPI spec and the TypeScript client generated from it.
// The db tags map columns for pgx struct scanning.
package domain

import "time"

// Educator is a parent or administrator who manages the system.
// PasswordHash is never serialized; it is loaded only for authentication.
type Educator struct {
	ID           string    `json:"id" db:"id" format:"uuid"`
	Email        string    `json:"email" db:"email" format:"email"`
	Name         string    `json:"name" db:"name"`
	Role         string    `json:"role" db:"role" enum:"parent,admin,tutor"`
	PasswordHash string    `json:"-" db:"password_hash"`
	CreatedAt    time.Time `json:"createdAt" db:"created_at"`
	UpdatedAt    time.Time `json:"updatedAt" db:"updated_at"`
}

// ParentSession is a server-side parent/admin login session.
type ParentSession struct {
	ID         string    `json:"id" db:"id" format:"uuid"`
	EducatorID string    `json:"educatorId" db:"educator_id" format:"uuid"`
	TokenHash  string    `json:"-" db:"token_hash"`
	ExpiresAt  time.Time `json:"expiresAt" db:"expires_at"`
	CreatedAt  time.Time `json:"createdAt" db:"created_at"`
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
// Stable identity for course enrollments; immutable content lives on revisions.
type Curriculum struct {
	ID          string         `json:"id" db:"id" format:"uuid"`
	Slug        string         `json:"slug" db:"slug"`
	Name        string         `json:"name" db:"name"`
	Description string         `json:"description" db:"description"`
	Approach    string         `json:"approach" db:"approach" enum:"mastery_based,spiral,classical,unit_study,custom"`
	SubjectCode string         `json:"subjectCode" db:"subject_code"`
	Status      string         `json:"status" db:"status" enum:"draft,published,retired"`
	GradeLevel  *int           `json:"gradeLevel,omitempty" db:"grade_level"`
	Metadata    map[string]any `json:"metadata,omitempty" db:"metadata"`
	CreatedAt   time.Time      `json:"createdAt" db:"created_at"`
	UpdatedAt   time.Time      `json:"updatedAt" db:"updated_at"`
}

// CurriculumRevision is an immutable published course snapshot.
type CurriculumRevision struct {
	ID             string         `json:"id" db:"id" format:"uuid"`
	CurriculumID   string         `json:"curriculumId" db:"curriculum_id" format:"uuid"`
	Revision       int            `json:"revision" db:"revision"`
	Title          string         `json:"title" db:"title"`
	Description    string         `json:"description" db:"description"`
	SubjectCode    string         `json:"subjectCode" db:"subject_code"`
	Version        string         `json:"version" db:"version"`
	RevisionPolicy string         `json:"revisionPolicy" db:"revision_policy" enum:"latest_published,pinned_digest"`
	Document       map[string]any `json:"document,omitempty" db:"document"`
	PublishedAt    *time.Time     `json:"publishedAt,omitempty" db:"published_at"`
	CreatedAt      time.Time      `json:"createdAt" db:"created_at"`
}

// CurriculumActivity is one ordered activity membership entry in a revision.
type CurriculumActivity struct {
	ID                   string         `json:"id" db:"id" format:"uuid"`
	CurriculumRevisionID string         `json:"curriculumRevisionId" db:"curriculum_revision_id" format:"uuid"`
	Position             int            `json:"position" db:"position"`
	ActivitySlug         string         `json:"activitySlug" db:"activity_slug"`
	ActivityRevisionID   *string        `json:"activityRevisionId,omitempty" db:"activity_revision_id" format:"uuid"`
	Module               string         `json:"module,omitempty" db:"module"`
	Capstone             bool           `json:"capstone" db:"capstone"`
	ContinuityMode       string         `json:"continuityMode,omitempty" db:"continuity_mode"`
	Metadata             map[string]any `json:"metadata,omitempty" db:"metadata"`
	CreatedAt            time.Time      `json:"createdAt" db:"created_at"`
}

// CurriculumActivityPrerequisite is a directed prerequisite edge.
type CurriculumActivityPrerequisite struct {
	ID                   string    `json:"id" db:"id" format:"uuid"`
	CurriculumRevisionID string    `json:"curriculumRevisionId" db:"curriculum_revision_id" format:"uuid"`
	ActivitySlug         string    `json:"activitySlug" db:"activity_slug"`
	RequiresSlug         string    `json:"requiresSlug" db:"requires_slug"`
	Requirement          string    `json:"requirement" db:"requirement" enum:"completed,approaching,mastered"`
	Description          string    `json:"description,omitempty" db:"description"`
	CreatedAt            time.Time `json:"createdAt" db:"created_at"`
}

// CurriculumActivityGate is an evidence or parent-review gate.
type CurriculumActivityGate struct {
	ID                   string    `json:"id" db:"id" format:"uuid"`
	CurriculumRevisionID string    `json:"curriculumRevisionId" db:"curriculum_revision_id" format:"uuid"`
	ActivitySlug         string    `json:"activitySlug" db:"activity_slug"`
	Kind                 string    `json:"kind" db:"kind" enum:"evidence,parent_review"`
	Standards            []string  `json:"standards,omitempty" db:"standards"`
	Description          string    `json:"description,omitempty" db:"description"`
	CreatedAt            time.Time `json:"createdAt" db:"created_at"`
}

// CurriculumActivityRemediation is a remediation/reinforcement branch placeholder.
type CurriculumActivityRemediation struct {
	ID                   string    `json:"id" db:"id" format:"uuid"`
	CurriculumRevisionID string    `json:"curriculumRevisionId" db:"curriculum_revision_id" format:"uuid"`
	ForActivitySlug      string    `json:"forActivitySlug" db:"for_activity_slug"`
	BranchSlug           string    `json:"branchSlug" db:"branch_slug"`
	Kind                 string    `json:"kind" db:"kind" enum:"remediation,reinforcement"`
	Description          string    `json:"description,omitempty" db:"description"`
	CreatedAt            time.Time `json:"createdAt" db:"created_at"`
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

// Enrollment ties a student to a curriculum revision (course).
type Enrollment struct {
	ID                   string         `json:"id" db:"id" format:"uuid"`
	StudentID            string         `json:"studentId" db:"student_id" format:"uuid"`
	CurriculumID         string         `json:"curriculumId" db:"curriculum_id" format:"uuid"`
	CurriculumRevisionID *string        `json:"curriculumRevisionId,omitempty" db:"curriculum_revision_id" format:"uuid"`
	Status               string         `json:"status" db:"status" enum:"active,paused,completed,withdrawn"`
	Priority             int            `json:"priority" db:"priority"`
	PinnedActivitySlug   string         `json:"pinnedActivitySlug,omitempty" db:"pinned_activity_slug"`
	PinnedReason         string         `json:"pinnedReason,omitempty" db:"pinned_reason"`
	PinnedBy             *string        `json:"pinnedBy,omitempty" db:"pinned_by" format:"uuid"`
	PinnedAt             *time.Time     `json:"pinnedAt,omitempty" db:"pinned_at"`
	OverrideSlugs        []string       `json:"overrideSlugs,omitempty" db:"override_slugs"`
	OverrideReason       string         `json:"overrideReason,omitempty" db:"override_reason"`
	BlockingReasons      []any          `json:"blockingReasons,omitempty" db:"blocking_reasons"`
	Constraints          map[string]any `json:"constraints,omitempty" db:"constraints"`
	StartedOn            time.Time      `json:"startedOn" db:"started_on"`
	EndedOn              *time.Time     `json:"endedOn,omitempty" db:"ended_on"`
	CreatedAt            time.Time      `json:"createdAt" db:"created_at"`
	UpdatedAt            time.Time      `json:"updatedAt" db:"updated_at"`
}

// EnrollmentAuditEvent records parent pin/override/status changes.
type EnrollmentAuditEvent struct {
	ID           string         `json:"id" db:"id" format:"uuid"`
	EnrollmentID string         `json:"enrollmentId" db:"enrollment_id" format:"uuid"`
	EducatorID   *string        `json:"educatorId,omitempty" db:"educator_id" format:"uuid"`
	Action       string         `json:"action" db:"action"`
	Reason       string         `json:"reason,omitempty" db:"reason"`
	Detail       map[string]any `json:"detail,omitempty" db:"detail"`
	CreatedAt    time.Time      `json:"createdAt" db:"created_at"`
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

// Evidence class constants for mastery evidence taxonomy.
const (
	EvidenceProceduralContinuous = "procedural_continuous"
	EvidenceConceptualResponse   = "conceptual_response"
	EvidenceParentAttestation    = "parent_attestation"
	EvidenceFormalAssessment     = "formal_assessment"
	EvidencePortfolio            = "portfolio"
)

// MasteryEvidence is a single piece of evidence backing a mastery record.
type MasteryEvidence struct {
	ID              string         `json:"id" db:"id" format:"uuid"`
	MasteryRecordID string         `json:"masteryRecordId" db:"mastery_record_id" format:"uuid"`
	Kind            string         `json:"kind" db:"kind" enum:"continuous,formal,project,portfolio"`
	EvidenceClass   string         `json:"evidenceClass" db:"evidence_class" enum:"procedural_continuous,conceptual_response,parent_attestation,formal_assessment,portfolio"`
	Provenance      map[string]any `json:"provenance,omitempty" db:"provenance"`
	PolicyVersion   int            `json:"policyVersion" db:"policy_version"`
	MigrationNote   string         `json:"migrationNote,omitempty" db:"migration_note"`
	OccurredOn      time.Time      `json:"occurredOn" db:"occurred_on"`
	Context         string         `json:"context" db:"context"`
	SourceRef       string         `json:"sourceRef" db:"source_ref"`
	CreatedAt       time.Time      `json:"createdAt" db:"created_at"`
	UpdatedAt       time.Time      `json:"updatedAt" db:"updated_at"`
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

// Activity status values.
const (
	ActivityStatusDraft     = "draft"
	ActivityStatusPublished = "published"
	ActivityStatusRetired   = "retired"
)

// Assignment state values.
const (
	AssignmentAvailable  = "available"
	AssignmentInProgress = "in_progress"
	AssignmentCompleted  = "completed"
	AssignmentCancelled  = "cancelled"
)

// Learning session state values.
const (
	SessionStarted   = "started"
	SessionPaused    = "paused"
	SessionCompleted = "completed"
	SessionAbandoned = "abandoned"
)

// LearningActivity is an authoring identity that survives revisions.
type LearningActivity struct {
	ID        string    `json:"id" db:"id" format:"uuid"`
	Slug      string    `json:"slug" db:"slug"`
	Title     string    `json:"title" db:"title"`
	Summary   string    `json:"summary" db:"summary"`
	Kind      string    `json:"kind" db:"kind" enum:"terminal,typing"`
	SubjectID *string   `json:"subjectId,omitempty" db:"subject_id" format:"uuid"`
	Status    string    `json:"status" db:"status" enum:"draft,published,retired"`
	CreatedAt time.Time `json:"createdAt" db:"created_at"`
	UpdatedAt time.Time `json:"updatedAt" db:"updated_at"`
}

// LearningActivityRevision is an immutable activity payload used for sessions.
type LearningActivityRevision struct {
	ID            string         `json:"id" db:"id" format:"uuid"`
	ActivityID    string         `json:"activityId" db:"activity_id" format:"uuid"`
	Revision      int            `json:"revision" db:"revision"`
	SchemaVersion string         `json:"schemaVersion" db:"schema_version"`
	Content       map[string]any `json:"content" db:"content"`
	ContentSHA256 string         `json:"contentSha256" db:"content_sha256"`
	PublishedAt   *time.Time     `json:"publishedAt,omitempty" db:"published_at"`
	CreatedAt     time.Time      `json:"createdAt" db:"created_at"`
}

// LearningActivityRevisionStandard links a revision to a standard.
type LearningActivityRevisionStandard struct {
	ID                 string         `json:"id" db:"id" format:"uuid"`
	ActivityRevisionID string         `json:"activityRevisionId" db:"activity_revision_id" format:"uuid"`
	StandardID         string         `json:"standardId" db:"standard_id" format:"uuid"`
	Role               string         `json:"role" db:"role" enum:"primary,reinforcement"`
	Weight             float64        `json:"weight" db:"weight"`
	MasteryCriterion   string         `json:"masteryCriterion" db:"mastery_criterion"`
	EvidencePolicy     map[string]any `json:"evidencePolicy,omitempty" db:"evidence_policy"`
	CreatedAt          time.Time      `json:"createdAt" db:"created_at"`
}

// StudentAssignment is a concrete work item for a student.
type StudentAssignment struct {
	ID                   string         `json:"id" db:"id" format:"uuid"`
	StudentID            string         `json:"studentId" db:"student_id" format:"uuid"`
	ActivityRevisionID   string         `json:"activityRevisionId" db:"activity_revision_id" format:"uuid"`
	EnrollmentID         *string        `json:"enrollmentId,omitempty" db:"enrollment_id" format:"uuid"`
	CurriculumActivityID *string        `json:"curriculumActivityId,omitempty" db:"curriculum_activity_id" format:"uuid"`
	SelectionReason      string         `json:"selectionReason,omitempty" db:"selection_reason"`
	State                string         `json:"state" db:"state" enum:"available,in_progress,completed,cancelled"`
	Priority             int            `json:"priority" db:"priority"`
	AvailableAt          time.Time      `json:"availableAt" db:"available_at"`
	DueAt                *time.Time     `json:"dueAt,omitempty" db:"due_at"`
	AssignedBy           *string        `json:"assignedBy,omitempty" db:"assigned_by" format:"uuid"`
	Reason               string         `json:"reason" db:"reason"`
	Constraints          map[string]any `json:"constraints,omitempty" db:"constraints"`
	CreatedAt            time.Time      `json:"createdAt" db:"created_at"`
	UpdatedAt            time.Time      `json:"updatedAt" db:"updated_at"`
}

// StudentDevice is a paired student workstation.
type StudentDevice struct {
	ID                      string         `json:"id" db:"id" format:"uuid"`
	StudentID               string         `json:"studentId" db:"student_id" format:"uuid"`
	Name                    string         `json:"name" db:"name"`
	TokenHash               string         `json:"-" db:"token_hash"`
	LastSeenAt              *time.Time     `json:"lastSeenAt,omitempty" db:"last_seen_at"`
	RevokedAt               *time.Time     `json:"revokedAt,omitempty" db:"revoked_at"`
	// Capabilities is the most recent device capability report (runtime profiles, runner flags).
	// Diagnostic only — not an authorization boundary.
	Capabilities            map[string]any `json:"capabilities,omitempty" db:"capabilities"`
	CapabilitiesReportedAt  *time.Time     `json:"capabilitiesReportedAt,omitempty" db:"capabilities_reported_at"`
	CreatedAt               time.Time      `json:"createdAt" db:"created_at"`
	UpdatedAt               time.Time      `json:"updatedAt" db:"updated_at"`
}

// StudentDevicePairingCode is a short-lived one-use pairing code.
type StudentDevicePairingCode struct {
	ID        string     `json:"id" db:"id" format:"uuid"`
	StudentID string     `json:"studentId" db:"student_id" format:"uuid"`
	CodeHash  string     `json:"-" db:"code_hash"`
	CreatedBy *string    `json:"createdBy,omitempty" db:"created_by" format:"uuid"`
	ExpiresAt time.Time  `json:"expiresAt" db:"expires_at"`
	UsedAt    *time.Time `json:"usedAt,omitempty" db:"used_at"`
	CreatedAt time.Time  `json:"createdAt" db:"created_at"`
}

// LearningSession is one attempt at an assignment on a device.
type LearningSession struct {
	ID                 string     `json:"id" db:"id" format:"uuid"`
	AssignmentID       string     `json:"assignmentId" db:"assignment_id" format:"uuid"`
	StudentID          string     `json:"studentId" db:"student_id" format:"uuid"`
	DeviceID           string     `json:"deviceId" db:"device_id" format:"uuid"`
	ClientSessionID    string     `json:"clientSessionId" db:"client_session_id"`
	ActivityRevisionID string     `json:"activityRevisionId" db:"activity_revision_id" format:"uuid"`
	State              string     `json:"state" db:"state" enum:"started,paused,completed,abandoned"`
	StartedAt          time.Time  `json:"startedAt" db:"started_at"`
	LastEventAt        *time.Time `json:"lastEventAt,omitempty" db:"last_event_at"`
	CompletedAt        *time.Time `json:"completedAt,omitempty" db:"completed_at"`
	DurationSeconds    int        `json:"durationSeconds" db:"duration_seconds"`
	Summary            string     `json:"summary" db:"summary"`
	CreatedAt          time.Time  `json:"createdAt" db:"created_at"`
	UpdatedAt          time.Time  `json:"updatedAt" db:"updated_at"`
}

// LearningSessionEvent is one append-only audit event.
type LearningSessionEvent struct {
	ID            string         `json:"id" db:"id" format:"uuid"`
	SessionID     string         `json:"sessionId" db:"session_id" format:"uuid"`
	EventID       string         `json:"eventId" db:"event_id" format:"uuid"`
	Sequence      int64          `json:"sequence" db:"sequence"`
	EventType     string         `json:"eventType" db:"event_type"`
	ClientTime    time.Time      `json:"clientTime" db:"client_time"`
	ReceivedAt    time.Time      `json:"receivedAt" db:"received_at"`
	SchemaVersion string         `json:"schemaVersion" db:"schema_version"`
	Payload       map[string]any `json:"payload,omitempty" db:"payload"`
}

// LearningSessionCompletion is the immutable completion result for a session.
type LearningSessionCompletion struct {
	ID            string         `json:"id" db:"id" format:"uuid"`
	SessionID     string         `json:"sessionId" db:"session_id" format:"uuid"`
	CompletionID  string         `json:"completionId" db:"completion_id" format:"uuid"`
	RequestDigest string         `json:"requestDigest" db:"request_digest"`
	Response      map[string]any `json:"response" db:"response"`
	CreatedAt     time.Time      `json:"createdAt" db:"created_at"`
}

// Artifact storage status values.
const (
	ArtifactStatusMetadataOnly = "metadata_only"
	ArtifactStatusReserved     = "reserved"
	ArtifactStatusUploaded     = "uploaded"
	ArtifactStatusRejected     = "rejected"
)

// Portfolio / fixture destination values.
const (
	PortfolioDestinationPortfolio     = "portfolio"
	PortfolioDestinationFixtureBundle = "fixture_bundle"
)

// Portfolio item status values.
const (
	PortfolioStatusActive    = "active"
	PortfolioStatusWithdrawn = "withdrawn"
)

// LearningSessionArtifact is metadata (and optional stored bytes) for evidence uploads.
type LearningSessionArtifact struct {
	ID                 string     `json:"id" db:"id" format:"uuid"`
	SessionID          string     `json:"sessionId" db:"session_id" format:"uuid"`
	ArtifactID         string     `json:"artifactId" db:"artifact_id" format:"uuid"`
	Filename           string     `json:"filename" db:"filename"`
	MediaType          string     `json:"mediaType" db:"media_type"`
	ByteSize           int64      `json:"byteSize" db:"byte_size"`
	SHA256             string     `json:"sha256" db:"sha256"`
	StoragePath        string     `json:"storagePath,omitempty" db:"storage_path"`
	Status             string     `json:"status,omitempty" db:"status"`
	StudentID          *string    `json:"studentId,omitempty" db:"student_id" format:"uuid"`
	ActivityRevisionID *string    `json:"activityRevisionId,omitempty" db:"activity_revision_id" format:"uuid"`
	BytesStored        bool       `json:"bytesStored" db:"bytes_stored"`
	RetentionUntil     *time.Time `json:"retentionUntil,omitempty" db:"retention_until"`
	CreatedAt          time.Time  `json:"createdAt" db:"created_at"`
	UpdatedAt          time.Time  `json:"updatedAt,omitempty" db:"updated_at"`
}

// PortfolioItem is a parent-promoted artifact with provenance.
type PortfolioItem struct {
	ID                 string         `json:"id" db:"id" format:"uuid"`
	StudentID          string         `json:"studentId" db:"student_id" format:"uuid"`
	SourceArtifactID   string         `json:"sourceArtifactId" db:"source_artifact_id" format:"uuid"`
	SessionID          string         `json:"sessionId" db:"session_id" format:"uuid"`
	ActivityRevisionID string         `json:"activityRevisionId" db:"activity_revision_id" format:"uuid"`
	Title              string         `json:"title" db:"title"`
	MediaType          string         `json:"mediaType" db:"media_type"`
	ByteSize           int64          `json:"byteSize" db:"byte_size"`
	SHA256             string         `json:"sha256" db:"sha256"`
	StoragePath        string         `json:"storagePath,omitempty" db:"storage_path"`
	PromotedBy         *string        `json:"promotedBy,omitempty" db:"promoted_by" format:"uuid"`
	Status             string         `json:"status" db:"status" enum:"active,withdrawn"`
	Destination        string         `json:"destination" db:"destination" enum:"portfolio,fixture_bundle"`
	Provenance         map[string]any `json:"provenance,omitempty" db:"provenance"`
	CreatedAt          time.Time      `json:"createdAt" db:"created_at"`
	UpdatedAt          time.Time      `json:"updatedAt" db:"updated_at"`
}

// ApprovedFixtureBundle is an immutable parent-approved workspace input.
type ApprovedFixtureBundle struct {
	ID                     string         `json:"id" db:"id" format:"uuid"`
	StudentID              string         `json:"studentId" db:"student_id" format:"uuid"`
	SourcePortfolioItemID  *string        `json:"sourcePortfolioItemId,omitempty" db:"source_portfolio_item_id" format:"uuid"`
	SourceArtifactID       string         `json:"sourceArtifactId" db:"source_artifact_id" format:"uuid"`
	Digest                 string         `json:"digest" db:"digest"`
	Label                  string         `json:"label" db:"label"`
	Entries                []map[string]any `json:"entries,omitempty" db:"entries"`
	StorageRoot            string         `json:"storageRoot,omitempty" db:"storage_root"`
	ApprovedBy             *string        `json:"approvedBy,omitempty" db:"approved_by" format:"uuid"`
	Status                 string         `json:"status" db:"status" enum:"approved,withdrawn"`
	Provenance             map[string]any `json:"provenance,omitempty" db:"provenance"`
	CreatedAt              time.Time      `json:"createdAt" db:"created_at"`
}

// AssignmentContinuityBinding records how an assignment materializes its workspace.
type AssignmentContinuityBinding struct {
	ID             string    `json:"id" db:"id" format:"uuid"`
	AssignmentID   string    `json:"assignmentId" db:"assignment_id" format:"uuid"`
	StudentID      string    `json:"studentId" db:"student_id" format:"uuid"`
	ContinuityMode string    `json:"continuityMode" db:"continuity_mode" enum:"fresh,optional_previous,required_project,portfolio_review"`
	BundleID       *string   `json:"bundleId,omitempty" db:"bundle_id" format:"uuid"`
	DecidedBy      *string   `json:"decidedBy,omitempty" db:"decided_by" format:"uuid"`
	Notes          string    `json:"notes,omitempty" db:"notes"`
	DecidedAt      time.Time `json:"decidedAt" db:"decided_at"`
}

// CurriculumImportRun is a durable plan/apply audit record.
type CurriculumImportRun struct {
	ID             string         `json:"id" db:"id" format:"uuid"`
	BundleDigest   string         `json:"bundleDigest" db:"bundle_digest"`
	ActorID        *string        `json:"actorId,omitempty" db:"actor_id" format:"uuid"`
	SourceLabel    string         `json:"sourceLabel" db:"source_label"`
	Mode           string         `json:"mode" db:"mode" enum:"plan,apply"`
	Status         string         `json:"status" db:"status" enum:"planned,applied,failed,rejected"`
	Plan           map[string]any `json:"plan,omitempty" db:"plan"`
	ResultManifest map[string]any `json:"resultManifest,omitempty" db:"result_manifest"`
	ErrorMessage   string         `json:"errorMessage,omitempty" db:"error_message"`
	CreatedAt      time.Time      `json:"createdAt" db:"created_at"`
	AppliedAt      *time.Time     `json:"appliedAt,omitempty" db:"applied_at"`
}

// Student response status values.
const (
	ResponseSubmitted = "submitted"
	ResponseAccepted  = "accepted"
	ResponseReturned  = "returned"
)

// Parent review decision values.
const (
	ReviewAccept = "accept"
	ReviewReturn = "return"
)

// StudentResponse is an immutable submitted conceptual response (one attempt).
// Returned work creates a new row rather than overwriting body text.
type StudentResponse struct {
	ID                   string         `json:"id" db:"id" format:"uuid"`
	SubmissionID         string         `json:"submissionId" db:"submission_id" format:"uuid"`
	StudentID            string         `json:"studentId" db:"student_id" format:"uuid"`
	SessionID            string         `json:"sessionId" db:"session_id" format:"uuid"`
	AssignmentID         string         `json:"assignmentId" db:"assignment_id" format:"uuid"`
	ActivityRevisionID   string         `json:"activityRevisionId" db:"activity_revision_id" format:"uuid"`
	TaskID               string         `json:"taskId" db:"task_id"`
	Body                 string         `json:"body" db:"body"`
	BodySHA256           string         `json:"bodySha256" db:"body_sha256"`
	Status               string         `json:"status" db:"status" enum:"submitted,accepted,returned"`
	RequestDigest        string         `json:"requestDigest" db:"request_digest"`
	Attempt              int            `json:"attempt" db:"attempt"`
	RubricSnapshot       []map[string]any `json:"rubricSnapshot,omitempty" db:"rubric_snapshot"`
	ParentReviewRequired bool           `json:"parentReviewRequired" db:"parent_review_required"`
	SubmittedAt          time.Time      `json:"submittedAt" db:"submitted_at"`
	ReviewedAt           *time.Time     `json:"reviewedAt,omitempty" db:"reviewed_at"`
	ReviewedBy           *string        `json:"reviewedBy,omitempty" db:"reviewed_by" format:"uuid"`
	ReturnReason         string         `json:"returnReason,omitempty" db:"return_reason"`
	CreatedAt            time.Time      `json:"createdAt" db:"created_at"`
	UpdatedAt            time.Time      `json:"updatedAt" db:"updated_at"`
}

// StudentResponseReview is one parent decision on a submitted response.
type StudentResponseReview struct {
	ID         string           `json:"id" db:"id" format:"uuid"`
	ResponseID string           `json:"responseId" db:"response_id" format:"uuid"`
	EducatorID string           `json:"educatorId" db:"educator_id" format:"uuid"`
	Decision   string           `json:"decision" db:"decision" enum:"accept,return"`
	Reason     string           `json:"reason" db:"reason"`
	Criteria   []map[string]any `json:"criteria,omitempty" db:"criteria"`
	CreatedAt  time.Time        `json:"createdAt" db:"created_at"`
}
