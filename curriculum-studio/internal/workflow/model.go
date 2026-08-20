// Package workflow runs persisted Curriculum Studio materialization stages.
package workflow

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"path"
	"strings"
	"sync"
)

// Stable stage keys. Objectives are already captured in a materialization's
// immutable input snapshot, so generation starts with lessons.
var StageKeys = []string{"lessons", "assessments", "critic"}

// Request is the provider-independent prompt supplied to a workflow stage.
type Request struct {
	Stage   string
	Input   map[string]any
	Fixture string
}

// Response is deliberately structured: scripted fixtures are data, not an LLM
// protocol, which keeps this release credential-free and deterministic.
type Response struct {
	Fixture  string
	Items    []Item
	Findings []map[string]any
}

// Item is a generated materialization candidate.
type Item struct {
	Kind      string         `json:"kind"`
	Title     string         `json:"title"`
	Body      map[string]any `json:"body"`
	UnitID    *string        `json:"unit_id,omitempty"`
	OutcomeID *string        `json:"outcome_id,omitempty"`
}

// LanguageModel is the only generation seam used by the runner.
type LanguageModel interface {
	Complete(context.Context, Request) (Response, error)
}

//go:embed testdata/fixtures
var fixtureFiles embed.FS

// Scripted is a no-network deterministic LanguageModel. Tests may block a
// call or arrange transient/permanent failures without replacing the seam.
type Scripted struct {
	Fixture string
	Pause   <-chan struct{}
	// Started is notified immediately before a configured Pause blocks.
	Started    chan<- struct{}
	FailOnce   bool
	FailAlways bool
	mu         sync.Mutex
	failed     bool
}

func (s *Scripted) Complete(ctx context.Context, req Request) (Response, error) {
	if s.Pause != nil {
		if s.Started != nil {
			select {
			case s.Started <- struct{}{}:
			default:
			}
		}
		select {
		case <-s.Pause:
		case <-ctx.Done():
			return Response{}, ctx.Err()
		}
	}
	s.mu.Lock()
	fail := s.FailAlways || (s.FailOnce && !s.failed)
	if fail {
		s.failed = true
	}
	s.mu.Unlock()
	if fail {
		return Response{}, fmt.Errorf("scripted provider failure")
	}
	fixture := s.Fixture
	if fixture == "" {
		fixture = "default-v1"
	}
	return loadFixture(fixture, req.Stage)
}

// loadFixture reads generation data embedded into the binary. Fixture names are
// directory identifiers, never host paths, so the scripted provider has neither
// filesystem nor network dependencies at runtime.
func loadFixture(fixture, stage string) (Response, error) {
	fixture = strings.TrimSpace(fixture)
	stage = strings.TrimSpace(stage)
	if fixture == "" || strings.Contains(fixture, "/") || strings.Contains(fixture, "\\") || fixture == "." || fixture == ".." {
		return Response{}, fmt.Errorf("invalid scripted fixture %q", fixture)
	}
	if stage == "" || strings.Contains(stage, "/") || strings.Contains(stage, "\\") {
		return Response{}, fmt.Errorf("invalid scripted stage %q", stage)
	}
	raw, err := fixtureFiles.ReadFile(path.Join("testdata/fixtures", fixture, stage+".json"))
	if err != nil {
		return Response{}, fmt.Errorf("load scripted fixture %s/%s: %w", fixture, stage, err)
	}
	var response Response
	if err := json.Unmarshal(raw, &response); err != nil {
		return Response{}, fmt.Errorf("decode scripted fixture %s/%s: %w", fixture, stage, err)
	}
	response.Fixture = fixture // provenance is the stable directory identifier.
	return response, nil
}

// NewModel fails closed: live providers are intentionally not part of S12.
func NewModel(provider string) (LanguageModel, error) {
	if provider == "" || provider == "scripted" {
		return &Scripted{}, nil
	}
	return nil, fmt.Errorf("model provider %q is blocked; only scripted is supported", provider)
}
