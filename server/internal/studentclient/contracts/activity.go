package contracts

// ActivityDocument is the on-disk / API authoring document for one activity
// revision payload. Phase 0 loads these from curriculum/activities/*/activity.yaml.
type ActivityDocument struct {
	SchemaVersion string            `json:"schemaVersion" yaml:"schema_version"`
	Slug          string            `json:"slug" yaml:"slug"`
	Title         string            `json:"title" yaml:"title"`
	Summary       string            `json:"summary" yaml:"summary"`
	Kind          string            `json:"kind" yaml:"kind"`
	SubjectCode   string            `json:"subjectCode" yaml:"subject_code"`
	Standards     []StandardRef     `json:"standards" yaml:"standards"`
	Content       ActivityContent   `json:"content" yaml:"content"`
	MinRunner     string            `json:"minRunnerVersion,omitempty" yaml:"min_runner_version,omitempty"`
	Metadata      map[string]string `json:"metadata,omitempty" yaml:"metadata,omitempty"`
}

// StandardRef links an activity revision to a standard code.
type StandardRef struct {
	Code   string  `json:"code" yaml:"code"`
	Role   string  `json:"role" yaml:"role"`
	Weight float64 `json:"weight,omitempty" yaml:"weight,omitempty"`
}

// ActivityContent is the versioned revision body stored as JSONB later.
type ActivityContent struct {
	Objective    string             `json:"objective" yaml:"objective"`
	Instructions string             `json:"instructions" yaml:"instructions"`
	Terminal     *TerminalContent   `json:"terminal,omitempty" yaml:"terminal,omitempty"`
	Typing       *TypingContent     `json:"typing,omitempty" yaml:"typing,omitempty"`
	Tasks        []Task             `json:"tasks" yaml:"tasks"`
	Checks       []Check            `json:"checks" yaml:"checks"`
	Hints        []Hint             `json:"hints,omitempty" yaml:"hints,omitempty"`
	Tutor        *TutorContext      `json:"tutor,omitempty" yaml:"tutor,omitempty"`
	Progression  *ProgressionPolicy `json:"progression,omitempty" yaml:"progression,omitempty"`
	Artifacts    *ArtifactPolicy    `json:"artifacts,omitempty" yaml:"artifacts,omitempty"`
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

// Task is one ordered learning step inside an activity.
type Task struct {
	ID            string    `json:"id" yaml:"id"`
	Title         string    `json:"title" yaml:"title"`
	Instructions  string    `json:"instructions" yaml:"instructions"`
	Prerequisites []string  `json:"prerequisites,omitempty" yaml:"prerequisites,omitempty"`
	Completion    CheckTree `json:"completion" yaml:"completion"`
	HintIDs       []string  `json:"hintIds,omitempty" yaml:"hint_ids,omitempty"`
	Optional      bool      `json:"optional,omitempty" yaml:"optional,omitempty"`
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
type Check struct {
	ID       string         `json:"id" yaml:"id"`
	Kind     string         `json:"kind" yaml:"kind"`
	Optional bool           `json:"optional,omitempty" yaml:"optional,omitempty"`
	Params   map[string]any `json:"params" yaml:"params"`
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
	AllowReset     bool `json:"allowReset" yaml:"allow_reset"`
	MaxAttempts    int  `json:"maxAttempts,omitempty" yaml:"max_attempts,omitempty"`
	ResumeFromTask bool `json:"resumeFromTask" yaml:"resume_from_task"`
	RequireInOrder bool `json:"requireInOrder" yaml:"require_in_order"`
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
