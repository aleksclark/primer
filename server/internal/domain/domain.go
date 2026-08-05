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
	ID                 string         `json:"id" db:"id" format:"uuid"`
	StudentID          string         `json:"studentId" db:"student_id" format:"uuid"`
	ActivityRevisionID string         `json:"activityRevisionId" db:"activity_revision_id" format:"uuid"`
	EnrollmentID       *string        `json:"enrollmentId,omitempty" db:"enrollment_id" format:"uuid"`
	State              string         `json:"state" db:"state" enum:"available,in_progress,completed,cancelled"`
	Priority           int            `json:"priority" db:"priority"`
	AvailableAt        time.Time      `json:"availableAt" db:"available_at"`
	DueAt              *time.Time     `json:"dueAt,omitempty" db:"due_at"`
	AssignedBy         *string        `json:"assignedBy,omitempty" db:"assigned_by" format:"uuid"`
	Reason             string         `json:"reason" db:"reason"`
	Constraints        map[string]any `json:"constraints,omitempty" db:"constraints"`
	CreatedAt          time.Time      `json:"createdAt" db:"created_at"`
	UpdatedAt          time.Time      `json:"updatedAt" db:"updated_at"`
}

// StudentDevice is a paired student workstation.
type StudentDevice struct {
	ID         string     `json:"id" db:"id" format:"uuid"`
	StudentID  string     `json:"studentId" db:"student_id" format:"uuid"`
	Name       string     `json:"name" db:"name"`
	TokenHash  string     `json:"-" db:"token_hash"`
	LastSeenAt *time.Time `json:"lastSeenAt,omitempty" db:"last_seen_at"`
	RevokedAt  *time.Time `json:"revokedAt,omitempty" db:"revoked_at"`
	CreatedAt  time.Time  `json:"createdAt" db:"created_at"`
	UpdatedAt  time.Time  `json:"updatedAt" db:"updated_at"`
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

// LearningSessionArtifact is metadata for optional evidence uploads.
type LearningSessionArtifact struct {
	ID          string    `json:"id" db:"id" format:"uuid"`
	SessionID   string    `json:"sessionId" db:"session_id" format:"uuid"`
	ArtifactID  string    `json:"artifactId" db:"artifact_id" format:"uuid"`
	Filename    string    `json:"filename" db:"filename"`
	MediaType   string    `json:"mediaType" db:"media_type"`
	ByteSize    int64     `json:"byteSize" db:"byte_size"`
	SHA256      string    `json:"sha256" db:"sha256"`
	StoragePath string    `json:"storagePath,omitempty" db:"storage_path"`
	CreatedAt   time.Time `json:"createdAt" db:"created_at"`
}
