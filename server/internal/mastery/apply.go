// Package mastery applies server-side mastery updates from accepted student
// completion evidence. Clients never set status or confidence directly.
package mastery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/aleksclark/primer/server/internal/domain"
	"github.com/aleksclark/primer/server/internal/repo"
	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
)

// ConfidenceBump is added per successful primary-standard completion.
const ConfidenceBump = 0.35

// MasteredThreshold is the confidence at which status becomes mastered.
const MasteredThreshold = 0.8

// ApplyCompletion validates and records a session completion, writing mastery
// evidence and updating aggregates in one transaction.
func ApplyCompletion(ctx context.Context, q repo.Querier, device *domain.StudentDevice, sessionID string, req contracts.CompletionRequest, now time.Time) (contracts.CompletionResult, error) {
	var result contracts.CompletionResult

	err := repo.WithTx(ctx, q, func(tx repo.Querier) error {
		session, err := repo.GetSession(ctx, tx, sessionID)
		if err != nil {
			return err
		}
		if session.StudentID != device.StudentID {
			return repo.ErrNotFound
		}

		// Idempotent retry by completion UUID.
		if existing, err := repo.GetCompletionByID(ctx, tx, req.CompletionID); err == nil {
			if existing.RequestDigest != req.RequestDigest {
				return fmt.Errorf("%w: completion id reused with different digest", repo.ErrConflict)
			}
			decoded, err := repo.CompletionResultFromRow(existing)
			if err != nil {
				return err
			}
			result = decoded
			return nil
		} else if !errors.Is(err, repo.ErrNotFound) {
			return err
		}

		// Session already completed with a different completion id.
		if existing, err := repo.GetCompletionBySession(ctx, tx, sessionID); err == nil {
			if existing.CompletionID == req.CompletionID && existing.RequestDigest == req.RequestDigest {
				decoded, err := repo.CompletionResultFromRow(existing)
				if err != nil {
					return err
				}
				result = decoded
				return nil
			}
			return fmt.Errorf("%w: session already completed", repo.ErrConflict)
		} else if !errors.Is(err, repo.ErrNotFound) {
			return err
		}

		if session.State == domain.SessionCompleted || session.State == domain.SessionAbandoned {
			return repo.ErrBadRequest{Msg: "session is not active"}
		}

		asg, err := repo.StudentAssignments.Get(ctx, tx, session.AssignmentID)
		if err != nil {
			return err
		}
		if asg.StudentID != device.StudentID {
			return repo.ErrNotFound
		}
		if asg.State == domain.AssignmentCancelled {
			return repo.ErrBadRequest{Msg: "assignment is cancelled"}
		}

		rev, err := repo.GetRevision(ctx, tx, session.ActivityRevisionID)
		if err != nil {
			return err
		}
		content, err := decodeContent(rev.Content)
		if err != nil {
			return err
		}
		if err := validateRequiredChecks(content, req.Observations); err != nil {
			return err
		}

		links, err := repo.ListRevisionStandards(ctx, tx, rev.ID)
		if err != nil {
			return err
		}

		primaryCheck := primaryCheckID(content, req.Observations)
		var evidenceIDs []string
		var transitions []contracts.MasteryTransition

		for _, link := range links {
			std, err := repo.Standards.Get(ctx, tx, link.StandardID)
			if err != nil {
				return err
			}
			sourceRef := fmt.Sprintf("student-client:%s:%s:%s", session.ID, primaryCheck, std.ID)
			rec, created, err := upsertMasteryRecord(ctx, tx, session.StudentID, link.StandardID)
			if err != nil {
				return err
			}
			_ = created

			fromStatus := rec.Status
			fromConf := rec.Confidence

			evID, inserted, err := insertEvidence(ctx, tx, rec.ID, sourceRef, now, session.ID, primaryCheck)
			if err != nil {
				return err
			}
			if inserted {
				evidenceIDs = append(evidenceIDs, evID)
				bump := ConfidenceBump * link.Weight
				if bump <= 0 {
					bump = ConfidenceBump
				}
				newConf := clamp01(fromConf + bump)
				newStatus := statusFor(fromStatus, newConf, link.Role)
				if err := updateMasteryAggregate(ctx, tx, rec.ID, newStatus, newConf, now); err != nil {
					return err
				}
				transitions = append(transitions, contracts.MasteryTransition{
					StandardCode: std.Code,
					FromStatus:   fromStatus,
					ToStatus:     newStatus,
					Confidence:   newConf,
					Reason:       "student-client completion",
				})
			} else {
				// Evidence already present (retry mid-flight); report current state.
				transitions = append(transitions, contracts.MasteryTransition{
					StandardCode: std.Code,
					FromStatus:   fromStatus,
					ToStatus:     rec.Status,
					Confidence:   rec.Confidence,
					Reason:       "already recorded",
				})
				evidenceIDs = append(evidenceIDs, evID)
			}
		}

		result = contracts.CompletionResult{
			SchemaVersion:   contracts.CompletionSchemaVersion,
			CompletionID:    req.CompletionID,
			Accepted:        true,
			RequestDigest:   req.RequestDigest,
			Observations:    req.Observations,
			EvidenceIDs:     evidenceIDs,
			MasterySnapshot: transitions,
			Message:         "accepted",
		}
		if _, err := repo.InsertCompletion(ctx, tx, session.ID, req.CompletionID, req.RequestDigest, result); err != nil {
			return err
		}
		summary := req.Summary
		if summary == "" {
			summary = "completed"
		}
		return repo.MarkSessionCompleted(ctx, tx, session, summary, now)
	})
	return result, err
}

func decodeContent(m map[string]any) (contracts.ActivityContent, error) {
	raw, err := json.Marshal(m)
	if err != nil {
		return contracts.ActivityContent{}, err
	}
	var c contracts.ActivityContent
	if err := json.Unmarshal(raw, &c); err != nil {
		return contracts.ActivityContent{}, fmt.Errorf("decode activity content: %w", err)
	}
	return c, nil
}

func validateRequiredChecks(content contracts.ActivityContent, obs []contracts.Observation) error {
	byID := map[string]contracts.Observation{}
	for _, o := range obs {
		byID[o.CheckID] = o
	}
	// All non-optional checks defined on the revision must pass.
	for _, check := range content.Checks {
		if check.Optional {
			continue
		}
		o, ok := byID[check.ID]
		if !ok || !o.Passed {
			return repo.ErrBadRequest{Msg: "required check not passed: " + check.ID}
		}
	}
	// Also require each task completion tree (required nodes).
	for _, task := range content.Tasks {
		if task.Optional {
			continue
		}
		if !evalTree(task.Completion, byID) {
			return repo.ErrBadRequest{Msg: "task completion requirements not met: " + task.ID}
		}
	}
	return nil
}

func evalTree(t contracts.CheckTree, byID map[string]contracts.Observation) bool {
	if t.Optional {
		return true
	}
	if t.CheckID != "" {
		o, ok := byID[t.CheckID]
		return ok && o.Passed
	}
	if len(t.All) > 0 {
		for _, c := range t.All {
			if !evalTree(c, byID) {
				return false
			}
		}
		return true
	}
	if len(t.Any) > 0 {
		for _, c := range t.Any {
			if evalTree(c, byID) {
				return true
			}
		}
		return false
	}
	return true
}

func primaryCheckID(content contracts.ActivityContent, obs []contracts.Observation) string {
	for _, c := range content.Checks {
		if !c.Optional {
			return c.ID
		}
	}
	if len(obs) > 0 {
		return obs[0].CheckID
	}
	return "unknown"
}

func upsertMasteryRecord(ctx context.Context, q repo.Querier, studentID, standardID string) (*domain.MasteryRecord, bool, error) {
	const sel = `
SELECT id, student_id, standard_id, status, confidence, decay_rate,
       last_assessed_at, last_reinforced_at, next_reinforcement_at, created_at, updated_at
FROM mastery_records WHERE student_id = $1 AND standard_id = $2 FOR UPDATE`
	rows, err := q.Query(ctx, sel, studentID, standardID)
	if err != nil {
		return nil, false, fmt.Errorf("select mastery: %w", err)
	}
	rec, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByNameLax[domain.MasteryRecord])
	if err == nil {
		return &rec, false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, false, fmt.Errorf("scan mastery: %w", err)
	}
	created, err := repo.MasteryRecords.Create(ctx, q, map[string]any{
		"student_id":  studentID,
		"standard_id": standardID,
		"status":      "not_introduced",
		"confidence":  0.0,
		"decay_rate":  0.02,
	})
	if err != nil {
		// concurrent insert
		rows2, err2 := q.Query(ctx, sel, studentID, standardID)
		if err2 != nil {
			return nil, false, err
		}
		rec2, err2 := pgx.CollectExactlyOneRow(rows2, pgx.RowToStructByNameLax[domain.MasteryRecord])
		if err2 != nil {
			return nil, false, err
		}
		return &rec2, false, nil
	}
	return created, true, nil
}

func insertEvidence(ctx context.Context, q repo.Querier, masteryRecordID, sourceRef string, now time.Time, sessionID, checkID string) (id string, inserted bool, err error) {
	ctxStr := fmt.Sprintf("session=%s check=%s", sessionID, checkID)
	const ins = `
INSERT INTO mastery_evidence (mastery_record_id, kind, occurred_on, context, source_ref)
VALUES ($1, 'continuous', $2::date, $3, $4)
ON CONFLICT (mastery_record_id, source_ref) WHERE source_ref <> '' DO NOTHING
RETURNING id`
	var newID string
	err = q.QueryRow(ctx, ins, masteryRecordID, now, ctxStr, sourceRef).Scan(&newID)
	if err == nil {
		return newID, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", false, fmt.Errorf("insert evidence: %w", err)
	}
	const sel = `SELECT id FROM mastery_evidence WHERE mastery_record_id = $1 AND source_ref = $2`
	if err := q.QueryRow(ctx, sel, masteryRecordID, sourceRef).Scan(&newID); err != nil {
		return "", false, fmt.Errorf("select existing evidence: %w", err)
	}
	return newID, false, nil
}

func updateMasteryAggregate(ctx context.Context, q repo.Querier, id, status string, confidence float64, now time.Time) error {
	_, err := q.Exec(ctx, `
UPDATE mastery_records
SET status = $2, confidence = $3, last_assessed_at = $4, last_reinforced_at = $4, updated_at = now()
WHERE id = $1`, id, status, confidence, now)
	if err != nil {
		return fmt.Errorf("update mastery: %w", err)
	}
	return nil
}

func statusFor(from string, conf float64, role string) string {
	if conf >= MasteredThreshold {
		return "mastered"
	}
	if conf >= 0.45 {
		return "approaching"
	}
	if from == "not_introduced" || from == "" {
		return "in_progress"
	}
	if role == contracts.StandardRoleReinforcement && from == "mastered" {
		return "mastered"
	}
	if from == "mastered" && conf < MasteredThreshold {
		return "approaching"
	}
	return "in_progress"
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
