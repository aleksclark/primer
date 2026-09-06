package api

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"strconv"
	"time"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"primer-tasks/internal/agent"
	"primer-tasks/internal/domain/parent"
	"primer-tasks/internal/jobs"
	"primer-tasks/internal/verification"
)

// StartDialogueWorker owns durable work independently of socket/request life.
// PostgreSQL stages/leases, not a goroutine map, make another process resumable.
func (s *Server) StartDialogueWorker(ctx context.Context) { go s.runDialogueWorker(ctx) }
func (s *Server) runDialogueWorker(ctx context.Context) {
	queue := jobs.NewPostgresRepository(s.DB)
	owner := "dialogue-" + uuid.NewString()
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	var lastWarning time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		claimCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		job, ok, err := queue.ClaimDialogue(claimCtx, owner)
		cancel()
		if err != nil {
			if time.Since(lastWarning) > 10*time.Second {
				slog.Warn("student dialogue queue unavailable")
				lastWarning = time.Now()
			}
			continue
		}
		if !ok {
			continue
		}
		err = s.runDialogueJob(ctx, queue, job)
		if ctx.Err() != nil {
			return
		} // Leave the durable stage for lease recovery.
		if err != nil {
			code := "provider_unavailable"
			if errors.Is(err, verification.ErrDialogueRevoked) {
				code = "revoked"
			}
			if errors.Is(err, verification.ErrDialogueTerminal) || errors.Is(err, verification.ErrDialogueLimit) {
				code = "exhausted"
			}
			failCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			failErr := queue.FailDialogue(failCtx, job, code)
			cancel()
			if failErr != nil && !errors.Is(failErr, jobs.ErrDialogueLeaseLost) {
				slog.Warn("student dialogue failure persistence unavailable")
			}
		}
	}
}

type dialogueBackend struct {
	engine              verification.DialogueEngine
	authority           verification.StudentAuthority
	occurrence, attempt string
	lease               verification.DialogueLease
}

func (b dialogueBackend) State(ctx context.Context) (verification.DialogueState, string, string, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	return b.engine.WorkerState(ctx, b.authority, b.occurrence, b.attempt, b.lease)
}
func (b dialogueBackend) Progress(ctx context.Context, phase string) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	return b.engine.Progress(ctx, b.authority, b.occurrence, b.attempt, b.lease, phase)
}

func (s *Server) runDialogueJob(parentCtx context.Context, queue *jobs.PostgresRepository, job jobs.DialogueJob) error {
	ctx, cancel := context.WithDeadline(parentCtx, job.Deadline)
	defer cancel()
	renewDone := make(chan struct{})
	go func() {
		defer close(renewDone)
		ticker := time.NewTicker(jobs.DialogueLeaseDuration / 3)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				check, done := context.WithTimeout(ctx, 2*time.Second)
				err := queue.RenewDialogue(check, job)
				done()
				if err != nil {
					cancel()
					return
				}
			}
		}
	}()
	defer func() { cancel(); <-renewDone }()
	backend := dialogueBackend{engine: verification.DialogueEngine{DB: s.DB}, authority: verification.StudentAuthority{TenantID: job.TenantID, StudentID: job.StudentID, SessionID: job.SessionID}, occurrence: job.OccurrenceID, attempt: job.AttemptID, lease: verification.DialogueLease{JobID: job.ID, Owner: job.LeaseOwner, Generation: job.LeaseGeneration}}
	cfg, err := parent.LoadProviderConfig()
	if err != nil {
		return err
	}
	limits := agent.Limits{MaxSteps: cfg.MaxSteps, MaxTokens: cfg.MaxTokens, MaxRetries: cfg.MaxRetries, Deadline: cfg.MaxDuration}
	// At most evaluation -> next question. Both are independently durable
	// stages, so death between them does not rerun the evaluated answer.
	for step := 0; step < 2; step++ {
		state, stage, message, err := backend.State(ctx)
		if err != nil {
			return err
		}
		var model fantasy.LanguageModel
		switch cfg.Mode {
		case parent.ProviderDisabled:
			return agent.ErrProviderDisabled
		case parent.ProviderScripted:
			if s.Env == "production" || cfg.Environment == "production" {
				return agent.ErrProviderDisabled
			}
			delay := 0
			if raw := os.Getenv("TASKS_AGENT_SCRIPTED_DIALOGUE_DELAY_MS"); raw != "" {
				delay, err = strconv.Atoi(raw)
				if err != nil {
					return err
				}
			}
			model, err = agent.NewScriptedDialogueModel(state, stage, message, os.Getenv("TASKS_AGENT_SCRIPTED_DIALOGUE_FAULT"), time.Duration(delay)*time.Millisecond)
		default:
			model, err = s.agentModel(ctx, cfg, "")
		}
		if err != nil {
			return err
		}
		execution, err := agent.RunDialogue(ctx, model, backend, limits)
		if err != nil {
			return err
		}
		commitCtx, done := context.WithTimeout(ctx, 3*time.Second)
		if execution.Stage == "question" {
			err = backend.engine.CommitQuestion(commitCtx, backend.authority, backend.occurrence, backend.attempt, backend.lease, execution.QuestionKey)
		} else {
			err = backend.engine.CommitEvaluation(commitCtx, backend.authority, backend.occurrence, backend.attempt, backend.lease, execution.Evaluation)
		}
		done()
		if err != nil {
			return err
		}
		var status string
		if err = s.DB.QueryRow(ctx, `SELECT status FROM verification_jobs WHERE id=$1`, job.ID).Scan(&status); err != nil {
			return err
		}
		if status == "succeeded" {
			return nil
		}
	}
	return verification.ErrDialogueConflict
}
