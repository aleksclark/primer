package verification

import (
	"errors"
	"sync"
	"time"

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
	r.Register(Manifest{Kind: "agent_artifact_rubric", ConfigVersion: 1, Interaction: "artifact_upload", Executor: "fantasy", MaxAttempts: 2, Timeout: 15 * time.Minute})
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

// ValidateConfig is the publish-time manifest boundary. Schema version and
// typed policy validation are both required; an unknown kind fails closed.
func (r *Registry) ValidateConfig(kind string, version int, config map[string]any) error {
	if err := r.Validate(kind, version); err != nil {
		return err
	}
	if kind == domain.AgentDialogueKind {
		return domain.ValidateDialogueRequirement(domain.VerificationRequirement{Kind: kind, ConfigVersion: version, Config: config, Interaction: "chat", Executor: "fantasy"})
	}
	if kind == "agent_artifact_rubric" {
		return validateArtifactRubricConfig(config)
	}
	return nil
}

type Decision struct {
	ID           string
	TenantID     string
	AttemptID    string
	OccurrenceID string
	Accepted     bool
	Reason       string
	DecidedBy    string
	ParentID     string
	CreatedAt    time.Time
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
