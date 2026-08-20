package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// StartArtifactWorker owns non-chat rubric evaluation after the browser has
// disconnected. It deliberately exposes only durable safe progress; provider
// reasoning and tool arguments never enter this worker's event surface.
func (s *Server) StartArtifactWorker(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_ = s.runArtifactStep(ctx)
			}
		}
	}()
}

func (s *Server) runArtifactStep(ctx context.Context) error {
	if s.DB == nil || s.Artifacts == nil {
		return errors.New("artifact worker is not configured")
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var job, tenant, submission string
	err = tx.QueryRow(ctx, `UPDATE artifact_rubric_jobs SET status='running',attempts=attempts+1,lease_owner=$1,lease_until=now()+interval '30 seconds',updated_at=now() WHERE id=(SELECT id FROM artifact_rubric_jobs WHERE status='queued' AND available_at<=now() ORDER BY available_at,id FOR UPDATE SKIP LOCKED LIMIT 1) RETURNING id,tenant_id,submission_id`, uuid.NewString()).Scan(&job, &tenant, &submission)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	return s.evaluateArtifact(ctx, job, tenant, submission)
}

func (s *Server) evaluateArtifact(ctx context.Context, job, tenant, submission string) error {
	var objectKey, kind, digest, occurrence, attempt string
	var rubric []byte
	err := s.DB.QueryRow(ctx, `SELECT a.object_key,a.kind,a.sha256,sub.occurrence_id,sub.attempt_id,j.rubric_snapshot FROM artifact_rubric_jobs j JOIN artifact_submissions sub ON sub.tenant_id=j.tenant_id AND sub.id=j.submission_id JOIN artifacts a ON a.tenant_id=sub.tenant_id AND a.id=sub.artifact_id WHERE j.tenant_id=$1 AND j.id=$2 AND j.submission_id=$3`, tenant, job, submission).Scan(&objectKey, &kind, &digest, &occurrence, &attempt, &rubric)
	if err != nil {
		return s.failArtifactJob(ctx, job, tenant, submission, "artifact_not_found")
	}
	// Audio/video are persisted and reviewable, but automatic evaluation is
	// capability-gated. They cannot silently pass when a provider lacks support.
	if kind != "image" || os.Getenv("TASKS_AGENT_MODE") != "scripted" || os.Getenv("TASKS_ARTIFACT_SCRIPTED_FIXTURE") != "1" {
		return s.finishArtifactReview(ctx, job, tenant, submission, "media capability requires parent review")
	}
	if expected := strings.TrimSpace(os.Getenv("TASKS_ARTIFACT_SCRIPTED_DIGEST")); expected != "" && !strings.EqualFold(expected, digest) {
		return s.failArtifactJob(ctx, job, tenant, submission, "fixture digest mismatch")
	}
	// The rubric job is authorized to load only the generated derivative, never
	// an arbitrary client key. The derivative is the multimodal fixture input.
	var derivativeKey string
	if err = s.DB.QueryRow(ctx, `SELECT object_key FROM artifact_derivatives WHERE tenant_id=$1 AND artifact_id=(SELECT artifact_id FROM artifact_submissions WHERE tenant_id=$1 AND id=$2) AND derivative_kind='thumbnail'`, tenant, submission).Scan(&derivativeKey); err != nil {
		return s.failArtifactJob(ctx, job, tenant, submission, "authorized_derivative_missing")
	}
	f, _, err := s.Artifacts.Open(ctx, derivativeKey)
	if err != nil {
		return s.failArtifactJob(ctx, job, tenant, submission, "authorized_derivative_unavailable")
	}
	_, readErr := io.Copy(io.Discard, f)
	_ = f.Close()
	if readErr != nil {
		return s.failArtifactJob(ctx, job, tenant, submission, "authorized_derivative_read_failed")
	}
	var cfg struct {
		Criteria []struct {
			ID       string `json:"id"`
			Required bool   `json:"required"`
		} `json:"criteria"`
	}
	if err = json.Unmarshal(rubric, &cfg); err != nil || len(cfg.Criteria) == 0 {
		return s.failArtifactJob(ctx, job, tenant, submission, "invalid_rubric_snapshot")
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	accepted := true
	for _, criterion := range cfg.Criteria {
		if strings.TrimSpace(criterion.ID) == "" {
			continue
		}
		status, evidence, feedback := "accepted", "The submitted image was available to the rubric reviewer.", "This criterion is present in the submitted work."
		_, err = tx.Exec(ctx, `INSERT INTO artifact_criterion_evaluations(id,tenant_id,submission_id,criterion_id,required,status,evidence,feedback,provider,model,policy_version) VALUES($1,$2,$3,$4,$5,$6,$7,$8,'scripted-fixture','scripted-multimodal','agent_artifact_rubric.v1') ON CONFLICT(tenant_id,submission_id,criterion_id) DO NOTHING`, uuid.New(), tenant, submission, criterion.ID, criterion.Required, status, evidence, feedback)
		if err != nil {
			return err
		}
		if criterion.Required && status != "accepted" {
			accepted = false
		}
	}
	if accepted {
		_, err = tx.Exec(ctx, `INSERT INTO verification_decisions(id,tenant_id,attempt_id,accepted,reason,decided_by) VALUES($1,$2,$3,true,'all required artifact rubric criteria accepted','verification_engine') ON CONFLICT(tenant_id,attempt_id) DO NOTHING`, uuid.New(), tenant, attempt)
		if err == nil {
			_, err = tx.Exec(ctx, `UPDATE verification_attempts SET status='accepted' WHERE tenant_id=$1 AND id=$2 AND status='open'`, tenant, attempt)
		}
		if err == nil {
			_, err = tx.Exec(ctx, `UPDATE task_occurrences SET status='completed' WHERE tenant_id=$1 AND id=$2 AND status NOT IN ('completed','canceled')`, tenant, occurrence)
		}
	}
	if err == nil {
		_, err = tx.Exec(ctx, `UPDATE artifact_submissions SET status='accepted' WHERE tenant_id=$1 AND id=$2`, tenant, submission)
	}
	if err == nil {
		_, err = tx.Exec(ctx, `UPDATE artifact_rubric_jobs SET status='succeeded',lease_owner=NULL,lease_until=NULL,provider='scripted-fixture',model='scripted-multimodal',updated_at=now() WHERE tenant_id=$1 AND id=$2`, tenant, job)
	}
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Server) finishArtifactReview(ctx context.Context, job, tenant, submission, reason string) error {
	_, err := s.DB.Exec(ctx, `UPDATE artifact_submissions SET status='review' WHERE tenant_id=$1 AND id=$2`, tenant, submission)
	if err == nil {
		_, err = s.DB.Exec(ctx, `UPDATE artifact_rubric_jobs SET status='review',last_error=$3,lease_owner=NULL,lease_until=NULL,updated_at=now() WHERE tenant_id=$1 AND id=$2`, tenant, job, reason)
	}
	return err
}
func (s *Server) failArtifactJob(ctx context.Context, job, tenant, submission, reason string) error {
	_, err := s.DB.Exec(ctx, `UPDATE artifact_submissions SET status='rejected' WHERE tenant_id=$1 AND id=$2`, tenant, submission)
	if err == nil {
		_, err = s.DB.Exec(ctx, `UPDATE artifact_rubric_jobs SET status='failed',last_error=$3,lease_owner=NULL,lease_until=NULL,updated_at=now() WHERE tenant_id=$1 AND id=$2`, tenant, job, reason)
	}
	return err
}
