package agent

import (
	"strings"

	"primer-tasks/internal/domain"
)

// CuratedDialogueFixture is a deterministic, three-question wiring fixture.
// It is intentionally small and is not an educational quality benchmark.
type CuratedDialogueFixture struct {
	SourceRef string
	Questions []FixtureQuestion
}

type FixtureQuestion struct {
	Key      string
	Prompt   string
	Concept  string
	Keywords []string
}

func CuratedThreeQuestionFixture() CuratedDialogueFixture {
	return CuratedDialogueFixture{
		SourceRef: "fixture://chapter-4",
		Questions: []FixtureQuestion{
			{Key: "conflict", Prompt: "What is the chapter's central conflict?", Concept: "central conflict", Keywords: []string{"conflict", "belonging"}},
			{Key: "evidence", Prompt: "Name one detail that supports the chapter's central idea.", Concept: "textual evidence", Keywords: []string{"detail", "evidence", "example"}},
			{Key: "consequence", Prompt: "What consequence follows from the protagonist's choice?", Concept: "cause and consequence", Keywords: []string{"consequence", "result", "because"}},
		},
	}
}

func (f CuratedDialogueFixture) Config() domain.DialogueConfig {
	return domain.DialogueConfig{
		SourceRef: f.SourceRef, LearningFocus: "identify a claim, support it with evidence, and explain consequence",
		RequiredQuestions: 3, Rubric: []string{"answers address the distinct question", "answers use evidence or a reason"},
		AllowedFollowUps: 1, MaxAttempts: 2, MaxTurns: 8, RetentionPolicy: "retain",
	}
}

func (f CuratedDialogueFixture) Evaluate(key, answer string) (bool, string) {
	answer = strings.ToLower(strings.TrimSpace(answer))
	for _, q := range f.Questions {
		if q.Key != key {
			continue
		}
		for _, keyword := range q.Keywords {
			if strings.Contains(answer, keyword) {
				return true, "answer addresses the distinct criterion"
			}
		}
		return false, "answer needs a specific detail or reason"
	}
	return false, "question is not part of the curated fixture"
}
