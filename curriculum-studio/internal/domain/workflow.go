package domain

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

const (
	WorkflowStagePending   = "pending"
	WorkflowStageRunning   = "running"
	WorkflowStageSucceeded = "succeeded"
	WorkflowStageFailed    = "failed"
	WorkflowStageSkipped   = "skipped"

	WorkflowAttemptRunning   = "running"
	WorkflowAttemptSucceeded = "succeeded"
	WorkflowAttemptFailed    = "failed"
)

// WorkflowStage is a durable, resumable materialization checkpoint.
type WorkflowStage struct {
	ID             uuid.UUID       `json:"id" db:"id"`
	RunID          uuid.UUID       `json:"runId" db:"run_id"`
	StageKey       string          `json:"stageKey" db:"stage_key"`
	Position       int             `json:"position" db:"position"`
	Status         string          `json:"status" db:"status"`
	Input          json.RawMessage `json:"input" db:"input"`
	Output         json.RawMessage `json:"output" db:"output"`
	LeaseOwner     string          `json:"leaseOwner,omitempty" db:"lease_owner"`
	LeaseExpiresAt *time.Time      `json:"leaseExpiresAt,omitempty" db:"lease_expires_at"`
	FenceToken     int64           `json:"fenceToken" db:"fence_token"`
	StartedAt      *time.Time      `json:"startedAt,omitempty" db:"started_at"`
	CompletedAt    *time.Time      `json:"completedAt,omitempty" db:"completed_at"`
	CreatedAt      time.Time       `json:"createdAt" db:"created_at"`
	UpdatedAt      time.Time       `json:"updatedAt" db:"updated_at"`
}

// WorkflowAttempt records one worker attempt against a stage.
type WorkflowAttempt struct {
	ID            uuid.UUID  `json:"id" db:"id"`
	StageID       uuid.UUID  `json:"stageId" db:"stage_id"`
	AttemptNumber int        `json:"attemptNumber" db:"attempt_number"`
	Status        string     `json:"status" db:"status"`
	Agent         string     `json:"agent" db:"agent"`
	Error         string     `json:"error" db:"error"`
	StartedAt     time.Time  `json:"startedAt" db:"started_at"`
	FinishedAt    *time.Time `json:"finishedAt,omitempty" db:"finished_at"`
}

// WorkflowStageSpec is the idempotent input to EnsureStages.
type WorkflowStageSpec struct {
	StageKey string
	Position int
	Input    json.RawMessage
}

// ClaimedStage contains the stage lease and its newly opened attempt.
type ClaimedStage struct {
	Stage   WorkflowStage
	Attempt WorkflowAttempt
}
