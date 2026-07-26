package repo

import "github.com/aleksclark/primer/server/internal/domain"

// Repositories for each LMS resource. Column lists must stay in sync with
// the migrations in internal/db/migrations.

// Educators is the educator repository.
var Educators = NewResource[domain.Educator](ListConfig{
	Table:             "educators",
	Columns:           []string{"id", "email", "name", "role", "created_at", "updated_at"},
	SearchColumns:     []string{"email", "name"},
	SortableColumns:   []string{"email", "name", "role", "created_at", "updated_at"},
	FilterableColumns: []string{"role"},
})

// Students is the student repository.
var Students = NewResource[domain.Student](ListConfig{
	Table:             "students",
	Columns:           []string{"id", "first_name", "last_name", "birthdate", "grade_level", "notes", "created_at", "updated_at"},
	SearchColumns:     []string{"first_name", "last_name", "notes"},
	SortableColumns:   []string{"first_name", "last_name", "grade_level", "birthdate", "created_at", "updated_at"},
	FilterableColumns: []string{"grade_level"},
})

// Subjects is the subject repository.
var Subjects = NewResource[domain.Subject](ListConfig{
	Table:             "subjects",
	Columns:           []string{"id", "code", "name", "description", "created_at", "updated_at"},
	SearchColumns:     []string{"code", "name", "description"},
	SortableColumns:   []string{"code", "name", "created_at", "updated_at"},
	FilterableColumns: []string{"code"},
})

// Standards is the standard repository.
var Standards = NewResource[domain.Standard](ListConfig{
	Table:             "standards",
	Columns:           []string{"id", "source", "subject_id", "parent_id", "code", "grade_level", "domain", "cluster", "description", "tcap_weight", "metadata", "created_at", "updated_at"},
	SearchColumns:     []string{"code", "description", "domain", "cluster"},
	SortableColumns:   []string{"code", "source", "grade_level", "domain", "created_at", "updated_at"},
	FilterableColumns: []string{"source", "subject_id", "parent_id", "grade_level", "domain", "tcap_weight"},
})

// Curricula is the curriculum repository.
var Curricula = NewResource[domain.Curriculum](ListConfig{
	Table:             "curricula",
	Columns:           []string{"id", "name", "description", "approach", "grade_level", "metadata", "created_at", "updated_at"},
	SearchColumns:     []string{"name", "description"},
	SortableColumns:   []string{"name", "approach", "grade_level", "created_at", "updated_at"},
	FilterableColumns: []string{"approach", "grade_level"},
})

// CurriculumStandards is the curriculum-standard placement repository.
var CurriculumStandards = NewResource[domain.CurriculumStandard](ListConfig{
	Table:             "curriculum_standards",
	Columns:           []string{"id", "curriculum_id", "standard_id", "unit", "position", "notes", "created_at", "updated_at"},
	SearchColumns:     []string{"unit", "notes"},
	SortableColumns:   []string{"unit", "position", "created_at", "updated_at"},
	FilterableColumns: []string{"curriculum_id", "standard_id", "unit"},
})

// Enrollments is the enrollment repository.
var Enrollments = NewResource[domain.Enrollment](ListConfig{
	Table:             "enrollments",
	Columns:           []string{"id", "student_id", "curriculum_id", "status", "started_on", "ended_on", "created_at", "updated_at"},
	SearchColumns:     nil,
	SortableColumns:   []string{"status", "started_on", "ended_on", "created_at", "updated_at"},
	FilterableColumns: []string{"student_id", "curriculum_id", "status"},
})

// MasteryRecords is the mastery record repository.
var MasteryRecords = NewResource[domain.MasteryRecord](ListConfig{
	Table:             "mastery_records",
	Columns:           []string{"id", "student_id", "standard_id", "status", "confidence", "decay_rate", "last_assessed_at", "last_reinforced_at", "next_reinforcement_at", "created_at", "updated_at"},
	SearchColumns:     nil,
	SortableColumns:   []string{"status", "confidence", "last_assessed_at", "next_reinforcement_at", "created_at", "updated_at"},
	FilterableColumns: []string{"student_id", "standard_id", "status"},
})

// MasteryEvidences is the mastery evidence repository.
var MasteryEvidences = NewResource[domain.MasteryEvidence](ListConfig{
	Table:             "mastery_evidence",
	Columns:           []string{"id", "mastery_record_id", "kind", "occurred_on", "context", "source_ref", "created_at", "updated_at"},
	SearchColumns:     []string{"context", "source_ref"},
	SortableColumns:   []string{"kind", "occurred_on", "created_at", "updated_at"},
	FilterableColumns: []string{"mastery_record_id", "kind"},
})

// Assessments is the assessment repository.
var Assessments = NewResource[domain.Assessment](ListConfig{
	Table:             "assessments",
	Columns:           []string{"id", "title", "description", "kind", "subject_id", "curriculum_id", "grade_level", "metadata", "created_at", "updated_at"},
	SearchColumns:     []string{"title", "description"},
	SortableColumns:   []string{"title", "kind", "grade_level", "created_at", "updated_at"},
	FilterableColumns: []string{"kind", "subject_id", "curriculum_id", "grade_level"},
})

// AssessmentItems is the assessment item repository.
var AssessmentItems = NewResource[domain.AssessmentItem](ListConfig{
	Table:             "assessment_items",
	Columns:           []string{"id", "assessment_id", "standard_id", "position", "item_type", "difficulty", "stem", "rationale", "points", "metadata", "created_at", "updated_at"},
	SearchColumns:     []string{"stem", "rationale"},
	SortableColumns:   []string{"position", "item_type", "difficulty", "points", "created_at", "updated_at"},
	FilterableColumns: []string{"assessment_id", "standard_id", "item_type", "difficulty"},
})

// AssessmentItemOptions is the item option repository.
var AssessmentItemOptions = NewResource[domain.AssessmentItemOption](ListConfig{
	Table:             "assessment_item_options",
	Columns:           []string{"id", "item_id", "position", "text", "correct", "feedback", "created_at", "updated_at"},
	SearchColumns:     []string{"text", "feedback"},
	SortableColumns:   []string{"position", "correct", "created_at", "updated_at"},
	FilterableColumns: []string{"item_id", "correct"},
})

// AssessmentAttempts is the attempt repository.
var AssessmentAttempts = NewResource[domain.AssessmentAttempt](ListConfig{
	Table:             "assessment_attempts",
	Columns:           []string{"id", "assessment_id", "student_id", "status", "score", "max_score", "started_at", "submitted_at", "scored_at", "created_at", "updated_at"},
	SearchColumns:     nil,
	SortableColumns:   []string{"status", "score", "started_at", "submitted_at", "created_at", "updated_at"},
	FilterableColumns: []string{"assessment_id", "student_id", "status"},
})

// ItemResponses is the item response repository.
var ItemResponses = NewResource[domain.ItemResponse](ListConfig{
	Table:             "item_responses",
	Columns:           []string{"id", "attempt_id", "item_id", "response", "is_correct", "points_awarded", "feedback", "created_at", "updated_at"},
	SearchColumns:     []string{"feedback"},
	SortableColumns:   []string{"is_correct", "points_awarded", "created_at", "updated_at"},
	FilterableColumns: []string{"attempt_id", "item_id", "is_correct"},
})
