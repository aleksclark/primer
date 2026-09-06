//go:build race

package api

// The compiler, not argv/environment inference, determines parent race mode.
const dialogueParentRace = true
