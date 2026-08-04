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
