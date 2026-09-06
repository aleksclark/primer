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
	"reflect"
	"strings"
	"unicode/utf8"
)

const (
	AgentDialogueKind          = "agent_dialogue"
	AgentDialogueConfigVersion = 1
	AgentDialogueInteraction   = "chat"
	AgentDialogueExecutor      = "fantasy"
	DialoguePolicyVersion      = "dialogue.v1"
	DialogueRequiredQuestions  = 3
	DialogueSourceMaxBytes     = 12000
	DialogueFocusMaxRunes      = 500
	DialogueRubricMaxItems     = 20
	DialogueCriterionMaxRunes  = 500
	DialogueFollowUpsMax       = 5
	DialogueAttemptsMax        = 10
	DialogueTurnsMax           = 100
	DialogueRetentionPolicy    = "retain"
	DialogueCuratedSourceRef   = "fixture://chapter-4"
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
		if c.SourceRef != DialogueCuratedSourceRef {
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
	if strings.TrimSpace(c.LearningFocus) == "" || !utf8.ValidString(c.LearningFocus) || len([]rune(c.LearningFocus)) > DialogueFocusMaxRunes || c.RequiredQuestions != DialogueRequiredQuestions {
		return ErrInvalidDialogueConfig
	}
	if len(c.Rubric) < 1 || len(c.Rubric) > DialogueRubricMaxItems {
		return ErrInvalidDialogueConfig
	}
	seen := map[string]bool{}
	for _, criterion := range c.Rubric {
		key := strings.ToLower(strings.TrimSpace(criterion))
		if key == "" || !utf8.ValidString(criterion) || len([]rune(criterion)) > DialogueCriterionMaxRunes || seen[key] {
			return ErrInvalidDialogueConfig
		}
		seen[key] = true
	}
	// A turn is one admitted student answer, never a question/tool step.
	if c.AllowedFollowUps < 0 || c.AllowedFollowUps > DialogueFollowUpsMax || c.MaxAttempts < 1 || c.MaxAttempts > DialogueAttemptsMax || c.MaxTurns < DialogueRequiredQuestions || c.MaxTurns > DialogueTurnsMax || c.RetentionPolicy != DialogueRetentionPolicy {
		return ErrInvalidDialogueConfig
	}
	return nil
}

func (c DialogueConfig) Criteria() []string { return append([]string(nil), c.Rubric...) }

// ParseDialogueConfig rejects unknown and case-aliased keys, including client
// source substitutions, retentionDays, prompts and unsupported retention modes.
// Create/revise/publish share this boundary; stored envelopes are not rewritten.
func ParseDialogueConfig(raw map[string]any) (DialogueConfig, error) {
	allowed := map[string]bool{}
	shape := DialogueConfigSchema()
	for key := range shape["properties"].(map[string]any) {
		allowed[key] = true
	}
	for key, value := range raw {
		if !allowed[key] || value == nil {
			return DialogueConfig{}, ErrInvalidDialogueConfig
		}
		if (key == "sourceRef" || key == "sourceText") && value == "" {
			return DialogueConfig{}, ErrInvalidDialogueConfig
		}
	}
	for _, key := range shape["required"].([]string) {
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

// DialogueConfigSchema reflects the actual parent-input Go type. Bounds and
// supported values are the SAME constants enforced by Validate/Resolve above.
// It is offline metadata, never a student-authority or published-plan input.
func DialogueConfigSchema() map[string]any {
	properties := map[string]any{}
	required := []string{}
	names := map[string]string{}
	t := reflect.TypeOf(DialogueConfig{})
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		tag := strings.Split(field.Tag.Get("json"), ",")
		name := tag[0]
		if name == "" || name == "-" {
			continue
		}
		names[field.Name] = name
		p := map[string]any{}
		switch field.Type.Kind() {
		case reflect.String:
			p["type"] = "string"
		case reflect.Int:
			p["type"] = "integer"
		case reflect.Bool:
			p["type"] = "boolean"
		case reflect.Slice:
			p["type"] = "array"
			p["items"] = map[string]any{"type": "string"}
		default:
			panic("unsupported dialogue config type")
		}
		switch field.Name {
		case "SourceRef":
			p["const"] = DialogueCuratedSourceRef
		case "SourceText":
			p["minLength"] = 1
			p["x-maxBytes"] = DialogueSourceMaxBytes
			p["x-nonBlank"] = true
		case "LearningFocus":
			p["minLength"] = 1
			p["maxLength"] = DialogueFocusMaxRunes
			p["x-nonBlank"] = true
		case "RequiredQuestions":
			p["const"] = DialogueRequiredQuestions
		case "Rubric":
			p["minItems"] = 1
			p["maxItems"] = DialogueRubricMaxItems
			p["uniqueItems"] = true
			p["x-uniqueNormalized"] = true
			p["items"] = map[string]any{"type": "string", "minLength": 1, "maxLength": DialogueCriterionMaxRunes, "x-nonBlank": true}
		case "AllowedFollowUps":
			p["minimum"] = 0
			p["maximum"] = DialogueFollowUpsMax
		case "MaxAttempts":
			p["minimum"] = 1
			p["maximum"] = DialogueAttemptsMax
		case "MaxTurns":
			p["minimum"] = DialogueRequiredQuestions
			p["maximum"] = DialogueTurnsMax
		case "RetentionPolicy":
			p["const"] = DialogueRetentionPolicy
		}
		properties[name] = p
		optional := false
		for _, option := range tag[1:] {
			optional = optional || option == "omitempty"
		}
		if !optional {
			required = append(required, name)
		}
	}
	return map[string]any{"$schema": "https://json-schema.org/draft/2020-12/schema", "title": "DialogueConfig", "x-manifest": map[string]any{"kind": AgentDialogueKind, "configVersion": AgentDialogueConfigVersion, "interaction": AgentDialogueInteraction, "executor": AgentDialogueExecutor}, "type": "object", "properties": properties, "required": required, "additionalProperties": false, "oneOf": []any{
		map[string]any{"required": []string{names["SourceRef"]}, "not": map[string]any{"required": []string{names["SourceText"]}}},
		map[string]any{"required": []string{names["SourceText"]}, "not": map[string]any{"required": []string{names["SourceRef"]}}},
	}}
}

func ValidateDialogueRequirement(r VerificationRequirement) error {
	if r.Kind != AgentDialogueKind || r.ConfigVersion != AgentDialogueConfigVersion || r.Interaction != AgentDialogueInteraction || r.Executor != AgentDialogueExecutor {
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
const DialogueQuestionPlanVersion = "dialogue.questions.v1"

// Question wording is affirmative server authority, never a provider-prose
// channel. Inline sources use closed, source-neutral comprehension templates;
// the embedded chapter retains its source-specific, explicitly curated plan.
type DialoguePlannedQuestion struct {
	Key    string `json:"key"`
	Prompt string `json:"prompt"`
}

func dialogueQuestionPlan(source DialogueSource) []DialoguePlannedQuestion {
	if source.Text == CuratedChapterSource {
		return []DialoguePlannedQuestion{
			{Key: "wall", Prompt: "What did the family repair after the storm?"},
			{Key: "mortar", Prompt: "Why must the mortar dry before the next course of stones?"},
			{Key: "rushing", Prompt: "What would rushing the work do to the wall?"},
		}
	}
	return []DialoguePlannedQuestion{
		{Key: "source-fact", Prompt: "What is one important fact in the assigned source?"},
		{Key: "source-evidence", Prompt: "Which different detail in the assigned source supports your first answer?"},
		{Key: "source-connection", Prompt: "How does another detail in the assigned source connect to the facts you have explained?"},
	}
}

type DialogueSnapshot struct {
	RevisionID          string                    `json:"revisionId"`
	RevisionVersion     int                       `json:"revisionVersion"`
	RequirementID       string                    `json:"requirementId"`
	PolicyVersion       string                    `json:"policyVersion"`
	Config              DialogueConfig            `json:"config"`
	Source              DialogueSource            `json:"source"`
	QuestionPlanVersion string                    `json:"questionPlanVersion"`
	Questions           []DialoguePlannedQuestion `json:"questions"`
	Digest              string                    `json:"digest"`
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
	s := DialogueSnapshot{RevisionID: revisionID, RevisionVersion: revisionVersion, RequirementID: requirementID, PolicyVersion: DialoguePolicyVersion, Config: c, Source: source, QuestionPlanVersion: DialogueQuestionPlanVersion, Questions: dialogueQuestionPlan(source)}
	b, err := json.Marshal(s)
	if err != nil {
		return DialogueSnapshot{}, err
	}
	s.Digest = dialogueDigest(b)
	return s, nil
}

func (s DialogueSnapshot) Validate() error {
	expected, err := NewDialogueSnapshot(s.RevisionID, s.RequirementID, s.RevisionVersion, s.Config)
	if err != nil || s.PolicyVersion != expected.PolicyVersion || s.Source != expected.Source || s.Digest != expected.Digest || s.QuestionPlanVersion != expected.QuestionPlanVersion || !reflect.DeepEqual(s.Questions, expected.Questions) {
		return fmt.Errorf("%w: snapshot binding", ErrInvalidDialogueConfig)
	}
	return nil
}
