// Package mastery applies server-side mastery updates from accepted student
// completion evidence. Clients never set status or confidence directly.
package mastery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/aleksclark/primer/server/internal/domain"
	"github.com/aleksclark/primer/server/internal/repo"
	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
)

// ConfidenceBump is added per successful primary-standard completion when policy permits.
const ConfidenceBump = 0.35

// MasteredThreshold is the confidence at which status becomes mastered.
const MasteredThreshold = 0.8

// ApplyCompletion validates and records a session completion, writing mastery
// evidence and updating aggregates only when the revision-standard evidence
// policy permits. Assignment completion is independent of mastery transitions.
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
		// Cancelled assignments never create mastery evidence.
		if asg.State == domain.AssignmentCancelled {
			return repo.ErrBadRequest{Msg: "assignment is cancelled"}
		}

		rev, err := repo.GetRevision(ctx, tx, session.ActivityRevisionID)
		if err != nil {
			return err
		}
		act, err := repo.LearningActivities.Get(ctx, tx, rev.ActivityID)
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
		if err := validateObservationCapabilities(content, req.Observations); err != nil {
			return err
		}

		links, err := repo.ListRevisionStandards(ctx, tx, rev.ID)
		if err != nil {
			return err
		}

		evidenceClass := contracts.EvidenceProceduralContinuous
		primaryCheck := primaryCheckID(content, req.Observations)
		var evidenceIDs []string
		var transitions []contracts.MasteryTransition

		for _, link := range links {
			std, err := repo.Standards.Get(ctx, tx, link.StandardID)
			if err != nil {
				return err
			}
			if !standardEligibleForActivity(act.Kind, std.Code) {
				continue
			}

			policy := policyFromLink(link, act.Kind)
			sourceRef := fmt.Sprintf("student-client:%s:%s:%s", session.ID, primaryCheck, std.ID)
			rec, _, err := upsertMasteryRecord(ctx, tx, session.StudentID, link.StandardID)
			if err != nil {
				return err
			}

			fromStatus := rec.Status
			fromConf := rec.Confidence

			prov := map[string]any{
				"sessionId":     session.ID,
				"assignmentId":  session.AssignmentID,
				"revisionId":    rev.ID,
				"checkId":       primaryCheck,
				"deviceId":      device.ID,
				"activityKind":  act.Kind,
				"evidenceClass": evidenceClass,
				"standardCode":  std.Code,
				"role":          link.Role,
			}
			evID, inserted, err := insertEvidence(ctx, tx, rec.ID, sourceRef, evidenceClass, prov, policy.Version, now, session.ID, primaryCheck)
			if err != nil {
				return err
			}
			evidenceIDs = append(evidenceIDs, evID)

			accepted, err := listAcceptedEvidenceClasses(ctx, tx, rec.ID)
			if err != nil {
				return err
			}
			if inserted {
				// Newly inserted class is already in DB; ensure set includes it.
				accepted = addClass(accepted, evidenceClass)
			}

			candidateStatus, candidateConf := proposedStatus(fromStatus, fromConf, link.Role, link.Weight)
			allowedStatus, missing, satisfied := highestAllowedStatus(fromStatus, candidateStatus, accepted, policy)

			tr := contracts.MasteryTransition{
				StandardCode:     std.Code,
				FromStatus:       fromStatus,
				ToStatus:         fromStatus,
				Confidence:       fromConf,
				EvidenceClass:    evidenceClass,
				AcceptedEvidence: accepted,
				MissingEvidence:  missing,
				PolicySatisfied:  satisfied && allowedStatus == candidateStatus,
				StatusChanged:    false,
			}

			if !inserted {
				tr.Reason = "already recorded"
				transitions = append(transitions, tr)
				continue
			}

			newStatus := fromStatus
			newConf := fromConf
			if statusRank(allowedStatus) > statusRank(fromStatus) {
				// Partial or full advance permitted by evidence policy.
				if statusRank(allowedStatus) >= statusRank(candidateStatus) {
					newStatus = candidateStatus
					newConf = candidateConf
				} else {
					newStatus = allowedStatus
					newConf = confForStatus(allowedStatus, fromConf, link.Weight)
				}
			} else if statusRank(allowedStatus) >= statusRank(candidateStatus) && candidateConf > fromConf {
				// Same status band fully permitted: confidence may rise.
				newStatus = candidateStatus
				newConf = candidateConf
			}

			if newStatus != fromStatus || newConf != fromConf {
				if err := updateMasteryAggregate(ctx, tx, rec.ID, newStatus, newConf, now); err != nil {
					return err
				}
				tr.ToStatus = newStatus
				tr.Confidence = newConf
				tr.StatusChanged = true
				tr.Reason = "student-client completion"
				tr.PolicySatisfied = len(missingForStatus(candidateStatus, accepted, policy)) == 0
				tr.MissingEvidence = missingForStatus(nextUnsatisfiedStatus(newStatus, accepted, policy), accepted, policy)
			} else {
				tr.Reason = "evidence recorded; policy limits mastery transition"
				if len(missing) == 0 {
					missing = missingForStatus(candidateStatus, accepted, policy)
				}
				tr.MissingEvidence = missing
				tr.PolicySatisfied = len(missingForStatus(candidateStatus, accepted, policy)) == 0
			}
			transitions = append(transitions, tr)
		}

		summary := req.Summary
		if summary == "" {
			summary = "completed"
		}
		result = contracts.CompletionResult{
			SchemaVersion:  contracts.CompletionSchemaVersion,
			CompletionID:   req.CompletionID,
			Accepted:       true,
			RequestDigest:  req.RequestDigest,
			Observations:   req.Observations,
			EvidenceIDs:    evidenceIDs,
			MasterySnapshot: transitions,
			MasteryTransitions: transitions,
			AssignmentCompletion: &contracts.AssignmentCompletion{
				AssignmentID: session.AssignmentID,
				SessionID:    session.ID,
				State:        domain.AssignmentCompleted,
				Summary:      summary,
			},
			Message: "accepted",
		}
		if _, err := repo.InsertCompletion(ctx, tx, session.ID, req.CompletionID, req.RequestDigest, result); err != nil {
			return err
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
	for _, check := range content.Checks {
		if check.Optional {
			continue
		}
		o, ok := byID[check.ID]
		if !ok || !o.Passed {
			return repo.ErrBadRequest{Msg: "required check not passed: " + check.ID}
		}
	}
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

// validateObservationCapabilities rejects completions that claim structured
// command evidence from an untrusted observation source (e.g. synthetic PTY).
func validateObservationCapabilities(content contracts.ActivityContent, obs []contracts.Observation) error {
	byID := map[string]contracts.Check{}
	for _, c := range content.Checks {
		byID[c.ID] = c
	}
	for _, o := range obs {
		if !o.Passed || o.Optional {
			continue
		}
		check, ok := byID[o.CheckID]
		if !ok {
			continue
		}
		if !contracts.RequiresStructuredCommandEvidence(check.Kind) {
			continue
		}
		if observationHasStructuredCommandEvidence(o) {
			continue
		}
		return repo.ErrBadRequest{Msg: "required check needs structured command evidence: " + check.ID}
	}
	return nil
}

func observationHasStructuredCommandEvidence(o contracts.Observation) bool {
	if o.Details == nil {
		return false
	}
	if v, ok := o.Details["structuredCommandEvidence"].(bool); ok && v {
		return true
	}
	if v, ok := o.Details["capability"].(string); ok && v == contracts.CapStructuredCommandEvidence {
		return true
	}
	if v, ok := o.Details["source"].(string); ok {
		switch strings.ToLower(v) {
		case "structured", "structured_command", "command_instrumentation":
			return true
		case "pty-shell", "synthetic-pty", "screen":
			return false
		}
	}
	// Default: command-sensitive checks without an explicit capability mark are untrusted.
	return false
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

// standardEligibleForActivity keeps typing evidence isolated from terminal standards.
func standardEligibleForActivity(kind, code string) bool {
	isTyping := strings.Contains(code, ".TYPE.")
	switch kind {
	case contracts.KindTyping:
		return isTyping
	case contracts.KindTerminal:
		return !isTyping
	default:
		return true
	}
}

func policyFromLink(link domain.LearningActivityRevisionStandard, kind string) contracts.EvidencePolicy {
	if p, ok := repo.ParseEvidencePolicy(link.EvidencePolicy); ok {
		return p
	}
	if kind == contracts.KindTyping {
		return contracts.DefaultTypingEvidencePolicy()
	}
	return contracts.DefaultTerminalEvidencePolicy()
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

func insertEvidence(ctx context.Context, q repo.Querier, masteryRecordID, sourceRef, evidenceClass string, provenance map[string]any, policyVersion int, now time.Time, sessionID, checkID string) (id string, inserted bool, err error) {
	ctxStr := fmt.Sprintf("session=%s check=%s class=%s", sessionID, checkID, evidenceClass)
	if provenance == nil {
		provenance = map[string]any{}
	}
	const ins = `
INSERT INTO mastery_evidence (
    mastery_record_id, kind, evidence_class, provenance, policy_version,
    occurred_on, context, source_ref
)
VALUES ($1, 'continuous', $2, $3, $4, $5::date, $6, $7)
ON CONFLICT (mastery_record_id, source_ref) WHERE source_ref <> '' DO NOTHING
RETURNING id`
	var newID string
	err = q.QueryRow(ctx, ins, masteryRecordID, evidenceClass, provenance, policyVersion, now, ctxStr, sourceRef).Scan(&newID)
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

func listAcceptedEvidenceClasses(ctx context.Context, q repo.Querier, masteryRecordID string) ([]string, error) {
	const sqlStr = `
SELECT DISTINCT evidence_class
FROM mastery_evidence
WHERE mastery_record_id = $1
  AND context NOT ILIKE '%superseded by parent%'
ORDER BY evidence_class`
	rows, err := q.Query(ctx, sqlStr, masteryRecordID)
	if err != nil {
		return nil, fmt.Errorf("list evidence classes: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
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

func proposedStatus(from string, fromConf float64, role string, weight float64) (string, float64) {
	bump := ConfidenceBump * weight
	if bump <= 0 {
		bump = ConfidenceBump
	}
	newConf := clamp01(fromConf + bump)
	return statusFor(from, newConf, role), newConf
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

func confForStatus(status string, fromConf, weight float64) float64 {
	bump := ConfidenceBump * weight
	if bump <= 0 {
		bump = ConfidenceBump
	}
	switch status {
	case "in_progress":
		if fromConf > 0 {
			return clamp01(fromConf + bump*0.25)
		}
		return clamp01(0.2)
	case "approaching":
		return clamp01(maxFloat(fromConf, 0.45))
	case "mastered":
		return clamp01(maxFloat(fromConf, MasteredThreshold))
	default:
		return fromConf
	}
}

func highestAllowedStatus(from, candidate string, accepted []string, policy contracts.EvidencePolicy) (allowed string, missing []string, fullySatisfied bool) {
	order := []string{"in_progress", "approaching", "mastered"}
	fromRank := statusRank(from)
	candRank := statusRank(candidate)
	allowed = from
	if from == "" {
		allowed = "not_introduced"
	}
	fullySatisfied = true
	for _, st := range order {
		r := statusRank(st)
		if r <= fromRank {
			continue
		}
		if r > candRank {
			break
		}
		miss := missingForStatus(st, accepted, policy)
		if len(miss) > 0 {
			fullySatisfied = false
			missing = miss
			break
		}
		allowed = st
	}
	if statusRank(allowed) < candRank {
		fullySatisfied = false
		if len(missing) == 0 {
			missing = missingForStatus(candidate, accepted, policy)
		}
	}
	return allowed, missing, fullySatisfied && allowed == candidate
}

func missingForStatus(status string, accepted []string, policy contracts.EvidencePolicy) []string {
	if status == "" || status == "not_introduced" {
		return nil
	}
	req, ok := policy.StatusRequirements[status]
	if !ok {
		return nil
	}
	have := map[string]bool{}
	for _, a := range accepted {
		have[a] = true
	}
	var missing []string
	for _, need := range req {
		if !have[need] {
			missing = append(missing, need)
		}
	}
	return missing
}

func nextUnsatisfiedStatus(current string, accepted []string, policy contracts.EvidencePolicy) string {
	for _, st := range []string{"in_progress", "approaching", "mastered"} {
		if statusRank(st) <= statusRank(current) {
			continue
		}
		if len(missingForStatus(st, accepted, policy)) > 0 {
			return st
		}
	}
	return ""
}

func statusRank(s string) int {
	switch s {
	case "not_introduced", "":
		return 0
	case "in_progress":
		return 1
	case "approaching":
		return 2
	case "mastered":
		return 3
	default:
		return 0
	}
}

func addClass(classes []string, c string) []string {
	for _, x := range classes {
		if x == c {
			return classes
		}
	}
	return append(append([]string{}, classes...), c)
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

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
