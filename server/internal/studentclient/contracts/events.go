package contracts

import "time"

// Observation is a normalized verifier observation for one check evaluation.
type Observation struct {
	SchemaVersion string         `json:"schemaVersion"`
	CheckID       string         `json:"checkId"`
	Kind          string         `json:"kind"`
	Passed        bool           `json:"passed"`
	Optional      bool           `json:"optional,omitempty"`
	Message       string         `json:"message,omitempty"`
	ObservedAt    time.Time      `json:"observedAt"`
	Details       map[string]any `json:"details,omitempty"`
}

// SessionEvent is one append-only audit event from the student client.
type SessionEvent struct {
	SchemaVersion string         `json:"schemaVersion"`
	EventID       string         `json:"eventId"`
	Type          string         `json:"type"`
	Sequence      int64          `json:"sequence"`
	ClientTime    time.Time      `json:"clientTime"`
	Payload       map[string]any `json:"payload,omitempty"`
}

// CommandFinishedPayload is the payload for EventCommandFinished.
type CommandFinishedPayload struct {
	Executable string   `json:"executable"`
	Args       []string `json:"args,omitempty"`
	ExitCode   int      `json:"exitCode"`
	Cwd        string   `json:"cwd,omitempty"`
	DurationMs int64    `json:"durationMs,omitempty"`
	StdoutNorm string   `json:"stdoutNorm,omitempty"`
	StderrNorm string   `json:"stderrNorm,omitempty"`
}

// CheckEvaluatedPayload is the payload for EventCheckEvaluated.
type CheckEvaluatedPayload struct {
	ActivityDigest string       `json:"activityDigest"`
	VerifierVersion string      `json:"verifierVersion"`
	WorkspaceDigest string      `json:"workspaceDigest,omitempty"`
	Observation     Observation `json:"observation"`
}

// CompletionRequest is the client completion intent (idempotent by CompletionID).
type CompletionRequest struct {
	SchemaVersion  string         `json:"schemaVersion"`
	CompletionID   string         `json:"completionId"`
	RequestDigest  string         `json:"requestDigest"`
	Observations   []Observation  `json:"observations"`
	ArtifactIDs    []string       `json:"artifactIds,omitempty"`
	ArtifactDigests map[string]string `json:"artifactDigests,omitempty"`
	ClientTime     time.Time      `json:"clientTime"`
	Summary        string         `json:"summary,omitempty"`
}

// CompletionResult is the immutable server (or local preview) completion outcome.
type CompletionResult struct {
	SchemaVersion         string               `json:"schemaVersion"`
	CompletionID          string               `json:"completionId"`
	Accepted              bool                 `json:"accepted"`
	RequestDigest         string               `json:"requestDigest"`
	Observations          []Observation        `json:"observations"`
	EvidenceIDs           []string             `json:"evidenceIds,omitempty"`
	MasterySnapshot       []MasteryTransition  `json:"masterySnapshot,omitempty"`
	AssignmentCompletion  *AssignmentCompletion `json:"assignmentCompletion,omitempty"`
	MasteryTransitions    []MasteryTransition  `json:"masteryTransitions,omitempty"`
	Message               string               `json:"message,omitempty"`
}

// AssignmentCompletion is the assignment/session outcome independent of mastery.
type AssignmentCompletion struct {
	AssignmentID string `json:"assignmentId"`
	SessionID    string `json:"sessionId"`
	State        string `json:"state"`
	Summary      string `json:"summary,omitempty"`
}

// MasteryTransition is a server-computed mastery change snapshot (never client-set).
type MasteryTransition struct {
	StandardCode          string   `json:"standardCode"`
	FromStatus            string   `json:"fromStatus"`
	ToStatus              string   `json:"toStatus"`
	Confidence            float64  `json:"confidence,omitempty"`
	Reason                string   `json:"reason,omitempty"`
	EvidenceClass         string   `json:"evidenceClass,omitempty"`
	AcceptedEvidence      []string `json:"acceptedEvidence,omitempty"`
	MissingEvidence       []string `json:"missingEvidence,omitempty"`
	PolicySatisfied       bool     `json:"policySatisfied,omitempty"`
	StatusChanged         bool     `json:"statusChanged,omitempty"`
}

// ArtifactMeta describes an uploaded or reserved evidence artifact.
type ArtifactMeta struct {
	SchemaVersion string    `json:"schemaVersion"`
	ArtifactID    string    `json:"artifactId"`
	Filename      string    `json:"filename"`
	MediaType     string    `json:"mediaType"`
	ByteSize      int64     `json:"byteSize"`
	SHA256        string    `json:"sha256"`
	CreatedAt     time.Time `json:"createdAt"`
}

// ResponseSubmission is the student client's durable conceptual response payload.
// Idempotent by SubmissionID. Body must be the student's own text — never tutor output.
type ResponseSubmission struct {
	SchemaVersion  string    `json:"schemaVersion"`
	SubmissionID   string    `json:"submissionId"`
	TaskID         string    `json:"taskId"`
	Body           string    `json:"body"`
	ClientTime     time.Time `json:"clientTime,omitempty"`
	// RequestDigest is a stable hash of taskId+body for conflict detection.
	RequestDigest string `json:"requestDigest,omitempty"`
}

// ResponseSubmissionResult is the server outcome of an idempotent response submit.
type ResponseSubmissionResult struct {
	SchemaVersion string   `json:"schemaVersion"`
	SubmissionID  string   `json:"submissionId"`
	ResponseID    string   `json:"responseId"`
	Status        string   `json:"status"`
	EvidenceIDs   []string `json:"evidenceIds,omitempty"`
	Message       string   `json:"message,omitempty"`
	// ReviewRequired is true when the task asks for parent review.
	ReviewRequired bool `json:"reviewRequired,omitempty"`
}

// IPCEnvelope is a versioned broker↔TUI message wrapper (design stub for Phase 2+).
type IPCEnvelope struct {
	SchemaVersion string         `json:"schemaVersion"`
	Type          string         `json:"type"`
	RequestID     string         `json:"requestId,omitempty"`
	Payload       map[string]any `json:"payload,omitempty"`
}
