package contracts

// CourseDocument is the git-authored course manifest contract.
// Phase 4 persists and executes these semantics; this package only defines and
// validates the document shape. Calendar fields are intentionally unsupported.
type CourseDocument struct {
	SchemaVersion      string               `json:"schemaVersion"`
	Slug               string               `json:"slug"`
	Title              string               `json:"title"`
	SubjectCode        string               `json:"subjectCode"`
	Version            string               `json:"version,omitempty"`
	ParentDescription  string               `json:"parentDescription,omitempty"`
	RevisionPolicy     string               `json:"revisionPolicy,omitempty"`
	PacingReference    *CoursePacing        `json:"pacingReference,omitempty"`
	Activities         []CourseActivityRef  `json:"activities"`
	Prerequisites      []CoursePrerequisite `json:"prerequisites,omitempty"`
	Gates              []CourseGate         `json:"gates,omitempty"`
	Remediation        []CourseRemediation  `json:"remediation,omitempty"`
	Modules            []CourseModule       `json:"modules,omitempty"`
	ContinuityDefaults *ContinuityPolicy    `json:"continuityDefaults,omitempty"`
	Metadata           map[string]string    `json:"metadata,omitempty"`
}

// CoursePacing is descriptive effort metadata only — never a progression gate.
type CoursePacing struct {
	NominalWeeks         int  `json:"nominalWeeks,omitempty"`
	NominalDaysPerWeek   int  `json:"nominalDaysPerWeek,omitempty"`
	NominalMinutesPerDay int  `json:"nominalMinutesPerDay,omitempty"`
	MasteryBased         bool `json:"masteryBased,omitempty"`
}

// CourseActivityRef places one activity in course order.
type CourseActivityRef struct {
	Order      int               `json:"order"`
	Slug       string            `json:"slug"`
	File       string            `json:"file,omitempty"`
	Module     string            `json:"module,omitempty"`
	Capstone   bool              `json:"capstone,omitempty"`
	Continuity *ContinuityPolicy `json:"continuity,omitempty"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

// CoursePrerequisite is a directed prerequisite edge between activity slugs.
type CoursePrerequisite struct {
	Activity     string   `json:"activity"`
	Requires     []string `json:"requires"`
	Requirement  string   `json:"requirement,omitempty"` // completed | mastered | approaching
	Description  string   `json:"description,omitempty"`
}

// CourseGate is an evidence or parent-review gate attached to an activity.
type CourseGate struct {
	Activity    string   `json:"activity"`
	Kind        string   `json:"kind"` // evidence | parent_review
	Standards   []string `json:"standards,omitempty"`
	Description string   `json:"description,omitempty"`
}

// CourseRemediation references a remediation or reinforcement branch.
type CourseRemediation struct {
	ForActivity string `json:"forActivity"`
	BranchSlug  string `json:"branchSlug"`
	Kind        string `json:"kind,omitempty"` // remediation | reinforcement
	Description string `json:"description,omitempty"`
}

// CourseModule groups activities for navigation/reporting.
type CourseModule struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description,omitempty"`
	Activities  []string `json:"activities,omitempty"`
}

// ContinuityPolicy placeholders for Phase 6 workspace continuity.
// Unknown variants must fail validation rather than being ignored.
type ContinuityPolicy struct {
	Mode string `json:"mode"` // fresh | optional_previous | required_project | portfolio_review
}

// Supported continuity modes (Phase 1 placeholders).
const (
	ContinuityFresh            = "fresh"
	ContinuityOptionalPrevious = "optional_previous"
	ContinuityRequiredProject  = "required_project"
	ContinuityPortfolioReview  = "portfolio_review"
)

// Revision resolution policies for course activity refs.
const (
	RevisionPolicyLatestPublished = "latest_published"
	RevisionPolicyPinnedDigest    = "pinned_digest"
)

// Gate kinds.
const (
	GateEvidence      = "evidence"
	GateParentReview  = "parent_review"
)

// Prerequisite requirement levels.
const (
	PrereqCompleted   = "completed"
	PrereqApproaching = "approaching"
	PrereqMastered    = "mastered"
)
