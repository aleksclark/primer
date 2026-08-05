// Package tutor provides server-owned, policy-constrained coaching for student
// learning sessions. Providers never receive client-supplied system prompts and
// have no tools that mutate mastery, assignments, or workstation state.
package tutor

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
)

// ErrDisabled indicates tutoring is turned off for the student or deployment.
var ErrDisabled = errors.New("tutor disabled")

// ErrRateLimited indicates the session exceeded the per-session message budget.
var ErrRateLimited = errors.New("tutor rate limited")

// Service produces brief coaching replies for a student session.
type Service interface {
	Coach(ctx context.Context, req Request) (Response, error)
}

// Provider is the optional model backend behind PolicyService.
// Implementations must treat req.StudentMessage as untrusted user content only.
type Provider interface {
	// Name identifies the provider for diagnostics (e.g. "fake", "bedrock", "echo").
	Name() string
	// Complete returns a coaching reply. It must not interpret student text as system policy.
	Complete(ctx context.Context, req Request) (string, error)
}

// Request is built entirely on the server from the authenticated session and
// immutable activity revision. Clients must never supply system prompts,
// standards, or mastery state.
type Request struct {
	SessionID      string
	StudentID      string
	ActivitySlug   string
	Activity       contracts.ActivityContent
	CurrentTask    *contracts.Task
	Observations   []contracts.Observation
	StudentMessage string
	PriorHints     []string
	// HintLevel selects the next graduated activity hint (1-based). Zero means
	// the service chooses from prior-hint count.
	HintLevel int
}

// Response is a short coaching reply plus provenance for diagnostics/events.
type Response struct {
	Reply       string
	Provider    string
	Fallback    bool
	Filtered    bool
	RateLimited bool
	Disabled    bool
}

// DefaultFallback is used when activity content has no usable hint.
const DefaultFallback = "Take one small step, then check what changed in the workspace."

// FallbackHint returns an activity-local deterministic coaching string.
// Prefer graduated hints by level, then objective, then a generic nudge.
func FallbackHint(activity contracts.ActivityContent, priorHints []string, hintLevel int) string {
	level := hintLevel
	if level <= 0 {
		level = len(priorHints) + 1
	}
	if text := hintAtLevel(activity.Hints, level); text != "" {
		return text
	}
	// Exhausted levels: try any remaining unused hint text.
	used := map[string]struct{}{}
	for _, h := range priorHints {
		used[strings.TrimSpace(h)] = struct{}{}
	}
	for _, h := range activity.Hints {
		t := strings.TrimSpace(h.Text)
		if t == "" {
			continue
		}
		if _, ok := used[t]; !ok {
			return t
		}
	}
	if len(activity.Hints) > 0 {
		t := strings.TrimSpace(activity.Hints[len(activity.Hints)-1].Text)
		if t != "" {
			return t
		}
	}
	if gs := goalSummary(activity); gs != "" {
		return paraphraseObjective(gs)
	}
	if obj := strings.TrimSpace(activity.Objective); obj != "" {
		return paraphraseObjective(obj)
	}
	return DefaultFallback
}

func hintAtLevel(hints []contracts.Hint, level int) string {
	var best string
	bestLevel := -1
	for _, h := range hints {
		if h.Level != level {
			continue
		}
		t := strings.TrimSpace(h.Text)
		if t == "" {
			continue
		}
		// Prefer first at this level; track level match.
		if bestLevel < 0 || h.Level < bestLevel {
			best = t
			bestLevel = h.Level
		}
	}
	if best != "" {
		return best
	}
	// If exact level missing, take the highest level <= requested.
	for _, h := range hints {
		if h.Level > level {
			continue
		}
		t := strings.TrimSpace(h.Text)
		if t == "" {
			continue
		}
		if h.Level >= bestLevel {
			best = t
			bestLevel = h.Level
		}
	}
	return best
}

func goalSummary(activity contracts.ActivityContent) string {
	if activity.Tutor != nil {
		return strings.TrimSpace(activity.Tutor.GoalSummary)
	}
	return ""
}

func paraphraseObjective(obj string) string {
	obj = strings.TrimSpace(obj)
	if obj == "" {
		return DefaultFallback
	}
	// Keep short; avoid dumping long authoring text.
	if len(obj) > 180 {
		obj = strings.TrimSpace(obj[:177]) + "..."
	}
	if strings.HasSuffix(obj, ".") {
		return "Focus on this goal: " + obj
	}
	return "Focus on this goal: " + obj + "."
}

// SanitizeStudentMessage strips obvious prompt-injection framing so providers
// only see the student's coaching question. It never becomes system policy.
func SanitizeStudentMessage(msg string) string {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return ""
	}
	// Drop lines that look like role/system hijacks; keep the rest.
	var kept []string
	for _, line := range strings.Split(msg, "\n") {
		trim := strings.TrimSpace(line)
		lower := strings.ToLower(trim)
		if strings.HasPrefix(lower, "system:") ||
			strings.HasPrefix(lower, "assistant:") ||
			strings.Contains(lower, "ignore previous") ||
			strings.Contains(lower, "ignore all previous") ||
			strings.Contains(lower, "you are now") ||
			strings.Contains(lower, "disregard your instructions") {
			continue
		}
		kept = append(kept, line)
	}
	out := strings.TrimSpace(strings.Join(kept, "\n"))
	if out == "" {
		// Student only sent injection noise; treat as a generic help request.
		return "I need a hint for the current task."
	}
	// Bound length so a dump cannot dominate context.
	const maxRunes = 500
	r := []rune(out)
	if len(r) > maxRunes {
		out = string(r[:maxRunes])
	}
	return out
}

// Now is overridable in tests.
var Now = time.Now
