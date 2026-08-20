package api

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"time"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"primer-tasks/internal/agent"
	agentprotocol "primer-tasks/internal/agent/protocol"
	"primer-tasks/internal/domain/parent"
	"primer-tasks/internal/verification"
)

// StartArtifactWorker owns non-chat rubric evaluation after the browser has
// disconnected. Only safe, durable progress is emitted; provider reasoning
// and tool arguments never enter the event surface.
func (s *Server) StartArtifactCleanup(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()
		_ = s.CleanupArtifactOrphans(ctx, time.Now().UTC())
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_ = s.CleanupArtifactOrphans(ctx, time.Now().UTC())
			}
		}
	}()
}

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

type artifactLeaseOwnerKey struct{}
type artifactLeaseJobKey struct{}

func withArtifactLease(ctx context.Context, owner, job string) context.Context {
	return context.WithValue(context.WithValue(ctx, artifactLeaseOwnerKey{}, owner), artifactLeaseJobKey{}, job)
}
func artifactLeaseOwner(ctx context.Context) string {
	owner, _ := ctx.Value(artifactLeaseOwnerKey{}).(string)
	return owner
}
func artifactLeaseJob(ctx context.Context) string {
	job, _ := ctx.Value(artifactLeaseJobKey{}).(string)
	return job
}

var errArtifactLeaseLost = errors.New("artifact lease expired or stolen")

// artifactLeaseWhere is deliberately repeated in every worker-owned mutation.
// A check followed by an unqualified UPDATE is not sufficient: another worker
// may reclaim the row between those two statements.
const artifactLeaseWhere = `tenant_id=$1 AND id=$2 AND status='running' AND lease_owner=$3 AND lease_until>now()`

func (s *Server) requireArtifactLease(ctx context.Context, tenant, job string) error {
	owner := artifactLeaseOwner(ctx)
	if owner == "" {
		return errors.New("artifact lease owner missing")
	}
	return requireArtifactLeaseWithOwner(ctx, s.DB, tenant, job, owner)
}

func requireArtifactLeaseWithOwner(ctx context.Context, db interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, tenant, job, owner string) error {
	var active bool
	if err := db.QueryRow(ctx, `SELECT status='running' AND lease_owner=$3 AND lease_until>now() FROM artifact_rubric_jobs WHERE tenant_id=$1 AND id=$2`, tenant, job, owner).Scan(&active); err != nil {
		return err
	}
	if !active {
		return errArtifactLeaseLost
	}
	return nil
}

func (s *Server) runArtifactStep(ctx context.Context) error {
	if s.DB == nil || s.Artifacts == nil {
		return errors.New("artifact worker is not configured")
	}
	if _, err := s.DB.Exec(ctx, `UPDATE artifact_rubric_jobs SET status='queued',lease_owner=NULL,lease_until=NULL,updated_at=now() WHERE status='running' AND lease_until<now()`); err != nil {
		return err
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var job, tenant, submission string
	owner := uuid.NewString()
	err = tx.QueryRow(ctx, `UPDATE artifact_rubric_jobs SET status='running',attempts=attempts+1,lease_owner=$1,lease_until=now()+interval '5 minutes',updated_at=now() WHERE id=(SELECT id FROM artifact_rubric_jobs WHERE status='queued' AND available_at<=now() AND attempts<max_attempts ORDER BY available_at,id FOR UPDATE SKIP LOCKED LIMIT 1) RETURNING id,tenant_id,submission_id`, owner).Scan(&job, &tenant, &submission)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if err = appendArtifactProgressTx(ctx, tx, tenant, job, submission, owner, "started", map[string]any{"phase": "evaluating"}); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	return s.evaluateArtifact(withArtifactLease(ctx, owner, job), job, tenant, submission)
}

func (s *Server) evaluateArtifact(ctx context.Context, job, tenant, submission string) error {
	var objectKey, kind, digest, occurrence, attempt string
	var rubricBytes []byte
	err := s.DB.QueryRow(ctx, `SELECT a.object_key,a.kind,a.sha256,sub.occurrence_id,sub.attempt_id,j.rubric_snapshot FROM artifact_rubric_jobs j JOIN artifact_submissions sub ON sub.tenant_id=j.tenant_id AND sub.id=j.submission_id JOIN artifacts a ON a.tenant_id=sub.tenant_id AND a.id=sub.artifact_id WHERE j.tenant_id=$1 AND j.id=$2 AND j.submission_id=$3`, tenant, job, submission).Scan(&objectKey, &kind, &digest, &occurrence, &attempt, &rubricBytes)
	if err != nil {
		return s.failArtifactJob(ctx, job, tenant, submission, "artifact_not_found")
	}
	rubric, err := verification.ParseArtifactRubric(rubricBytes)
	if err != nil {
		return s.failArtifactJob(ctx, job, tenant, submission, "invalid_rubric_snapshot")
	}
	if kind != "image" {
		return s.resolveArtifactPolicy(ctx, job, tenant, submission, rubric, "media capability requires parent review")
	}

	derivativeKey, derivativeType, derivativeDigest, err := s.authorizedDerivative(ctx, tenant, submission)
	if err != nil {
		return s.failArtifactJob(ctx, job, tenant, submission, "authorized_derivative_missing")
	}
	f, _, err := s.Artifacts.Open(ctx, derivativeKey)
	if err != nil {
		return s.failArtifactJob(ctx, job, tenant, submission, "authorized_derivative_unavailable")
	}
	derivativeBytes, readErr := io.ReadAll(io.LimitReader(f, 32<<20))
	_ = f.Close()
	if readErr != nil {
		return s.failArtifactJob(ctx, job, tenant, submission, "authorized_derivative_read_failed")
	}
	computed := sha256.Sum256(derivativeBytes)
	if derivativeDigest == "" || !strings.EqualFold(derivativeDigest, hex.EncodeToString(computed[:])) {
		return s.failArtifactJob(ctx, job, tenant, submission, "authorized_derivative_digest_mismatch")
	}

	var cfg parent.ProviderConfig
	if os.Getenv("TASKS_AGENT_MODE") == string(parent.ProviderScripted) && os.Getenv("TASKS_ARTIFACT_SCRIPTED_FIXTURE") == "1" {
		// Scripted qualification is intentionally self-contained and does not
		// require the live provider allowlist or credentials.
		cfg.Mode = parent.ProviderScripted
	} else {
		cfg, err = parent.LoadProviderConfig()
		if err != nil {
			return s.failArtifactJob(ctx, job, tenant, submission, "provider_configuration_invalid")
		}
	}
	model, provider, err := s.artifactModel(ctx, cfg, digest, derivativeBytes, rubric)
	if err != nil {
		return s.resolveArtifactPolicy(ctx, job, tenant, submission, rubric, err.Error())
	}
	if !artifactProviderSupportsFiles(provider) {
		return s.resolveArtifactPolicy(ctx, job, tenant, submission, rubric, "provider does not support authorized artifact files")
	}
	if err := s.appendArtifactProgress(ctx, tenant, job, submission, "provider_started", map[string]any{"provider": provider, "model": model.Model()}); err != nil {
		return err
	}

	backend := artifactRubricBackend{server: s}
	scope := verification.ArtifactContext{TenantID: tenant, JobID: job, SubmissionID: submission, Rubric: rubric, Provider: model.Provider(), Model: model.Model(), PolicyVersion: verification.ArtifactRubricPolicyVersion}
	tools, err := agent.NewArtifactRubricTools(backend, scope)
	if err != nil {
		return err
	}
	rt, err := agent.NewFantasyAgent(model, tools, agent.Limits{MaxSteps: maxArtifactSteps(len(rubric.Criteria)), MaxTokens: 4096, Deadline: 2 * time.Minute, MaxRetries: 0})
	if err != nil {
		return err
	}
	prompt := artifactPrompt(rubricBytes, digest)
	_, runErr := rt.ExecuteFiles(ctx, job, prompt, []fantasy.FilePart{{Filename: "authorized-derivative", Data: derivativeBytes, MediaType: derivativeType}}, func(e agentprotocol.Event) error {
		if e.Kind == agentprotocol.EventToolProgress && e.Phase == "completed" {
			return s.appendArtifactProgress(ctx, tenant, job, submission, "criterion_recorded", map[string]any{"label": e.Label})
		}
		return nil
	})
	if runErr != nil {
		return s.retryOrResolveArtifact(ctx, job, tenant, submission, rubric, "provider evaluation failed")
	}

	results, err := s.artifactCriterionResults(ctx, tenant, submission)
	if err != nil {
		return err
	}
	ready, err := verification.EvaluateArtifact(rubric, results)
	if err != nil {
		return s.failArtifactJob(ctx, job, tenant, submission, "invalid_provider_rubric_result")
	}
	if !ready.Ready {
		return s.retryOrResolveArtifact(ctx, job, tenant, submission, rubric, "provider returned an incomplete rubric")
	}
	inserted, err := verification.CommitDecision(ctx, s, verification.Decision{ID: uuid.NewString(), TenantID: tenant, AttemptID: attempt, OccurrenceID: occurrence, Accepted: ready.Accepted, Reason: ready.Reason, DecidedBy: "verification_engine"})
	if err != nil {
		return err
	}
	if !inserted {
		return s.resolveArtifactPolicy(ctx, job, tenant, submission, rubric, "verification decision already exists")
	}
	if err := s.finishArtifactDecision(ctx, job, tenant, submission, model.Provider(), model.Model(), ready.Accepted, inserted); err != nil {
		return err
	}
	return nil
}

func maxArtifactSteps(criteria int) int {
	if criteria < 1 {
		return 4
	}
	return criteria + 2
}

func artifactPrompt(rubric []byte, digest string) string {
	return "Evaluate only the authorized derivative with the parent-authored rubric. Use record_artifact_criterion once for each criterion; never claim completion.\nRUBRIC=" + string(rubric) + "\nARTIFACT_DIGEST=" + digest
}

func artifactProviderSupportsFiles(provider string) bool {
	// Fantasy's OpenRouter adapter maps FilePart; the current Bedrock adapter
	// does not. Scripted is a byte-checking qualification fixture, not a live
	// provider.
	return provider == "openrouter" || provider == "scripted"
}

func (s *Server) artifactModel(ctx context.Context, cfg parent.ProviderConfig, digest string, derivative []byte, rubric verification.ArtifactRubric) (fantasy.LanguageModel, string, error) {
	if cfg.Mode == parent.ProviderScripted && os.Getenv("TASKS_ARTIFACT_SCRIPTED_FIXTURE") == "1" {
		expected := strings.TrimSpace(os.Getenv("TASKS_ARTIFACT_SCRIPTED_DIGEST"))
		if expected == "" || !strings.EqualFold(expected, digest) {
			return nil, "", errors.New("scripted fixture digest mismatch")
		}
		outcome := strings.TrimSpace(os.Getenv("TASKS_ARTIFACT_SCRIPTED_OUTCOME"))
		return &scriptedArtifactModel{expectedDigest: digest, expectedDerivative: append([]byte(nil), derivative...), criteria: rubric.Criteria, accepted: outcome != "negative", fault: strings.TrimSpace(os.Getenv("TASKS_ARTIFACT_SCRIPTED_FAULT"))}, "scripted", nil
	}
	// Live multimodal qualification is an explicit opt-in and remains blocked
	// by default. This prevents a production provider from being treated as a
	// qualified evaluator merely because it returned a tool call.
	if os.Getenv("TASKS_ARTIFACT_LIVE_QUALIFICATION") != "1" {
		return nil, "", errors.New("live artifact model qualification is blocked")
	}
	model, err := s.agentModel(ctx, cfg, "artifact qualification")
	if err != nil {
		return nil, "", err
	}
	return model, model.Provider(), nil
}

func (s *Server) authorizedDerivative(ctx context.Context, tenant, submission string) (key, contentType, digest string, err error) {
	err = s.DB.QueryRow(ctx, `SELECT d.object_key,d.content_type,d.sha256 FROM artifact_derivatives d JOIN artifact_submissions sub ON sub.tenant_id=d.tenant_id AND sub.artifact_id=d.artifact_id WHERE d.tenant_id=$1 AND sub.id=$2 AND d.derivative_kind='thumbnail'`, tenant, submission).Scan(&key, &contentType, &digest)
	return
}

type artifactRubricBackend struct{ server *Server }

func (b artifactRubricBackend) RecordArtifactCriterion(ctx context.Context, scope verification.ArtifactContext, result verification.ArtifactCriterionResult) (verification.ArtifactCriterionResult, bool, error) {
	if err := b.server.requireArtifactLease(ctx, scope.TenantID, scope.JobID); err != nil {
		return verification.ArtifactCriterionResult{}, false, err
	}
	owner := artifactLeaseOwner(ctx)
	var stored verification.ArtifactCriterionResult
	var inserted bool
	tag, err := b.server.DB.Exec(ctx, `INSERT INTO artifact_criterion_evaluations(id,tenant_id,submission_id,criterion_id,required,status,evidence,feedback,provider,model,policy_version) SELECT $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11 WHERE EXISTS (SELECT 1 FROM artifact_rubric_jobs WHERE tenant_id=$12 AND id=$13 AND status='running' AND lease_owner=$14 AND lease_until>now()) ON CONFLICT(tenant_id,submission_id,criterion_id) DO NOTHING`, uuid.New(), scope.TenantID, scope.SubmissionID, result.CriterionID, result.Required, result.Status, result.Evidence, result.Feedback, scope.Provider, scope.Model, scope.PolicyVersion, scope.TenantID, scope.JobID, owner)
	if err != nil {
		return stored, false, err
	}
	if tag.RowsAffected() == 0 {
		if leaseErr := b.server.requireArtifactLease(ctx, scope.TenantID, scope.JobID); leaseErr != nil {
			return stored, false, leaseErr
		}
		err = b.server.DB.QueryRow(ctx, `SELECT criterion_id,required,status,evidence,feedback FROM artifact_criterion_evaluations WHERE tenant_id=$1 AND submission_id=$2 AND criterion_id=$3`, scope.TenantID, scope.SubmissionID, result.CriterionID).Scan(&stored.CriterionID, &stored.Required, &stored.Status, &stored.Evidence, &stored.Feedback)
		return stored, false, err
	}
	inserted = true
	if err = b.server.DB.QueryRow(ctx, `SELECT criterion_id,required,status,evidence,feedback FROM artifact_criterion_evaluations WHERE tenant_id=$1 AND submission_id=$2 AND criterion_id=$3`, scope.TenantID, scope.SubmissionID, result.CriterionID).Scan(&stored.CriterionID, &stored.Required, &stored.Status, &stored.Evidence, &stored.Feedback); err != nil {
		return stored, false, err
	}
	if err = b.server.appendArtifactProgress(ctx, scope.TenantID, scope.JobID, scope.SubmissionID, "criterion", map[string]any{"criterionId": result.CriterionID, "status": result.Status}); err != nil {
		return stored, false, err
	}
	return stored, inserted, nil
}

func (s *Server) artifactCriterionResults(ctx context.Context, tenant, submission string) ([]verification.ArtifactCriterionResult, error) {
	rows, err := s.DB.Query(ctx, `SELECT criterion_id,required,status,evidence,feedback FROM artifact_criterion_evaluations WHERE tenant_id=$1 AND submission_id=$2 ORDER BY created_at,criterion_id`, tenant, submission)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var results []verification.ArtifactCriterionResult
	for rows.Next() {
		var result verification.ArtifactCriterionResult
		if err := rows.Scan(&result.CriterionID, &result.Required, &result.Status, &result.Evidence, &result.Feedback); err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	return results, rows.Err()
}

func (s *Server) finishArtifactDecision(ctx context.Context, job, tenant, submission, provider, model string, accepted, inserted bool) error {
	if err := s.requireArtifactLease(ctx, tenant, job); err != nil {
		return err
	}
	owner := artifactLeaseOwner(ctx)
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	status := "rejected"
	if accepted {
		status = "accepted"
	}
	var tag pgconn.CommandTag
	tag, err = tx.Exec(ctx, `UPDATE artifact_submissions SET status=$3 WHERE tenant_id=$1 AND id=$2 AND EXISTS (SELECT 1 FROM artifact_rubric_jobs j WHERE j.tenant_id=$4 AND j.id=$5 AND j.status='running' AND j.lease_owner=$6 AND j.lease_until>now() AND j.submission_id=$7)`, tenant, submission, status, tenant, job, owner, submission)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return errArtifactLeaseLost
	}
	if err = appendArtifactProgressTx(ctx, tx, tenant, job, submission, owner, "complete", map[string]any{"accepted": accepted, "decisionInserted": inserted}); err != nil {
		return err
	}
	tag, err = tx.Exec(ctx, `UPDATE artifact_rubric_jobs SET status='succeeded',provider=$4,model=$5,lease_owner=NULL,lease_until=NULL,last_error='',updated_at=now() WHERE `+artifactLeaseWhere, tenant, job, owner, provider, model)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return errArtifactLeaseLost
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	_ = s.publishArtifactProgress(ctx, tenant, job, submission, "complete", map[string]any{"accepted": accepted, "decisionInserted": inserted})
	return nil
}

func (s *Server) retryOrResolveArtifact(ctx context.Context, job, tenant, submission string, rubric verification.ArtifactRubric, reason string) error {
	if err := s.requireArtifactLease(ctx, tenant, job); err != nil {
		return err
	}
	owner := artifactLeaseOwner(ctx)
	var attempts, maxAttempts int
	if err := s.DB.QueryRow(ctx, `SELECT attempts,max_attempts FROM artifact_rubric_jobs WHERE tenant_id=$1 AND id=$2`, tenant, job).Scan(&attempts, &maxAttempts); err != nil {
		return err
	}
	if attempts < maxAttempts {
		tx, err := s.DB.Begin(ctx)
		if err != nil {
			return err
		}
		defer tx.Rollback(ctx)
		// Progress must be written before the lease is cleared. Keeping both
		// mutations in this transaction prevents a stale retry from leaving a
		// durable event behind after reclaim.
		if err = appendArtifactProgressTx(ctx, tx, tenant, job, submission, owner, "retry", map[string]any{"attempt": attempts, "reason": reason}); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `UPDATE artifact_rubric_jobs SET status='queued',available_at=now()+interval '1 second',lease_owner=NULL,lease_until=NULL,last_error=$4,updated_at=now() WHERE `+artifactLeaseWhere, tenant, job, owner, reason)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return errArtifactLeaseLost
		}
		if err = tx.Commit(ctx); err != nil {
			return err
		}
		_ = s.publishArtifactProgress(ctx, tenant, job, submission, "retry", map[string]any{"attempt": attempts, "reason": reason})
		return nil
	}
	return s.resolveArtifactPolicy(ctx, job, tenant, submission, rubric, reason)
}

func (s *Server) resolveArtifactPolicy(ctx context.Context, job, tenant, submission string, rubric verification.ArtifactRubric, reason string) error {
	if err := s.requireArtifactLease(ctx, tenant, job); err != nil {
		return err
	}
	owner := artifactLeaseOwner(ctx)
	status, jobStatus := "rejected", "failed"
	phase := "rejected"
	if rubric.ReviewPolicy == "parent_review" {
		status, jobStatus, phase = "review", "review", "review"
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var tag pgconn.CommandTag
	tag, err = tx.Exec(ctx, `UPDATE artifact_submissions SET status=$3 WHERE tenant_id=$1 AND id=$2 AND EXISTS (SELECT 1 FROM artifact_rubric_jobs j WHERE j.tenant_id=$4 AND j.id=$5 AND j.status='running' AND j.lease_owner=$6 AND j.lease_until>now() AND j.submission_id=$7)`, tenant, submission, status, tenant, job, owner, submission)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return errArtifactLeaseLost
	}
	if err = appendArtifactProgressTx(ctx, tx, tenant, job, submission, owner, phase, map[string]any{"reason": reason, "reviewPolicy": rubric.ReviewPolicy}); err != nil {
		return err
	}
	tag, err = tx.Exec(ctx, `UPDATE artifact_rubric_jobs SET status=$4,last_error=$5,lease_owner=NULL,lease_until=NULL,updated_at=now() WHERE `+artifactLeaseWhere, tenant, job, owner, jobStatus, reason)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return errArtifactLeaseLost
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	_ = s.publishArtifactProgress(ctx, tenant, job, submission, phase, map[string]any{"reason": reason, "reviewPolicy": rubric.ReviewPolicy})
	return nil
}

// Kept as a small compatibility helper for existing operational callers.
// It is still a worker mutation and therefore requires the exact lease.
func (s *Server) finishArtifactReview(ctx context.Context, job, tenant, submission, reason string) error {
	if err := s.requireArtifactLease(ctx, tenant, job); err != nil {
		return err
	}
	owner := artifactLeaseOwner(ctx)
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var tag pgconn.CommandTag
	tag, err = tx.Exec(ctx, `UPDATE artifact_submissions SET status='review' WHERE tenant_id=$1 AND id=$2 AND EXISTS (SELECT 1 FROM artifact_rubric_jobs j WHERE j.tenant_id=$3 AND j.id=$4 AND j.status='running' AND j.lease_owner=$5 AND j.lease_until>now() AND j.submission_id=$6)`, tenant, submission, tenant, job, owner, submission)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return errArtifactLeaseLost
	}
	if err = appendArtifactProgressTx(ctx, tx, tenant, job, submission, owner, "review", map[string]any{"reason": reason}); err != nil {
		return err
	}
	if _, err = execArtifactJobStatusTx(ctx, tx, job, tenant, "review", reason, owner); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	_ = s.publishArtifactProgress(ctx, tenant, job, submission, "review", map[string]any{"reason": reason})
	return nil
}

func (s *Server) failArtifactJob(ctx context.Context, job, tenant, submission, reason string) error {
	var raw []byte
	if err := s.DB.QueryRow(ctx, `SELECT rubric_snapshot FROM artifact_rubric_jobs WHERE tenant_id=$1 AND id=$2`, tenant, job).Scan(&raw); err == nil {
		if rubric, parseErr := verification.ParseArtifactRubric(raw); parseErr == nil {
			return s.resolveArtifactPolicy(ctx, job, tenant, submission, rubric, reason)
		}
	}
	if err := s.requireArtifactLease(ctx, tenant, job); err != nil {
		return err
	}
	owner := artifactLeaseOwner(ctx)
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var tag pgconn.CommandTag
	tag, err = tx.Exec(ctx, `UPDATE artifact_submissions SET status='rejected' WHERE tenant_id=$1 AND id=$2 AND EXISTS (SELECT 1 FROM artifact_rubric_jobs j WHERE j.tenant_id=$3 AND j.id=$4 AND j.status='running' AND j.lease_owner=$5 AND j.lease_until>now() AND j.submission_id=$6)`, tenant, submission, tenant, job, owner, submission)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return errArtifactLeaseLost
	}
	if err = appendArtifactProgressTx(ctx, tx, tenant, job, submission, owner, "rejected", map[string]any{"reason": reason}); err != nil {
		return err
	}
	if _, err = execArtifactJobStatusTx(ctx, tx, job, tenant, "failed", reason, owner); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	_ = s.publishArtifactProgress(ctx, tenant, job, submission, "rejected", map[string]any{"reason": reason})
	return nil
}

func execArtifactJobStatusTx(ctx context.Context, tx pgx.Tx, job, tenant, status, reason, owner string) (int64, error) {
	tag, err := tx.Exec(ctx, `UPDATE artifact_rubric_jobs SET status=$4,last_error=$5,lease_owner=NULL,lease_until=NULL,updated_at=now() WHERE `+artifactLeaseWhere, tenant, job, owner, status, reason)
	if err != nil {
		return 0, err
	}
	if tag.RowsAffected() != 1 {
		return 0, errArtifactLeaseLost
	}
	return tag.RowsAffected(), nil
}

func appendArtifactProgressTx(ctx context.Context, tx pgx.Tx, tenant, job, submission, owner, kind string, payload map[string]any) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	var sequence int64
	if err = tx.QueryRow(ctx, `UPDATE artifact_rubric_jobs SET progress_sequence=progress_sequence+1 WHERE `+artifactLeaseWhere+` RETURNING progress_sequence`, tenant, job, owner).Scan(&sequence); err != nil {
		if err == pgx.ErrNoRows {
			return errArtifactLeaseLost
		}
		return err
	}
	tag, err := tx.Exec(ctx, `INSERT INTO artifact_rubric_events(tenant_id,job_id,submission_id,sequence,kind,payload) SELECT $1,$2,$4,$5,$6,$7 WHERE EXISTS (SELECT 1 FROM artifact_rubric_jobs WHERE `+artifactLeaseWhere+`)`, tenant, job, owner, submission, sequence, kind, b)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return errArtifactLeaseLost
	}
	return nil
}

func (s *Server) appendArtifactProgress(ctx context.Context, tenant, job, submission, kind string, payload map[string]any) error {
	if err := s.requireArtifactLease(ctx, tenant, job); err != nil {
		return err
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err = appendArtifactProgressTx(ctx, tx, tenant, job, submission, artifactLeaseOwner(ctx), kind, payload); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	return s.publishArtifactProgress(ctx, tenant, job, submission, kind, payload)
}

func (s *Server) publishArtifactProgress(ctx context.Context, tenant, job, submission, kind string, payload map[string]any) error {
	var occurrence, student string
	if err := s.DB.QueryRow(ctx, `SELECT occurrence_id,student_id FROM artifact_submissions WHERE tenant_id=$1 AND id=$2`, tenant, submission).Scan(&occurrence, &student); err == nil {
		eventType := "progress"
		if kind == "complete" || kind == "rejected" {
			eventType = "complete"
		}
		var sequence int64
		_ = s.DB.QueryRow(ctx, `SELECT progress_sequence FROM artifact_rubric_jobs WHERE tenant_id=$1 AND id=$2`, tenant, job).Scan(&sequence)
		status := "evaluating"
		if kind == "complete" {
			if accepted, ok := payload["accepted"].(bool); ok && accepted {
				status = "accepted"
			} else {
				status = "rejected"
			}
		}
		event := wireStudentEvent{Type: eventType, ProtocolVersion: studentProtocolVersion, TenantID: tenant, StudentID: student, SubmissionID: submission, OccurrenceID: occurrence, Sequence: sequence, Cursor: sequence, Phase: kind, Status: status, Time: time.Now().UTC()}
		payloadBytes, _ := json.Marshal(payload)
		_ = json.Unmarshal(payloadBytes, &event)
		s.studentDialogueHub().publish(event)
	}
	return nil
}

// CommitDecision is the only artifact completion authority. The INSERT and
// attempt/occurrence transition are one transaction and the unique constraint
// makes retries/replays idempotent.
func (s *Server) CommitDecision(ctx context.Context, decision verification.Decision) (bool, error) {
	var kind string
	if err := s.DB.QueryRow(ctx, `SELECT r.kind FROM verification_attempts a JOIN verification_requirements r ON r.tenant_id=a.tenant_id AND r.id=a.requirement_id WHERE a.tenant_id=$1 AND a.id=$2`, decision.TenantID, decision.AttemptID).Scan(&kind); err == nil && kind == verification.ExternalCallbackKind {
		return s.commitExternalDecision(ctx, decision)
	}
	job := artifactLeaseJob(ctx)
	if err := s.requireArtifactLease(ctx, decision.TenantID, job); err != nil {
		return false, err
	}
	owner := artifactLeaseOwner(ctx)
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	var tag pgconn.CommandTag
	tag, err = tx.Exec(ctx, `INSERT INTO verification_decisions(id,tenant_id,attempt_id,accepted,reason,decided_by) SELECT $1,$2,$3,$4,$5,$6 WHERE EXISTS (SELECT 1 FROM artifact_rubric_jobs WHERE tenant_id=$7 AND id=$8 AND status='running' AND lease_owner=$9 AND lease_until>now()) ON CONFLICT(tenant_id,attempt_id) DO NOTHING`, decision.ID, decision.TenantID, decision.AttemptID, decision.Accepted, decision.Reason, "verification_engine", decision.TenantID, job, owner)
	if err != nil {
		return false, err
	}
	if tag.RowsAffected() == 0 {
		if leaseErr := requireArtifactLeaseWithOwner(ctx, tx, decision.TenantID, job, owner); leaseErr != nil {
			return false, leaseErr
		}
		return false, nil
	}
	attemptStatus, occurrenceStatus := "rejected", "pending"
	if decision.Accepted {
		attemptStatus, occurrenceStatus = "accepted", "completed"
	}
	tag, err = tx.Exec(ctx, `UPDATE verification_attempts SET status=$3 WHERE tenant_id=$1 AND id=$2 AND status='open' AND EXISTS (SELECT 1 FROM artifact_rubric_jobs j WHERE j.tenant_id=$4 AND j.id=$5 AND j.status='running' AND j.lease_owner=$6 AND j.lease_until>now())`, decision.TenantID, decision.AttemptID, attemptStatus, decision.TenantID, job, owner)
	if err != nil {
		return false, err
	}
	if tag.RowsAffected() != 1 {
		return false, errArtifactLeaseLost
	}
	tag, err = tx.Exec(ctx, `UPDATE task_occurrences SET status=$3 WHERE tenant_id=$1 AND id=$2 AND status NOT IN ('canceled','completed') AND EXISTS (SELECT 1 FROM artifact_rubric_jobs j WHERE j.tenant_id=$4 AND j.id=$5 AND j.status='running' AND j.lease_owner=$6 AND j.lease_until>now())`, decision.TenantID, decision.OccurrenceID, occurrenceStatus, decision.TenantID, job, owner)
	if err != nil {
		return false, err
	}
	if tag.RowsAffected() != 1 {
		return false, errArtifactLeaseLost
	}
	if err = tx.Commit(ctx); err != nil {
		return false, err
	}
	return true, nil
}

// commitExternalDecision is the generic external decision boundary. It is
// deliberately independent of the artifact worker lease: callbacks are
// authenticated and bound by the external protocol worker before they reach
// this transaction. The decision unique key and versioned facts make replay
// and callback races exactly-once in effect.
func (s *Server) commitExternalDecision(ctx context.Context, decision verification.Decision) (bool, error) {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	var status, deliveryStatus string
	if err = tx.QueryRow(ctx, `SELECT status FROM verification_attempts WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, decision.TenantID, decision.AttemptID).Scan(&status); err != nil {
		return false, err
	}
	if status != "open" {
		return false, nil
	}
	if err = tx.QueryRow(ctx, `SELECT status FROM external_verifier_outbox WHERE tenant_id=$1 AND attempt_id=$2 FOR UPDATE`, decision.TenantID, decision.AttemptID).Scan(&deliveryStatus); err != nil {
		return false, err
	}
	if deliveryStatus == "canceled" || deliveryStatus == "dead" {
		return false, nil
	}
	decisionID := decision.ID
	if decisionID == "" {
		decisionID = uuid.NewString()
	}
	tag, err := tx.Exec(ctx, `INSERT INTO verification_decisions(id,tenant_id,attempt_id,accepted,reason,decided_by) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(tenant_id,attempt_id) DO NOTHING`, decisionID, decision.TenantID, decision.AttemptID, decision.Accepted, boundedSafeReason(decision.Reason), decision.DecidedBy)
	if err != nil {
		return false, err
	}
	if tag.RowsAffected() == 0 {
		return false, nil
	}
	attemptStatus, occurrenceStatus := "rejected", "pending"
	if decision.Accepted {
		attemptStatus, occurrenceStatus = "accepted", "completed"
	}
	if _, err = tx.Exec(ctx, `UPDATE verification_attempts SET status=$3 WHERE tenant_id=$1 AND id=$2`, decision.TenantID, decision.AttemptID, attemptStatus); err != nil {
		return false, err
	}
	if _, err = tx.Exec(ctx, `UPDATE task_occurrences SET status=$3 WHERE tenant_id=$1 AND id=$2 AND status NOT IN ('canceled','completed')`, decision.TenantID, decision.OccurrenceID, occurrenceStatus); err != nil {
		return false, err
	}
	decisionPayload, _ := json.Marshal(map[string]any{"accepted": decision.Accepted, "reason": boundedSafeReason(decision.Reason), "attemptId": decision.AttemptID})
	if _, err = tx.Exec(ctx, `INSERT INTO external_verifier_facts(id,tenant_id,aggregate_type,aggregate_id,fact_type,schema_version,payload) VALUES($1,$2,'verification_attempt',$3,'verification.decided',1,$4) ON CONFLICT DO NOTHING`, uuid.New(), decision.TenantID, decision.AttemptID, decisionPayload); err != nil {
		return false, err
	}
	if decision.Accepted {
		if _, err = tx.Exec(ctx, `INSERT INTO external_verifier_facts(id,tenant_id,aggregate_type,aggregate_id,fact_type,schema_version,payload) VALUES($1,$2,'task_occurrence',$3,'occurrence.completed',1,$4) ON CONFLICT DO NOTHING`, uuid.New(), decision.TenantID, decision.OccurrenceID, decisionPayload); err != nil {
			return false, err
		}
	}
	if _, err = tx.Exec(ctx, `UPDATE external_verifier_outbox SET status=$3,lease_owner=NULL,lease_until=NULL,updated_at=now() WHERE tenant_id=$1 AND attempt_id=$2 AND status NOT IN ('canceled','dead')`, decision.TenantID, decision.AttemptID, func() string {
		if decision.Accepted {
			return "accepted"
		}
		return "rejected"
	}()); err != nil {
		return false, err
	}
	if err = tx.Commit(ctx); err != nil {
		return false, err
	}
	return true, nil
}

func boundedSafeReason(reason string) string {
	reason = strings.TrimSpace(reason)
	if len(reason) > 500 {
		return reason[:500]
	}
	return reason
}

// scriptedArtifactModel is a non-sensitive qualification fixture. It checks
// both the server-supplied original digest and the exact authorized derivative
// bytes before producing structured tool calls.
type scriptedArtifactModel struct {
	expectedDigest, fault string
	expectedDerivative    []byte
	criteria              []verification.ArtifactCriterion
	accepted              bool
	calls                 int
}

func (m *scriptedArtifactModel) Provider() string { return "scripted" }
func (m *scriptedArtifactModel) Model() string    { return "scripted-fixture" }
func (m *scriptedArtifactModel) Generate(context.Context, fantasy.Call) (*fantasy.Response, error) {
	return nil, errors.New("scripted artifact model only streams")
}
func (m *scriptedArtifactModel) GenerateObject(context.Context, fantasy.ObjectCall) (*fantasy.ObjectResponse, error) {
	return nil, errors.New("scripted artifact objects disabled")
}
func (m *scriptedArtifactModel) StreamObject(context.Context, fantasy.ObjectCall) (fantasy.ObjectStreamResponse, error) {
	return nil, errors.New("scripted artifact objects disabled")
}
func (m *scriptedArtifactModel) Stream(ctx context.Context, call fantasy.Call) (fantasy.StreamResponse, error) {
	if m.fault == "always" || (m.fault == "once" && m.calls == 0) {
		m.calls++
		return nil, errors.New("scripted artifact provider failure")
	}
	var digest string
	var files [][]byte
	for _, message := range call.Prompt {
		for _, part := range message.Content {
			switch part.GetType() {
			case fantasy.ContentTypeText:
				if text, ok := fantasy.AsMessagePart[fantasy.TextPart](part); ok && strings.Contains(text.Text, "ARTIFACT_DIGEST=") {
					digest = strings.TrimSpace(strings.SplitN(text.Text, "ARTIFACT_DIGEST=", 2)[1])
				}
			case fantasy.ContentTypeFile:
				if file, ok := fantasy.AsMessagePart[fantasy.FilePart](part); ok {
					files = append(files, file.Data)
				}
			}
		}
	}
	if digest != m.expectedDigest || len(files) != 1 || !bytes.Equal(files[0], m.expectedDerivative) {
		return nil, errors.New("scripted artifact fixture digest or derivative mismatch")
	}
	index := m.calls
	m.calls++
	if index < len(m.criteria) {
		status := "rejected"
		if m.accepted {
			status = "accepted"
		}
		input, _ := json.Marshal(map[string]any{"criterionId": m.criteria[index].ID, "status": status, "evidence": "fixture observed the authorized derivative", "feedback": "fixture criterion result"})
		return scriptedToolStream(ctx, agent.ToolRecordArtifactCriterion, string(input)), nil
	}
	return scriptedStream(ctx, ""), nil
}
