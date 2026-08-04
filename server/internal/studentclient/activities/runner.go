// Package activities defines the shared runner interface for local activity kinds.
//
// Terminal and typing sessions live under engine/ and diverge in I/O, but both
// expose Kind() via SessionSnapshot and follow the same lifecycle:
//
//	Open(assignment) → interact → evaluate checks → complete → sync
//
// New runners should implement Runner and be selected by activity kind without
// leaking kind-specific types into the work-queue or server protocol.
package activities

import (
	"context"

	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
)

// Runner is a local activity executor for one activity kind (terminal, typing, …).
// Implementations own kind-specific interaction; the host TUI/engine owns
// assignment loading, event outbox, and completion submission.
type Runner interface {
	// Kind returns the activity kind this runner handles (e.g. contracts.KindTerminal).
	Kind() string

	// Open prepares workspace/state for the given immutable content.
	// workspace is an absolute path the runner may write under.
	Open(ctx context.Context, workspace string, content contracts.ActivityContent) error

	// Close releases runner resources (PTY, temp files, etc.).
	Close() error
}

// KindOf returns content-derived kind or empty string.
func KindOf(kind string) string {
	switch kind {
	case contracts.KindTerminal, contracts.KindTyping:
		return kind
	default:
		return kind
	}
}
