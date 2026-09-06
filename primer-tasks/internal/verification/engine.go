package verification

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"primer-tasks/internal/domain"
)

type Manifest struct {
	Kind          string
	ConfigVersion int
	Interaction   string
	Executor      string
	MaxAttempts   int
	Timeout       time.Duration
}
type Registry struct {
	mu        sync.RWMutex
	manifests map[string]Manifest
}

func NewRegistry() *Registry {
	r := &Registry{manifests: map[string]Manifest{}}
	r.Register(Manifest{Kind: "parent_approval", ConfigVersion: 1, Interaction: "parent_action", Executor: "human", MaxAttempts: 3, Timeout: 24 * time.Hour})
	r.Register(Manifest{Kind: domain.AgentDialogueKind, ConfigVersion: domain.AgentDialogueConfigVersion, Interaction: "chat", Executor: "fantasy", MaxAttempts: 2, Timeout: 10 * time.Minute})
	return r
}
func (r *Registry) Register(m Manifest) { r.mu.Lock(); defer r.mu.Unlock(); r.manifests[m.Kind] = m }
func (r *Registry) Lookup(kind string) (Manifest, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	m, ok := r.manifests[kind]
	return m, ok
}
func (r *Registry) Validate(kind string, version int) error {
	m, ok := r.Lookup(kind)
	if !ok {
		return errors.New("unsupported verification kind")
	}
	if m.ConfigVersion != version {
		return errors.New("unsupported verification schema version")
	}
	return nil
}

// ValidateConfig is the shared manifest/config boundary, to be invoked by
// authoritative adapters. Registration alone never grants completion authority.
func (r *Registry) ValidateConfig(requirement domain.VerificationRequirement) error {
	if err := r.Validate(requirement.Kind, requirement.ConfigVersion); err != nil {
		return err
	}
	if requirement.Kind == domain.AgentDialogueKind {
		return domain.ValidateDialogueRequirement(requirement)
	}
	return nil
}

// RequirementOutcome is the current authoritative result for one requirement,
// loaded from the issued revision and durable decisions by the engine adapter.
// A satisfied dialogue requirement must not bypass another required driver.
type RequirementOutcome struct {
	RequirementID string
	Accepted      bool
}

func AllRequirementsAccepted(required []string, outcomes []RequirementOutcome) (bool, error) {
	if len(required) == 0 {
		return false, errors.New("issued requirements are required")
	}
	wanted := make(map[string]bool, len(required))
	for _, id := range required {
		if id == "" || wanted[id] {
			return false, errors.New("invalid issued requirement identity")
		}
		wanted[id] = true
	}
	seen := make(map[string]bool, len(outcomes))
	accepted := 0
	for _, result := range outcomes {
		if !wanted[result.RequirementID] || seen[result.RequirementID] {
			return false, errors.New("unbound or duplicate requirement outcome")
		}
		seen[result.RequirementID] = true
		if result.Accepted {
			accepted++
		}
	}
	return accepted == len(wanted), nil
}

// RequirementPolicySatisfied loads ALL requirements from the issued revision.
// Callers hold the occurrence lock in the same transaction as decision writes;
// the latest attempt per requirement is authoritative, never a cached counter.
// The engine owns this shared decision rule for human and dialogue drivers.
func RequirementPolicySatisfied(ctx context.Context, tx pgx.Tx, tenant, occurrence string) (bool, error) {
	if tx == nil || tenant == "" || occurrence == "" {
		return false, errors.New("bound verification transaction required")
	}
	rows, err := tx.Query(ctx, `SELECT r.id,COALESCE((
 SELECT COALESCE((SELECT ov.accepted FROM verification_overrides ov
  WHERE ov.tenant_id=a.tenant_id AND ov.attempt_id=a.id ORDER BY ov.result_version DESC LIMIT 1),d.accepted,false)
 FROM verification_attempts a LEFT JOIN verification_decisions d ON d.tenant_id=a.tenant_id AND d.attempt_id=a.id
 WHERE a.tenant_id=o.tenant_id AND a.occurrence_id=o.id AND a.requirement_id=r.id
 ORDER BY a.number DESC LIMIT 1),false)
 FROM task_occurrences o JOIN verification_requirements r ON r.tenant_id=o.tenant_id AND r.revision_id=o.revision_id
 WHERE o.tenant_id=$1 AND o.id=$2 ORDER BY r.ordinal`, tenant, occurrence)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	var required []string
	var outcomes []RequirementOutcome
	for rows.Next() {
		var outcome RequirementOutcome
		if err = rows.Scan(&outcome.RequirementID, &outcome.Accepted); err != nil {
			return false, err
		}
		required = append(required, outcome.RequirementID)
		outcomes = append(outcomes, outcome)
	}
	if err = rows.Err(); err != nil {
		return false, err
	}
	return AllRequirementsAccepted(required, outcomes)
}

// PublishDialogueOccurrenceCompletion records the dialogue-facing completion
// when a later manual requirement supplies the final accepted decision. The
// caller already holds the occurrence lock and has applied the all-requirement
// policy in this transaction; no repository/model owns a completion shortcut.
func PublishDialogueOccurrenceCompletion(ctx context.Context, tx pgx.Tx, tenant, occurrence string) error {
	var student, attempt string
	err := tx.QueryRow(ctx, `SELECT o.student_id,d.attempt_id FROM task_occurrences o JOIN dialogue_attempts d ON d.tenant_id=o.tenant_id AND d.occurrence_id=o.id JOIN verification_attempts a ON a.tenant_id=d.tenant_id AND a.id=d.attempt_id WHERE o.tenant_id=$1 AND o.id=$2 AND o.status='completed' AND a.status='accepted' AND a.number=(SELECT max(number) FROM verification_attempts WHERE tenant_id=a.tenant_id AND occurrence_id=a.occurrence_id AND requirement_id=a.requirement_id) ORDER BY a.number DESC,d.attempt_id LIMIT 1`, tenant, occurrence).Scan(&student, &attempt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	s, err := LoadDialogueState(ctx, tx, StudentAuthority{TenantID: tenant, StudentID: student}, occurrence, attempt)
	if err != nil {
		return err
	}
	ready, err := DialogueEvidence(s)
	if err != nil {
		return err
	}
	var decision, source string
	err = tx.QueryRow(ctx, `SELECT id::text,'parent_override' FROM verification_overrides WHERE tenant_id=$1 AND attempt_id=$2 AND accepted ORDER BY result_version DESC LIMIT 1`, tenant, attempt).Scan(&decision, &source)
	if errors.Is(err, pgx.ErrNoRows) {
		err = tx.QueryRow(ctx, `SELECT id::text,'verification_engine' FROM verification_decisions WHERE tenant_id=$1 AND attempt_id=$2 AND accepted`, tenant, attempt).Scan(&decision, &source)
	}
	if err != nil {
		return err
	}
	return appendDialogueEvent(ctx, tx, s, "completion", DialogueEvent{Kind: "complete", Status: "accepted", OccurrenceStatus: "completed", DecisionID: decision, DecisionSource: source, AcceptedCount: ready.AcceptedCount, RequiredCount: 3})
}

type Decision struct {
	ID        string
	AttemptID string
	Accepted  bool
	Reason    string
	ParentID  string
	CreatedAt time.Time
}
type Policy struct{ All bool }

func Apply(policy Policy, decisions []Decision) (bool, error) {
	if len(decisions) == 0 {
		return false, nil
	}
	if policy.All {
		for _, d := range decisions {
			if !d.Accepted {
				return false, nil
			}
		}
		return true, nil
	}
	for _, d := range decisions {
		if d.Accepted {
			return true, nil
		}
	}
	return false, nil
}
