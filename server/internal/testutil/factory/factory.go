// Package factory provides FactoryBot-style test data builders. Each factory
// inserts a row through the repo layer with sensible defaults, accepting
// override maps for per-test customization. Associations are created
// automatically unless the relevant foreign key is provided.
package factory

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aleksclark/primer/server/internal/domain"
	"github.com/aleksclark/primer/server/internal/repo"
)

var seq atomic.Int64

// n returns a process-unique sequence number for generating distinct values.
func n() int64 { return seq.Add(1) }

// Override is a column->value map merged over factory defaults.
type Override map[string]any

func merge(defaults map[string]any, overrides []Override) map[string]any {
	out := make(map[string]any, len(defaults))
	for k, v := range defaults {
		out[k] = v
	}
	for _, o := range overrides {
		for k, v := range o {
			out[k] = v
		}
	}
	return out
}

func create[T any](t *testing.T, q repo.Querier, res *repo.Resource[T], values map[string]any) *T {
	t.Helper()
	item, err := res.Create(context.Background(), q, values)
	if err != nil {
		t.Fatalf("factory create %T: %v", *new(T), err)
	}
	return item
}

// Educator creates an educator.
func Educator(t *testing.T, q repo.Querier, overrides ...Override) *domain.Educator {
	i := n()
	return create(t, q, repo.Educators, merge(map[string]any{
		"email": fmt.Sprintf("educator%d@example.com", i),
		"name":  fmt.Sprintf("Educator %d", i),
		"role":  "parent",
	}, overrides))
}

// Student creates a student.
func Student(t *testing.T, q repo.Querier, overrides ...Override) *domain.Student {
	i := n()
	return create(t, q, repo.Students, merge(map[string]any{
		"first_name":  fmt.Sprintf("Student%d", i),
		"last_name":   "Example",
		"grade_level": 6,
		"notes":       "",
	}, overrides))
}

// Subject creates a subject.
func Subject(t *testing.T, q repo.Querier, overrides ...Override) *domain.Subject {
	i := n()
	return create(t, q, repo.Subjects, merge(map[string]any{
		"code": fmt.Sprintf("subj-%d", i),
		"name": fmt.Sprintf("Subject %d", i),
	}, overrides))
}

// Standard creates a standard, creating a subject unless subject_id is given.
func Standard(t *testing.T, q repo.Querier, overrides ...Override) *domain.Standard {
	i := n()
	defaults := map[string]any{
		"source":      "custom",
		"code":        fmt.Sprintf("STD.%d", i),
		"grade_level": 6,
		"domain":      "test-domain",
		"description": fmt.Sprintf("Standard %d description", i),
	}
	merged := merge(defaults, overrides)
	if _, ok := merged["subject_id"]; !ok {
		merged["subject_id"] = Subject(t, q).ID
	}
	return create(t, q, repo.Standards, merged)
}

// Curriculum creates a curriculum.
func Curriculum(t *testing.T, q repo.Querier, overrides ...Override) *domain.Curriculum {
	i := n()
	return create(t, q, repo.Curricula, merge(map[string]any{
		"name":        fmt.Sprintf("Curriculum %d", i),
		"approach":    "mastery_based",
		"grade_level": 6,
	}, overrides))
}

// CurriculumStandard places a standard in a curriculum, creating both unless
// their IDs are provided.
func CurriculumStandard(t *testing.T, q repo.Querier, overrides ...Override) *domain.CurriculumStandard {
	merged := merge(map[string]any{
		"unit":     "Unit 1",
		"position": int(n()),
	}, overrides)
	if _, ok := merged["curriculum_id"]; !ok {
		merged["curriculum_id"] = Curriculum(t, q).ID
	}
	if _, ok := merged["standard_id"]; !ok {
		merged["standard_id"] = Standard(t, q).ID
	}
	return create(t, q, repo.CurriculumStandards, merged)
}

// Enrollment enrolls a student in a curriculum, creating both unless IDs are
// provided.
func Enrollment(t *testing.T, q repo.Querier, overrides ...Override) *domain.Enrollment {
	merged := merge(map[string]any{
		"status": "active",
	}, overrides)
	if _, ok := merged["student_id"]; !ok {
		merged["student_id"] = Student(t, q).ID
	}
	if _, ok := merged["curriculum_id"]; !ok {
		merged["curriculum_id"] = Curriculum(t, q).ID
	}
	return create(t, q, repo.Enrollments, merged)
}

// MasteryRecord creates a mastery record, creating student and standard
// unless IDs are provided.
func MasteryRecord(t *testing.T, q repo.Querier, overrides ...Override) *domain.MasteryRecord {
	merged := merge(map[string]any{
		"status":     "in_progress",
		"confidence": 0.5,
		"decay_rate": 0.02,
	}, overrides)
	if _, ok := merged["student_id"]; !ok {
		merged["student_id"] = Student(t, q).ID
	}
	if _, ok := merged["standard_id"]; !ok {
		merged["standard_id"] = Standard(t, q).ID
	}
	return create(t, q, repo.MasteryRecords, merged)
}

// MasteryEvidence creates evidence, creating a mastery record unless
// mastery_record_id is provided.
func MasteryEvidence(t *testing.T, q repo.Querier, overrides ...Override) *domain.MasteryEvidence {
	merged := merge(map[string]any{
		"kind":        "continuous",
		"occurred_on": time.Now().UTC().Truncate(24 * time.Hour),
		"context":     "Solved 5/5 practice problems",
	}, overrides)
	if _, ok := merged["mastery_record_id"]; !ok {
		merged["mastery_record_id"] = MasteryRecord(t, q).ID
	}
	return create(t, q, repo.MasteryEvidences, merged)
}

// Assessment creates an assessment.
func Assessment(t *testing.T, q repo.Querier, overrides ...Override) *domain.Assessment {
	i := n()
	return create(t, q, repo.Assessments, merge(map[string]any{
		"title": fmt.Sprintf("Assessment %d", i),
		"kind":  "quiz",
	}, overrides))
}

// AssessmentItem creates an item, creating an assessment unless
// assessment_id is provided.
func AssessmentItem(t *testing.T, q repo.Querier, overrides ...Override) *domain.AssessmentItem {
	i := n()
	merged := merge(map[string]any{
		"position":   int(i),
		"item_type":  "mc",
		"difficulty": "on_track",
		"stem":       fmt.Sprintf("Question %d?", i),
		"points":     1.0,
	}, overrides)
	if _, ok := merged["assessment_id"]; !ok {
		merged["assessment_id"] = Assessment(t, q).ID
	}
	return create(t, q, repo.AssessmentItems, merged)
}

// AssessmentItemOption creates an option, creating an item unless item_id is
// provided.
func AssessmentItemOption(t *testing.T, q repo.Querier, overrides ...Override) *domain.AssessmentItemOption {
	i := n()
	merged := merge(map[string]any{
		"position": int(i),
		"text":     fmt.Sprintf("Option %d", i),
		"correct":  false,
	}, overrides)
	if _, ok := merged["item_id"]; !ok {
		merged["item_id"] = AssessmentItem(t, q).ID
	}
	return create(t, q, repo.AssessmentItemOptions, merged)
}

// AssessmentAttempt creates an attempt, creating assessment and student
// unless IDs are provided.
func AssessmentAttempt(t *testing.T, q repo.Querier, overrides ...Override) *domain.AssessmentAttempt {
	merged := merge(map[string]any{
		"status": "in_progress",
	}, overrides)
	if _, ok := merged["assessment_id"]; !ok {
		merged["assessment_id"] = Assessment(t, q).ID
	}
	if _, ok := merged["student_id"]; !ok {
		merged["student_id"] = Student(t, q).ID
	}
	return create(t, q, repo.AssessmentAttempts, merged)
}

// ItemResponse creates a response. If attempt_id or item_id are missing, a
// coherent attempt+item pair is created on the same assessment.
func ItemResponse(t *testing.T, q repo.Querier, overrides ...Override) *domain.ItemResponse {
	merged := merge(map[string]any{
		"response": map[string]any{"selected": []string{"a"}},
	}, overrides)
	_, hasAttempt := merged["attempt_id"]
	_, hasItem := merged["item_id"]
	if !hasAttempt || !hasItem {
		assessment := Assessment(t, q)
		if !hasAttempt {
			merged["attempt_id"] = AssessmentAttempt(t, q, Override{"assessment_id": assessment.ID}).ID
		}
		if !hasItem {
			merged["item_id"] = AssessmentItem(t, q, Override{"assessment_id": assessment.ID}).ID
		}
	}
	return create(t, q, repo.ItemResponses, merged)
}
