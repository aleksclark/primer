package contracts

// SchemaVersion is the activity revision content schema version implemented here.
const SchemaVersion = "1"

// Activity kinds.
const (
	KindTerminal = "terminal"
	KindTyping   = "typing"
)

// Standard link roles on an activity revision.
const (
	StandardRolePrimary       = "primary"
	StandardRoleReinforcement = "reinforcement"
)

// Fixture entry types.
const (
	FixtureFile      = "file"
	FixtureDirectory = "directory"
)

// Check kinds (deterministic verifier vocabulary).
const (
	CheckFileExists        = "file_exists"
	CheckFileNotExists     = "file_not_exists"
	CheckContentEquals     = "content_equals"
	CheckContentContains   = "content_contains"
	CheckContentMatch      = "content_match"
	CheckPathType          = "path_type"
	CheckPathMode          = "path_mode"
	CheckCwd               = "cwd"
	CheckCommandProperties = "command_properties"
	CheckPipelineOutput    = "pipeline_output"
	CheckTypingMetrics     = "typing_metrics"
)

// Path types for path_type checks and fixtures.
const (
	PathTypeFile      = "file"
	PathTypeDirectory = "directory"
	PathTypeSymlink   = "symlink"
)

// Runtime profiles for terminal activities.
const (
	RuntimeCoreutilsBasic  = "coreutils-basic"
	RuntimeTextProcessing  = "text-processing"
)

// Event types for the session audit stream.
const (
	EventSessionStarted   = "session_started"
	EventTaskViewed       = "task_viewed"
	EventCommandFinished  = "command_finished"
	EventCheckEvaluated   = "check_evaluated"
	EventHintRequested    = "hint_requested"
	EventTutorMessage     = "tutor_message"
	EventTypingSample     = "typing_sample"
	EventSessionPaused    = "session_paused"
	EventSessionCompleted = "session_completed"
)

// Payload schema versions for observations/events/completions/artifacts.
const (
	ObservationSchemaVersion = "1"
	EventSchemaVersion       = "1"
	CompletionSchemaVersion  = "1"
	ArtifactSchemaVersion    = "1"
	IPCSchemaVersion         = "1"
)

// Evidence classes recorded with mastery evidence.
const (
	EvidenceProceduralContinuous = "procedural_continuous"
	EvidenceConceptualResponse   = "conceptual_response"
	EvidenceParentAttestation    = "parent_attestation"
	EvidenceFormalAssessment     = "formal_assessment"
	EvidencePortfolio            = "portfolio"
)

// Runner capability flags (Phase 0 minimum set).
const (
	// CapStructuredCommandEvidence is required to trust command_properties /
	// pipeline_output checks. Synthetic PTY screen text is not this capability.
	CapStructuredCommandEvidence = "structured_command_evidence"
)

// RequiresStructuredCommandEvidence reports whether a check kind needs trusted
// structured command completion data rather than filesystem state alone.
func RequiresStructuredCommandEvidence(kind string) bool {
	switch kind {
	case CheckCommandProperties, CheckPipelineOutput:
		return true
	default:
		return false
	}
}

// RevisionRequiresStructuredCommand reports whether required completion cannot
// succeed without structured command evidence (no filesystem-only path).
// Shared by server session start and the student client so offline/local opens
// cannot bypass capability policy.
func RevisionRequiresStructuredCommand(content ActivityContent) bool {
	byID := map[string]Check{}
	for _, c := range content.Checks {
		byID[c.ID] = c
	}
	if len(content.Tasks) == 0 {
		// All non-optional checks must pass; if any is non-command, activity is runnable.
		hasRequired := false
		for _, c := range content.Checks {
			if c.Optional {
				continue
			}
			hasRequired = true
			if !RequiresStructuredCommandEvidence(c.Kind) {
				return false
			}
		}
		return hasRequired
	}
	// Every required task must be satisfiable; reject only if some required task
	// has no completion path that avoids structured command evidence.
	for _, task := range content.Tasks {
		if task.Optional {
			continue
		}
		if treeRequiresStructuredCommand(task.Completion, byID) {
			return true
		}
	}
	return false
}

// treeRequiresStructuredCommand is true when every non-optional path through the
// tree includes at least one structured-command check (no capability-free path).
func treeRequiresStructuredCommand(t CheckTree, byID map[string]Check) bool {
	if t.Optional {
		return false
	}
	if t.CheckID != "" {
		c, ok := byID[t.CheckID]
		if !ok {
			return true
		}
		if c.Optional {
			return false
		}
		return RequiresStructuredCommandEvidence(c.Kind)
	}
	if len(t.All) > 0 {
		// all: requires capability if any required child does.
		for _, c := range t.All {
			if treeRequiresStructuredCommand(c, byID) {
				return true
			}
		}
		return false
	}
	if len(t.Any) > 0 {
		// any: requires capability only if every alternative does.
		for _, c := range t.Any {
			if !treeRequiresStructuredCommand(c, byID) {
				return false
			}
		}
		return true
	}
	return false
}

// ErrIncompatibleRevision is returned when a runner lacks capabilities required
// by an activity revision's completion policy.
type ErrIncompatibleRevision struct {
	Msg string
}

func (e ErrIncompatibleRevision) Error() string {
	if e.Msg == "" {
		return "runner lacks structured_command_evidence required by this activity revision"
	}
	return e.Msg
}

// RejectIncompatibleRevision returns ErrIncompatibleRevision when every required
// completion path needs structured command evidence the runner lacks.
func RejectIncompatibleRevision(content ActivityContent, capabilities map[string]bool) error {
	if capabilities[CapStructuredCommandEvidence] {
		return nil
	}
	if !RevisionRequiresStructuredCommand(content) {
		return nil
	}
	return ErrIncompatibleRevision{
		Msg: "runner lacks structured_command_evidence required by this activity revision",
	}
}
