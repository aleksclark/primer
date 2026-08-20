// Package workflow runs persisted Curriculum Studio materialization stages.
package workflow

import (
	"context"
	"fmt"
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
	Kind      string
	Title     string
	Body      map[string]any
	UnitID    *string
	OutcomeID *string
}

// LanguageModel is the only generation seam used by the runner.
type LanguageModel interface {
	Complete(context.Context, Request) (Response, error)
}

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
	return scriptedResponse(req.Stage, fixture), nil
}

func scriptedResponse(stage, fixture string) Response {
	r := Response{Fixture: fixture}
	switch stage {
	case "lessons":
		r.Items = []Item{{Kind: "lesson", Title: "Scripted lesson", Body: map[string]any{"content": "A scripted lesson."}}}
	case "assessments":
		r.Items = []Item{
			{Kind: "assessment", Title: "Scripted assessment", Body: map[string]any{"questions": []any{"What did you learn?"}}},
			{Kind: "rubric", Title: "Scripted assessment rubric", Body: map[string]any{"criteria": []any{"accuracy"}}},
			{Kind: "answer_key", Title: "Scripted assessment answers", Body: map[string]any{"answers": []any{"A complete response."}}},
		}
	case "critic":
		r.Findings = []map[string]any{}
	}
	return r
}

// NewModel fails closed: live providers are intentionally not part of S12.
func NewModel(provider string) (LanguageModel, error) {
	if provider == "" || provider == "scripted" {
		return &Scripted{}, nil
	}
	return nil, fmt.Errorf("model provider %q is blocked; only scripted is supported", provider)
}
