package domain

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const (
	AgentDialogueKind          = "agent_dialogue"
	AgentDialogueConfigVersion = 1
)

var (
	ErrInvalidDialogueConfig = errors.New("invalid agent dialogue configuration")
	ErrDialogueSourceMissing = errors.New("dialogue source is required")
)

// DialogueConfig is the parent-authored, server-owned policy for a dialogue
// requirement. SourceText is deliberately bounded: callers must not use it as
// a place to smuggle a system prompt or an answer key.
type DialogueConfig struct {
	SourceRef          string   `json:"sourceRef,omitempty"`
	SourceText         string   `json:"sourceText,omitempty"`
	LearningFocus      string   `json:"learningFocus"`
	RequiredQuestions  int      `json:"requiredQuestions"`
	Rubric             []string `json:"rubric"`
	AcceptanceCriteria []string `json:"acceptanceCriteria,omitempty"`
	AllowedFollowUps   int      `json:"allowedFollowUps"`
	MaxAttempts        int      `json:"maxAttempts"`
	MaxTurns           int      `json:"maxTurns"`
	RetentionPolicy    string   `json:"retentionPolicy"`
}

// AgentDialogueConfig is the explicit manifest name used by task authors.
type AgentDialogueConfig = DialogueConfig

func (c DialogueConfig) Validate() error {
	if strings.TrimSpace(c.SourceRef) == "" && strings.TrimSpace(c.SourceText) == "" {
		return ErrDialogueSourceMissing
	}
	if len(strings.TrimSpace(c.SourceText)) > 12000 {
		return fmt.Errorf("%w: source text exceeds 12000 characters", ErrInvalidDialogueConfig)
	}
	if strings.TrimSpace(c.LearningFocus) == "" || len([]rune(c.LearningFocus)) > 500 {
		return fmt.Errorf("%w: learning focus is required and bounded", ErrInvalidDialogueConfig)
	}
	if c.RequiredQuestions < 1 || c.RequiredQuestions > 10 {
		return fmt.Errorf("%w: required questions must be between 1 and 10", ErrInvalidDialogueConfig)
	}
	criteria := c.Criteria()
	if len(criteria) == 0 || len(criteria) > 20 {
		return fmt.Errorf("%w: at least one bounded rubric criterion is required", ErrInvalidDialogueConfig)
	}
	for _, criterion := range criteria {
		if strings.TrimSpace(criterion) == "" || len([]rune(criterion)) > 500 {
			return fmt.Errorf("%w: rubric criterion is empty or too long", ErrInvalidDialogueConfig)
		}
	}
	if c.AllowedFollowUps < 0 || c.AllowedFollowUps > 5 {
		return fmt.Errorf("%w: follow-ups must be between 0 and 5", ErrInvalidDialogueConfig)
	}
	if c.MaxAttempts < 1 || c.MaxAttempts > 10 || c.MaxTurns < c.RequiredQuestions || c.MaxTurns > 100 {
		return fmt.Errorf("%w: attempt and turn bounds are invalid", ErrInvalidDialogueConfig)
	}
	if p := strings.TrimSpace(c.RetentionPolicy); p != "retain" && p != "redact" && p != "delete_after_review" {
		return fmt.Errorf("%w: unsupported retention policy", ErrInvalidDialogueConfig)
	}
	return nil
}

func (c DialogueConfig) Criteria() []string {
	if len(c.Rubric) != 0 {
		return append([]string(nil), c.Rubric...)
	}
	return append([]string(nil), c.AcceptanceCriteria...)
}

// ValidateDialogueRequirement validates the manifest envelope and its typed
// configuration before a revision can be published.
func ValidateDialogueRequirement(r VerificationRequirement) error {
	if r.Kind != AgentDialogueKind || r.ConfigVersion != AgentDialogueConfigVersion {
		return ErrInvalidDialogueConfig
	}
	if r.Interaction != "chat" || r.Executor != "fantasy" {
		return fmt.Errorf("%w: dialogue must use chat/fantasy", ErrInvalidDialogueConfig)
	}
	b, err := json.Marshal(r.Config)
	if err != nil {
		return fmt.Errorf("%w: malformed config", ErrInvalidDialogueConfig)
	}
	var c DialogueConfig
	if err := json.Unmarshal(b, &c); err != nil {
		return fmt.Errorf("%w: malformed config", ErrInvalidDialogueConfig)
	}
	return c.Validate()
}

// SnapshotDialogueConfig returns a canonical JSON-safe copy. It is intended to
// be stored in an occurrence/attempt and never reads mutable task state again.
func SnapshotDialogueConfig(c DialogueConfig) (map[string]any, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	c.Rubric = c.Criteria()
	c.AcceptanceCriteria = nil
	b, err := json.Marshal(c)
	if err != nil {
		return nil, fmt.Errorf("%w: cannot snapshot", ErrInvalidDialogueConfig)
	}
	var snapshot map[string]any
	if err := json.Unmarshal(b, &snapshot); err != nil {
		return nil, fmt.Errorf("%w: cannot snapshot", ErrInvalidDialogueConfig)
	}
	return snapshot, nil
}

func ValidateAgentDialogueConfig(c DialogueConfig) error { return c.Validate() }

func SnapshotAgentDialogueConfig(c DialogueConfig) (map[string]any, error) {
	return SnapshotDialogueConfig(c)
}

func ParseDialogueConfig(raw map[string]any) (DialogueConfig, error) {
	b, err := json.Marshal(raw)
	if err != nil {
		return DialogueConfig{}, fmt.Errorf("%w: malformed config", ErrInvalidDialogueConfig)
	}
	var c DialogueConfig
	if err := json.Unmarshal(b, &c); err != nil {
		return DialogueConfig{}, fmt.Errorf("%w: malformed config", ErrInvalidDialogueConfig)
	}
	if err := c.Validate(); err != nil {
		return DialogueConfig{}, err
	}
	return c, nil
}
