package verification

import (
	"errors"
	"sync"
	"time"
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
