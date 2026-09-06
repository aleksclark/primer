package domain

// Recovered from the original P4 dialogue configuration/snapshot boundary
// (dc8cedb0..89583ced), adapted to retained evidence and immutable issued policy.
import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

const (
	AgentDialogueKind          = "agent_dialogue"
	AgentDialogueConfigVersion = 1
	DialoguePolicyVersion      = "dialogue.v1"
	DialogueRequiredQuestions  = 3
	DialogueSourceMaxBytes     = 12000
)

var (
	ErrInvalidDialogueConfig = errors.New("invalid agent dialogue configuration")
	ErrDialogueSourceMissing = errors.New("dialogue source is required")
)

// DialogueConfig is parent-authored configuration, NOT a student command.
// Exactly one inline source or known embedded reference is allowed. The v1
// policy retains evidence; unsupported retention promises fail closed.
type DialogueConfig struct {
	SourceRef         string   `json:"sourceRef,omitempty"`
	SourceText        string   `json:"sourceText,omitempty"`
	LearningFocus     string   `json:"learningFocus"`
	RequiredQuestions int      `json:"requiredQuestions"`
	Rubric            []string `json:"rubric"`
	AllowedFollowUps  int      `json:"allowedFollowUps"`
	MaxAttempts       int      `json:"maxAttempts"`
	MaxTurns          int      `json:"maxTurns"`
	RetentionPolicy   string   `json:"retentionPolicy"`
}

// The original garden-wall fixture is an explicit, versioned source rather
// than a fallback for arbitrary chapters. Live-provider inline sources do not
// have to match this fixture. No reference ever causes an HTTP/file fetch.
const CuratedChapterSource = "Chapter 4 follows the family as they repair the garden wall after the storm. The narrator notices that the mortar must dry before the next course of stones can be laid, and that rushing the work would weaken the whole wall."

type DialogueSource struct {
	Reference string `json:"reference"`
	Version   string `json:"version"`
	Text      string `json:"text"`
	SHA256    string `json:"sha256"`
}

func dialogueDigest(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func ResolveDialogueSource(c DialogueConfig) (DialogueSource, error) {
	if c.SourceRef == "" && strings.TrimSpace(c.SourceText) == "" {
		return DialogueSource{}, ErrDialogueSourceMissing
	}
	if c.SourceRef != "" && c.SourceText != "" {
		return DialogueSource{}, ErrInvalidDialogueConfig
	}
	source := DialogueSource{Reference: "inline", Version: "inline.v1", Text: c.SourceText}
	if c.SourceRef != "" {
		if c.SourceRef != "fixture://chapter-4" {
			return DialogueSource{}, ErrInvalidDialogueConfig
		}
		source = DialogueSource{Reference: c.SourceRef, Version: "garden-wall.v1", Text: CuratedChapterSource}
	}
	if !utf8.ValidString(source.Text) || len(source.Text) > DialogueSourceMaxBytes || strings.TrimSpace(source.Text) == "" {
		return DialogueSource{}, ErrInvalidDialogueConfig
	}
	source.SHA256 = dialogueDigest([]byte(source.Text))
	return source, nil
}

func (c DialogueConfig) Validate() error {
	if _, err := ResolveDialogueSource(c); err != nil {
		return err
	}
	if strings.TrimSpace(c.LearningFocus) == "" || !utf8.ValidString(c.LearningFocus) || len([]rune(c.LearningFocus)) > 500 || c.RequiredQuestions != DialogueRequiredQuestions {
		return ErrInvalidDialogueConfig
	}
	if len(c.Rubric) < 1 || len(c.Rubric) > 20 {
		return ErrInvalidDialogueConfig
	}
	seen := map[string]bool{}
	for _, criterion := range c.Rubric {
		key := strings.ToLower(strings.TrimSpace(criterion))
		if key == "" || !utf8.ValidString(criterion) || len([]rune(criterion)) > 500 || seen[key] {
			return ErrInvalidDialogueConfig
		}
		seen[key] = true
	}
	// A turn is one admitted student answer, never a question/tool step.
	if c.AllowedFollowUps < 0 || c.AllowedFollowUps > 5 || c.MaxAttempts < 1 || c.MaxAttempts > 10 || c.MaxTurns < DialogueRequiredQuestions || c.MaxTurns > 100 || c.RetentionPolicy != "retain" {
		return ErrInvalidDialogueConfig
	}
	return nil
}

func (c DialogueConfig) Criteria() []string { return append([]string(nil), c.Rubric...) }

// ParseDialogueConfig rejects unknown and case-aliased keys, including client
// source substitutions, retentionDays, prompts and unsupported retention modes.
// Create/revise/publish share this boundary; stored envelopes are not rewritten.
func ParseDialogueConfig(raw map[string]any) (DialogueConfig, error) {
	allowed := map[string]bool{"sourceRef": true, "sourceText": true, "learningFocus": true, "requiredQuestions": true, "rubric": true, "allowedFollowUps": true, "maxAttempts": true, "maxTurns": true, "retentionPolicy": true}
	for key, value := range raw {
		if !allowed[key] || value == nil {
			return DialogueConfig{}, ErrInvalidDialogueConfig
		}
		if (key == "sourceRef" || key == "sourceText") && value == "" {
			return DialogueConfig{}, ErrInvalidDialogueConfig
		}
	}
	for _, key := range []string{"learningFocus", "requiredQuestions", "rubric", "allowedFollowUps", "maxAttempts", "maxTurns", "retentionPolicy"} {
		if _, ok := raw[key]; !ok {
			return DialogueConfig{}, ErrInvalidDialogueConfig
		}
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return DialogueConfig{}, ErrInvalidDialogueConfig
	}
	var c DialogueConfig
	if err = json.Unmarshal(b, &c); err != nil {
		return c, ErrInvalidDialogueConfig
	}
	return c, c.Validate()
}

func ValidateDialogueRequirement(r VerificationRequirement) error {
	if r.Kind != AgentDialogueKind || r.ConfigVersion != AgentDialogueConfigVersion || r.Interaction != "chat" || r.Executor != "fantasy" {
		return ErrInvalidDialogueConfig
	}
	_, err := ParseDialogueConfig(r.Config)
	return err
}

func SnapshotDialogueConfig(c DialogueConfig) (map[string]any, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	b, err := json.Marshal(c)
	if err != nil {
		return nil, err
	}
	var snapshot map[string]any
	err = json.Unmarshal(b, &snapshot)
	return snapshot, err
}

// DialogueConfigFromRequirementJSON reads the canonical array envelope used by
// config->0 consumers. It neither changes storage nor accepts a donor bare map.
// The row's server-owned kind/version/interaction/executor must be checked too.
func DialogueConfigFromRequirementJSON(raw []byte) (DialogueConfig, error) {
	var envelope []VerificationRequirement
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&envelope); err != nil || len(envelope) != 1 {
		return DialogueConfig{}, ErrInvalidDialogueConfig
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return DialogueConfig{}, ErrInvalidDialogueConfig
	}
	if err := ValidateDialogueRequirement(envelope[0]); err != nil {
		return DialogueConfig{}, err
	}
	return ParseDialogueConfig(envelope[0].Config)
}

// DialogueSnapshot is server-created from the issued revision, not accepted
// from a task form or student socket. Its digest binds source, rubric and limits
// to the revision/requirement identity as well as the policy version.
type DialogueSnapshot struct {
	RevisionID      string         `json:"revisionId"`
	RevisionVersion int            `json:"revisionVersion"`
	RequirementID   string         `json:"requirementId"`
	PolicyVersion   string         `json:"policyVersion"`
	Config          DialogueConfig `json:"config"`
	Source          DialogueSource `json:"source"`
	Digest          string         `json:"digest"`
}

func NewDialogueSnapshot(revisionID, requirementID string, revisionVersion int, c DialogueConfig) (DialogueSnapshot, error) {
	if strings.TrimSpace(revisionID) == "" || strings.TrimSpace(requirementID) == "" || revisionVersion < 1 {
		return DialogueSnapshot{}, ErrInvalidDialogueConfig
	}
	if err := c.Validate(); err != nil {
		return DialogueSnapshot{}, err
	}
	c.Rubric = c.Criteria()
	source, err := ResolveDialogueSource(c)
	if err != nil {
		return DialogueSnapshot{}, err
	}
	s := DialogueSnapshot{RevisionID: revisionID, RevisionVersion: revisionVersion, RequirementID: requirementID, PolicyVersion: DialoguePolicyVersion, Config: c, Source: source}
	b, err := json.Marshal(s)
	if err != nil {
		return DialogueSnapshot{}, err
	}
	s.Digest = dialogueDigest(b)
	return s, nil
}

func (s DialogueSnapshot) Validate() error {
	expected, err := NewDialogueSnapshot(s.RevisionID, s.RequirementID, s.RevisionVersion, s.Config)
	if err != nil || s.PolicyVersion != expected.PolicyVersion || s.Source != expected.Source || s.Digest != expected.Digest {
		return fmt.Errorf("%w: snapshot binding", ErrInvalidDialogueConfig)
	}
	return nil
}
