package api

import (
	"context"
	"time"

	"github.com/aleksclark/primer/agents/internal/domain"
	"github.com/aleksclark/primer/agents/internal/repo"
)

// AgentService is the application-service interface the API routes depend on.
type AgentService interface {
	CreateRun(ctx context.Context, cmd CreateRunCmd) (*domain.Run, error)
	GetRun(ctx context.Context, id, namespace string) (*domain.Run, error)
	ListRuns(ctx context.Context, namespace string, limit int) ([]*domain.Run, error)
	RequestCancel(ctx context.Context, id, namespace string, reasonClass *string) (*domain.Run, error)
	ListEvents(ctx context.Context, runID, namespace string, afterSeq int64, limit int) ([]*domain.RunEvent, error)
	CreateSession(ctx context.Context, cmd CreateSessionCmd) (*domain.Session, error)
	GetSession(ctx context.Context, id, namespace string) (*domain.Session, error)
	AppendTurn(ctx context.Context, cmd AppendTurnCmd) (*repo.AppendTurnResult, error)
	ListTurns(ctx context.Context, sessionID, namespace string, limit int) ([]*domain.SessionTurn, error)
}

// CreateRunCmd mirrors appservice.CreateRunCmd.
type CreateRunCmd struct {
	OwnerNamespace string
	IdempotencyKey string
	Profile        string
	InputContent   []byte
	InputPreview   *string
	SessionID      *string
}

// CreateSessionCmd mirrors appservice.CreateSessionCmd.
type CreateSessionCmd struct {
	OwnerNamespace string
	Profile        string
	CallerContext  *string
	ExpiresAt      *time.Time
}

// AppendTurnCmd mirrors appservice.AppendTurnCmd.
type AppendTurnCmd struct {
	SessionID        string
	OwnerNamespace   string
	IdempotencyKey   string
	Profile          string
	InputPreview     *string
	ExpectedRevision int64
}
