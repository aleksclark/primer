package contracts

// ActivityDocument is the on-disk / API authoring document for one activity
// revision payload. Phase 0 loads these from curriculum/activities/*/activity.yaml.
//
// ReferenceSolution is authoring-only and is never part of the published
// student revision body (PublishActivityRevision stores Content only).
type ActivityDocument struct {
	SchemaVersion     string             `json:"schemaVersion" yaml:"schema_version"`
	Slug              string             `json:"slug" yaml:"slug"`
	Title             string             `json:"title" yaml:"title"`
	Summary           string             `json:"summary" yaml:"summary"`
	Kind              string             `json:"kind" yaml:"kind"`
	SubjectCode       string             `json:"subjectCode" yaml:"subject_code"`
	Standards         []StandardRef      `json:"standards" yaml:"standards"`
	Content           ActivityContent    `json:"content" yaml:"content"`
	MinRunner         string             `json:"minRunnerVersion,omitempty" yaml:"min_runner_version,omitempty"`
	Metadata          map[string]string  `json:"metadata,omitempty" yaml:"metadata,omitempty"`
	ReferenceSolution *ReferenceSolution `json:"referenceSolution,omitempty" yaml:"reference_solution,omitempty"`
}

// StandardRef links an activity revision to a standard code.
type StandardRef struct {
	Code           string          `json:"code" yaml:"code"`
	Role           string          `json:"role" yaml:"role"`
	Weight         float64         `json:"weight,omitempty" yaml:"weight,omitempty"`
	EvidencePolicy *EvidencePolicy `json:"evidencePolicy,omitempty" yaml:"evidence_policy,omitempty"`
}

// EvidencePolicy describes which evidence classes are required before a
// mastery status transition for a revision-standard link.
type EvidencePolicy struct {
	Version            int                 `json:"version" yaml:"version"`
	StatusRequirements map[string][]string `json:"statusRequirements" yaml:"status_requirements"`
}

// DefaultTerminalEvidencePolicy is the conservative v1 default for terminal
// activities: procedural evidence may introduce a standard, but cannot alone
// establish approaching or mastered.
func DefaultTerminalEvidencePolicy() EvidencePolicy {
	return EvidencePolicy{
		Version: 1,
		StatusRequirements: map[string][]string{
			"in_progress": {"procedural_continuous"},
			"approaching": {"procedural_continuous", "conceptual_response"},
			"mastered":    {"procedural_continuous", "conceptual_response", "formal_assessment"},
		},
	}
}

// DefaultTypingEvidencePolicy allows procedural typing evidence to advance
// typing standards through the normal confidence ladder.
func DefaultTypingEvidencePolicy() EvidencePolicy {
	return EvidencePolicy{
		Version: 1,
		StatusRequirements: map[string][]string{
			"in_progress": {"procedural_continuous"},
			"approaching": {"procedural_continuous"},
			"mastered":    {"procedural_continuous"},
		},
	}
}

// ActivityContent is the versioned revision body stored as JSONB later.
type ActivityContent struct {
	Objective    string              `json:"objective" yaml:"objective"`
	Instructions string              `json:"instructions" yaml:"instructions"`
	// Blocks are ordered typed instructional material delivered with the
	// immutable revision. Parent-note blocks are authoring/parent-only.
	Blocks       []InstructionBlock  `json:"blocks,omitempty" yaml:"blocks,omitempty"`
	Terminal     *TerminalContent    `json:"terminal,omitempty" yaml:"terminal,omitempty"`
	Typing       *TypingContent      `json:"typing,omitempty" yaml:"typing,omitempty"`
	Tasks        []Task              `json:"tasks" yaml:"tasks"`
	Checks       []Check             `json:"checks" yaml:"checks"`
	Hints        []Hint              `json:"hints,omitempty" yaml:"hints,omitempty"`
	Tutor        *TutorContext       `json:"tutor,omitempty" yaml:"tutor,omitempty"`
	Progression  *ProgressionPolicy  `json:"progression,omitempty" yaml:"progression,omitempty"`
	Artifacts    *ArtifactPolicy     `json:"artifacts,omitempty" yaml:"artifacts,omitempty"`
}

// Instruction block kinds (typed teaching material, not free-form HTML).
const (
	BlockProse       = "prose"
	BlockVocabulary  = "vocabulary"
	BlockExample     = "example"
	BlockWarning     = "warning"
	BlockQuestion    = "question"
	BlockPractice    = "practice"
	BlockParentNote  = "parent_note"
	BlockResource    = "resource"
)

// InstructionBlock is one ordered teaching unit inside ActivityContent.
// Exactly the fields for Kind should be populated; validation enforces this.
type InstructionBlock struct {
	ID    string `json:"id" yaml:"id"`
	Kind  string `json:"kind" yaml:"kind"`
	Title string `json:"title,omitempty" yaml:"title,omitempty"`
	// Text is used by prose, warning, question, practice, and parent_note.
	Text string `json:"text,omitempty" yaml:"text,omitempty"`
	// Terms is used by vocabulary blocks.
	Terms []VocabularyTerm `json:"terms,omitempty" yaml:"terms,omitempty"`
	// Example fields (example blocks).
	Input       string `json:"input,omitempty" yaml:"input,omitempty"`
	Output      string `json:"output,omitempty" yaml:"output,omitempty"`
	Explanation string `json:"explanation,omitempty" yaml:"explanation,omitempty"`
	// Resource is used by resource blocks (content-addressed local attachment).
	Resource *ResourceRef `json:"resource,omitempty" yaml:"resource,omitempty"`
}

// VocabularyTerm is one term/definition pair.
type VocabularyTerm struct {
	Term       string `json:"term" yaml:"term"`
	Definition string `json:"definition" yaml:"definition"`
}

// ResourceRef points at an approved content-addressed attachment.
type ResourceRef struct {
	// SHA256 is the hex digest of the attachment bytes.
	SHA256   string `json:"sha256" yaml:"sha256"`
	// MediaType is a simple type such as text/plain or image/png.
	MediaType string `json:"mediaType" yaml:"media_type"`
	// Label is a short student-visible name.
	Label string `json:"label" yaml:"label"`
	// ByteSize bounds the attachment (validated at authoring time).
	ByteSize int64 `json:"byteSize,omitempty" yaml:"byte_size,omitempty"`
}

// TerminalContent configures a terminal activity revision.
type TerminalContent struct {
	RuntimeProfile string         `json:"runtimeProfile" yaml:"runtime_profile"`
	Fixtures       []FixtureEntry `json:"fixtures" yaml:"fixtures"`
	InitialCwd     string         `json:"initialCwd,omitempty" yaml:"initial_cwd,omitempty"`
	Shell          string         `json:"shell,omitempty" yaml:"shell,omitempty"`
}

// TypingContent configures a typing activity revision.
type TypingContent struct {
	PromptSetID     string         `json:"promptSetId" yaml:"prompt_set_id"`
	Prompts         []TypingPrompt `json:"prompts" yaml:"prompts"`
	Ordering        string         `json:"ordering,omitempty" yaml:"ordering,omitempty"`
	TimeLimitSec    int            `json:"timeLimitSec,omitempty" yaml:"time_limit_sec,omitempty"`
	SuccessWPM      float64        `json:"successWpm,omitempty" yaml:"success_wpm,omitempty"`
	SuccessAccuracy float64        `json:"successAccuracy,omitempty" yaml:"success_accuracy,omitempty"`
}

// TypingPrompt is one prompt in a typing set.
type TypingPrompt struct {
	ID       string `json:"id" yaml:"id"`
	Text     string `json:"text" yaml:"text"`
	Category string `json:"category,omitempty" yaml:"category,omitempty"`
}

// FixtureEntry declares a file or directory to materialize in the workspace.
type FixtureEntry struct {
	Path    string `json:"path" yaml:"path"`
	Type    string `json:"type" yaml:"type"`
	Content string `json:"content,omitempty" yaml:"content,omitempty"`
	Mode    string `json:"mode,omitempty" yaml:"mode,omitempty"`
}

// Task kinds. Empty / "action" means ordinary terminal/typing work.
const (
	TaskKindAction        = "action"
	TaskKindShortResponse = "short_response"
)

// Task is one ordered learning step inside an activity.
type Task struct {
	ID            string    `json:"id" yaml:"id"`
	Title         string    `json:"title" yaml:"title"`
	Instructions  string    `json:"instructions" yaml:"instructions"`
	// Kind selects the interaction model. Empty defaults to action.
	Kind          string    `json:"kind,omitempty" yaml:"kind,omitempty"`
	Prerequisites []string  `json:"prerequisites,omitempty" yaml:"prerequisites,omitempty"`
	Completion    CheckTree `json:"completion" yaml:"completion"`
	HintIDs       []string  `json:"hintIds,omitempty" yaml:"hint_ids,omitempty"`
	Optional      bool      `json:"optional,omitempty" yaml:"optional,omitempty"`
	// Response configures short constructed-response tasks.
	Response *ResponseTaskSpec `json:"response,omitempty" yaml:"response,omitempty"`
}

// ResponseTaskSpec authors a short constructed-response task with rubric criteria.
type ResponseTaskSpec struct {
	// Prompt is shown to the student (may mirror or extend task.instructions).
	Prompt string `json:"prompt" yaml:"prompt"`
	// MaxChars bounds the submitted body (default 2000 when zero).
	MaxChars int `json:"maxChars,omitempty" yaml:"max_chars,omitempty"`
	// Rubric is the explicit criteria parents review against.
	Rubric []RubricCriterion `json:"rubric" yaml:"rubric"`
	// ParentReviewRequired forces parent decision before conceptual evidence is fully accepted
	// for gates that require parent_attestation. Submission still records conceptual_response.
	ParentReviewRequired bool `json:"parentReviewRequired,omitempty" yaml:"parent_review_required,omitempty"`
}

// RubricCriterion is one named review criterion on a response task.
type RubricCriterion struct {
	ID          string `json:"id" yaml:"id"`
	Description string `json:"description" yaml:"description"`
	// Required means the parent must accept (or mark N/A with reason) this criterion.
	Required bool `json:"required,omitempty" yaml:"required,omitempty"`
}

// CheckTree is an all/any tree over check IDs or nested trees.
// Exactly one of All, Any, or CheckID should be set at each node.
type CheckTree struct {
	All      []CheckTree `json:"all,omitempty" yaml:"all,omitempty"`
	Any      []CheckTree `json:"any,omitempty" yaml:"any,omitempty"`
	CheckID  string      `json:"checkId,omitempty" yaml:"check_id,omitempty"`
	Optional bool        `json:"optional,omitempty" yaml:"optional,omitempty"`
}

// Check is a deterministic verifier assertion.
//
// Stages declare when the check is expected to hold. When omitted, validation
// defaults the check to StageFinal so intentional repair/initial-condition
// checks must opt into StageFixture or StageTask explicitly.
type Check struct {
	ID               string         `json:"id" yaml:"id"`
	Kind             string         `json:"kind" yaml:"kind"`
	Optional         bool           `json:"optional,omitempty" yaml:"optional,omitempty"`
	Params           map[string]any `json:"params" yaml:"params"`
	Stages           []string       `json:"stages,omitempty" yaml:"stages,omitempty"`
	EvidenceBearing  *bool          `json:"evidenceBearing,omitempty" yaml:"evidence_bearing,omitempty"`
	InvariantAt      []string       `json:"invariantAt,omitempty" yaml:"invariant_at,omitempty"`
}

// Hint is a graduated coaching tip referenced by tasks.
type Hint struct {
	ID    string `json:"id" yaml:"id"`
	Level int    `json:"level" yaml:"level"`
	Text  string `json:"text" yaml:"text"`
}

// TutorContext steers server-side coaching without granting tool access.
type TutorContext struct {
	Misconceptions []string `json:"misconceptions,omitempty" yaml:"misconceptions,omitempty"`
	Constraints    []string `json:"constraints,omitempty" yaml:"constraints,omitempty"`
	GoalSummary    string   `json:"goalSummary,omitempty" yaml:"goal_summary,omitempty"`
}

// ProgressionPolicy controls attempt/reset/resume behavior.
type ProgressionPolicy struct {
	AllowReset       bool `json:"allowReset" yaml:"allow_reset"`
	MaxAttempts      int  `json:"maxAttempts,omitempty" yaml:"max_attempts,omitempty"`
	ResumeFromTask   bool `json:"resumeFromTask" yaml:"resume_from_task"`
	RequireInOrder   bool `json:"requireInOrder" yaml:"require_in_order"`
}

// ArtifactPolicy bounds optional evidence uploads for a revision.
type ArtifactPolicy struct {
	Enabled       bool     `json:"enabled" yaml:"enabled"`
	MaxFiles      int      `json:"maxFiles,omitempty" yaml:"max_files,omitempty"`
	MaxBytesEach  int64    `json:"maxBytesEach,omitempty" yaml:"max_bytes_each,omitempty"`
	MaxBytesTotal int64    `json:"maxBytesTotal,omitempty" yaml:"max_bytes_total,omitempty"`
	AllowedTypes  []string `json:"allowedTypes,omitempty" yaml:"allowed_types,omitempty"`
	RetainDays    int      `json:"retainDays,omitempty" yaml:"retain_days,omitempty"`
}

// ReferenceSolution is an authoring-only replay script. It must never be treated
// as student-delivered verifier code; publish paths store Content only.
type ReferenceSolution struct {
	Description   string          `json:"description,omitempty" yaml:"description,omitempty"`
	Deterministic bool            `json:"deterministic,omitempty" yaml:"deterministic,omitempty"`
	Steps         []ReferenceStep `json:"steps" yaml:"steps"`
}

// ReferenceStep is one sandboxed runner input applied during authoring replay.
type ReferenceStep struct {
	WorkDir string   `json:"workDir,omitempty" yaml:"work_dir,omitempty"`
	Argv    []string `json:"argv" yaml:"argv"`
	Stdin   string   `json:"stdin,omitempty" yaml:"stdin,omitempty"`
}
