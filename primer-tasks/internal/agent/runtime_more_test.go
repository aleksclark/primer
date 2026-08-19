package agent

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"primer-tasks/internal/agent/protocol"
)

func TestRunLimitsTransitionsAndPreviewValidation(t *testing.T) {
	base := Run{MaxSteps: 1, MaxTokens: 1, Deadline: time.Now().Add(time.Minute)}
	cases := []struct {
		name string
		run  Run
	}{
		{"no steps", Run{MaxSteps: 0, MaxTokens: 1, Deadline: base.Deadline}},
		{"too many steps", Run{MaxSteps: 101, MaxTokens: 1, Deadline: base.Deadline}},
		{"no tokens", Run{MaxSteps: 1, MaxTokens: 0, Deadline: base.Deadline}},
		{"too many tokens", Run{MaxSteps: 1, MaxTokens: 200001, Deadline: base.Deadline}},
		{"no deadline", Run{MaxSteps: 1, MaxTokens: 1}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := ValidateRunLimits(tc.run); err == nil {
				t.Fatal("invalid run limits accepted")
			}
		})
	}
	for _, tc := range []struct {
		from, to RunStatus
		want     bool
	}{
		{RunQueued, RunRunning, true}, {RunQueued, RunCanceled, true},
		{RunRunning, RunSucceeded, true}, {RunRunning, RunCancelRequested, true},
		{RunCancelRequested, RunCanceled, true}, {RunCancelRequested, RunRunning, false},
		{RunSucceeded, RunFailed, false},
	} {
		if got := CanTransition(tc.from, tc.to); got != tc.want {
			t.Errorf("CanTransition(%q,%q)=%v, want %v", tc.from, tc.to, got, tc.want)
		}
	}
	p := ConfirmationPreview{TenantID: "tenant-a", ActorID: "parent-a", Action: "retire", ActionDigest: ActionDigest("retire", []byte("task-1")), ExpiresAt: time.Now().Add(time.Minute)}
	if err := ValidatePreview(p, "tenant-a", "parent-a", "retire", []byte("task-1"), time.Now()); err != nil {
		t.Fatal(err)
	}
	for _, check := range []struct {
		name   string
		tenant string
		actor  string
		action string
		body   []byte
	}{
		{"foreign tenant", "tenant-b", "parent-a", "retire", []byte("task-1")},
		{"foreign actor", "tenant-a", "parent-b", "retire", []byte("task-1")},
		{"altered action", "tenant-a", "parent-a", "publish", []byte("task-1")},
		{"altered payload", "tenant-a", "parent-a", "retire", []byte("task-2")},
	} {
		t.Run(check.name, func(t *testing.T) {
			if err := ValidatePreview(p, check.tenant, check.actor, check.action, check.body, time.Now()); !errors.Is(err, ErrPreviewExpired) {
				t.Fatalf("error=%v", err)
			}
		})
	}
	used := p
	now := time.Now()
	used.UsedAt = &now
	if err := ValidatePreview(used, "tenant-a", "parent-a", "retire", []byte("task-1"), now); !errors.Is(err, ErrPreviewExpired) {
		t.Fatalf("used preview error=%v", err)
	}
}

func TestRuntimeRejectsEmitterFailureAndInvalidLimits(t *testing.T) {
	if _, err := NewFantasyAgent(&scriptedModel{}, nil, Limits{MaxSteps: 0, MaxTokens: 1, Deadline: time.Second}); err == nil {
		t.Fatal("invalid limits accepted")
	}
	r, err := NewFantasyAgent(&scriptedModel{}, nil, Limits{MaxSteps: 2, MaxTokens: 10, Deadline: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	want := errors.New("subscriber gone")
	_, err = r.Execute(context.Background(), "run", "prompt", func(protocol.Event) error { return want })
	if !errors.Is(err, want) {
		t.Fatalf("emitter error=%v, want %v", err, want)
	}
	var seen []protocol.Event
	_, err = r.Execute(context.Background(), "run", "prompt", func(e protocol.Event) error {
		seen = append(seen, e)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for i, event := range seen {
		if event.Sequence != int64(i+1) || event.Cursor != event.Sequence {
			t.Fatalf("event %d has non-contiguous cursor: %+v", i, event)
		}
		if strings.Contains(event.Text, "secret") || strings.Contains(event.Label, "secret") {
			t.Fatalf("unsafe event: %+v", event)
		}
	}
}
