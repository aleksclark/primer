package contracts

import "time"

// Shell event / observation schema versions (Phase 2).
const (
	ShellEventSchemaVersion        = "1"
	CommandObservationSchemaVersion = "1"
	WorkspaceManifestSchemaVersion = "1"
)

// Observation quality / provenance sources.
const (
	SourceStructured      = "structured"       // process-wait scripted exec
	SourceObserveBash     = "observe-bash"     // bash DEBUG/PROMPT instrumentation
	SourcePTYShell        = "pty-shell"        // untrusted screen scrape (never evidence)
	SourceSyntheticPTY    = "synthetic-pty"
	SourceScreen          = "screen"
)

// ShellEvent is a versioned, sequenced command boundary observation.
// Produced by the broker-owned observe path or scripted process wait — never by
// trusting visible PTY screen text as command stdout.
type ShellEvent struct {
	SchemaVersion string `json:"schemaVersion"`
	// SessionID is the client session id when known.
	SessionID string `json:"sessionId,omitempty"`
	// Sequence is monotonically increasing within the session (1-based).
	Sequence int64 `json:"sequence"`
	// StartedAt / FinishedAt bound the command attempt.
	StartedAt  time.Time `json:"startedAt,omitempty"`
	FinishedAt time.Time `json:"finishedAt"`

	// SubmittedLine is the original line for audit display (bounded).
	SubmittedLine string `json:"submittedLine,omitempty"`
	// Executable and Argv are set when determinable; empty when unavailable.
	Executable string   `json:"executable,omitempty"`
	Argv       []string `json:"argv,omitempty"`
	// ArgvAvailable is false when executable/argv could not be determined.
	ArgvAvailable bool `json:"argvAvailable"`

	// CwdBefore / CwdAfter are workspace-relative paths when known.
	CwdBefore string `json:"cwdBefore,omitempty"`
	CwdAfter  string `json:"cwdAfter,omitempty"`
	// CwdAvailable is true when CwdAfter was observed outside the student screen.
	CwdAvailable bool `json:"cwdAvailable"`

	// ExitCode is the process or shell status when ExitAvailable.
	ExitCode      int  `json:"exitCode"`
	ExitAvailable bool `json:"exitAvailable"`
	// Signal is a terminating signal name when applicable (e.g. "SIGTERM").
	Signal string `json:"signal,omitempty"`

	// Stdout / Stderr carry bounded display excerpts (not full transcripts).
	Stdout Excerpt `json:"stdout"`
	Stderr Excerpt `json:"stderr"`

	// Pipeline notes simple structure when known (best-effort).
	Pipeline *PipelineInfo `json:"pipeline,omitempty"`

	// ManifestBefore / ManifestAfter are optional workspace digests around the command.
	ManifestBefore string `json:"manifestBefore,omitempty"`
	ManifestAfter  string `json:"manifestAfter,omitempty"`
	// WriteSet lists relative paths that changed between manifests (bounded).
	WriteSet []string `json:"writeSet,omitempty"`

	// Runner / instrumentation / verifier identity.
	RunnerVersion           string `json:"runnerVersion,omitempty"`
	ShellInstrumentation    string `json:"shellInstrumentation,omitempty"`
	VerifierVersion         string `json:"verifierVersion,omitempty"`
	Source                  string `json:"source"`
	// Structured is true only when quality meets the structured-evidence bar.
	Structured bool `json:"structured"`
	// Quality records which fields are trusted.
	Quality EvidenceQuality `json:"quality"`
}

// Excerpt is a bounded stream sample plus digests.
type Excerpt struct {
	// Text is a display-bounded excerpt (may be empty when untrusted/unavailable).
	Text string `json:"text,omitempty"`
	// SHA256 is the digest of the full captured stream when available.
	SHA256 string `json:"sha256,omitempty"`
	// ByteLen is the full captured length before bounding.
	ByteLen int `json:"byteLen,omitempty"`
	// Truncated is true when Text is shorter than the full stream.
	Truncated bool `json:"truncated,omitempty"`
	// Trusted is true when Text/digest came from process pipes, not screen scrape.
	Trusted bool `json:"trusted"`
}

// PipelineInfo is a minimal pipeline/redirection sketch for predicates.
type PipelineInfo struct {
	// Stages are executable names left-to-right when parseable.
	Stages []string `json:"stages,omitempty"`
	// HasPipe / HasRedirectOut are structural flags.
	HasPipe       bool `json:"hasPipe,omitempty"`
	HasRedirectOut bool `json:"hasRedirectOut,omitempty"`
	HasRedirectIn  bool `json:"hasRedirectIn,omitempty"`
}

// EvidenceQuality declares which observation fields meet the trust bar.
type EvidenceQuality struct {
	Exit   bool `json:"exit"`
	Cwd    bool `json:"cwd"`
	Argv   bool `json:"argv"`
	Stdout bool `json:"stdout"`
	Stderr bool `json:"stderr"`
}

// MeetsStructuredBar reports whether this quality is enough to advertise
// structured command evidence for command_properties-style checks.
// Stdout is not required: many interactive observations lack pipe capture.
func (q EvidenceQuality) MeetsStructuredBar() bool {
	return q.Exit && q.Cwd && q.Argv
}

// CommandObservation is the verifier-facing view of one finished command,
// including task-scoping metadata retained in session history.
type CommandObservation struct {
	SchemaVersion string    `json:"schemaVersion"`
	Sequence      int64     `json:"sequence"`
	TaskID        string    `json:"taskId,omitempty"`
	TaskIndex     int       `json:"taskIndex"`
	RecordedAt    time.Time `json:"recordedAt"`

	Executable    string   `json:"executable,omitempty"`
	Argv          []string `json:"argv,omitempty"`
	ArgvAvailable bool     `json:"argvAvailable"`
	SubmittedLine string   `json:"submittedLine,omitempty"`

	CwdBefore    string `json:"cwdBefore,omitempty"`
	CwdAfter     string `json:"cwdAfter,omitempty"`
	CwdAvailable bool   `json:"cwdAvailable"`

	ExitCode      int  `json:"exitCode"`
	ExitAvailable bool `json:"exitAvailable"`

	Stdout Excerpt `json:"stdout"`
	Stderr Excerpt `json:"stderr"`

	Pipeline *PipelineInfo `json:"pipeline,omitempty"`

	ManifestBefore string   `json:"manifestBefore,omitempty"`
	ManifestAfter  string   `json:"manifestAfter,omitempty"`
	WriteSet       []string `json:"writeSet,omitempty"`

	Source     string          `json:"source"`
	Structured bool            `json:"structured"`
	Quality    EvidenceQuality `json:"quality"`
}

// WorkspaceManifestEntry is one path in a workspace snapshot.
type WorkspaceManifestEntry struct {
	Path    string `json:"path"`
	Type    string `json:"type"` // file|directory|symlink
	Mode    string `json:"mode,omitempty"`
	SHA256  string `json:"sha256,omitempty"`
	Size    int64  `json:"size,omitempty"`
}

// WorkspaceManifest is a bounded relative-path digest of a workspace tree.
type WorkspaceManifest struct {
	SchemaVersion string                   `json:"schemaVersion"`
	Digest        string                   `json:"digest"`
	Entries       []WorkspaceManifestEntry `json:"entries,omitempty"`
	Truncated     bool                     `json:"truncated,omitempty"`
}

// ObservationFromShellEvent builds a CommandObservation from a ShellEvent.
func ObservationFromShellEvent(ev ShellEvent, taskID string, taskIndex int) CommandObservation {
	return CommandObservation{
		SchemaVersion:  CommandObservationSchemaVersion,
		Sequence:       ev.Sequence,
		TaskID:         taskID,
		TaskIndex:      taskIndex,
		RecordedAt:     ev.FinishedAt,
		Executable:     ev.Executable,
		Argv:           append([]string(nil), ev.Argv...),
		ArgvAvailable:  ev.ArgvAvailable,
		SubmittedLine:  ev.SubmittedLine,
		CwdBefore:      ev.CwdBefore,
		CwdAfter:       ev.CwdAfter,
		CwdAvailable:   ev.CwdAvailable,
		ExitCode:       ev.ExitCode,
		ExitAvailable:  ev.ExitAvailable,
		Stdout:         ev.Stdout,
		Stderr:         ev.Stderr,
		Pipeline:       ev.Pipeline,
		ManifestBefore: ev.ManifestBefore,
		ManifestAfter:  ev.ManifestAfter,
		WriteSet:       append([]string(nil), ev.WriteSet...),
		Source:         ev.Source,
		Structured:     ev.Structured,
		Quality:        ev.Quality,
	}
}
