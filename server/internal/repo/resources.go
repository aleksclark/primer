package repo

import "github.com/aleksclark/primer/server/internal/domain"

// Repositories for each LMS resource. Search/sort/filter column lists must
// stay in sync with the migrations in internal/db/migrations. Selected
// columns are derived from domain struct db tags (see NewResource).

// Educators is the educator repository.
var Educators = NewResource[domain.Educator](ListConfig{
	Table:             "educators",
	SearchColumns:     []string{"email", "name"},
	SortableColumns:   []string{"email", "name", "role", "created_at", "updated_at"},
	FilterableColumns: []string{"role"},
})

// Students is the student repository.
var Students = NewResource[domain.Student](ListConfig{
	Table:             "students",
	SearchColumns:     []string{"first_name", "last_name", "notes"},
	SortableColumns:   []string{"first_name", "last_name", "grade_level", "birthdate", "created_at", "updated_at"},
	FilterableColumns: []string{"grade_level"},
})

// Subjects is the subject repository.
var Subjects = NewResource[domain.Subject](ListConfig{
	Table:             "subjects",
	SearchColumns:     []string{"code", "name", "description"},
	SortableColumns:   []string{"code", "name", "created_at", "updated_at"},
	FilterableColumns: []string{"code"},
})

// Standards is the standard repository.
var Standards = NewResource[domain.Standard](ListConfig{
	Table:             "standards",
	SearchColumns:     []string{"code", "description", "domain", "cluster"},
	SortableColumns:   []string{"code", "source", "grade_level", "domain", "created_at", "updated_at"},
	FilterableColumns: []string{"source", "subject_id", "parent_id", "grade_level", "domain", "tcap_weight"},
})

// Curricula is the curriculum repository.
var Curricula = NewResource[domain.Curriculum](ListConfig{
	Table:             "curricula",
	SearchColumns:     []string{"name", "description"},
	SortableColumns:   []string{"name", "approach", "grade_level", "created_at", "updated_at"},
	FilterableColumns: []string{"approach", "grade_level"},
})

// CurriculumStandards is the curriculum-standard placement repository.
var CurriculumStandards = NewResource[domain.CurriculumStandard](ListConfig{
	Table:             "curriculum_standards",
	SearchColumns:     []string{"unit", "notes"},
	SortableColumns:   []string{"unit", "position", "created_at", "updated_at"},
	FilterableColumns: []string{"curriculum_id", "standard_id", "unit"},
})

// Enrollments is the enrollment repository.
var Enrollments = NewResource[domain.Enrollment](ListConfig{
	Table:             "enrollments",
	SortableColumns:   []string{"status", "started_on", "ended_on", "created_at", "updated_at"},
	FilterableColumns: []string{"student_id", "curriculum_id", "status"},
})

// MasteryRecords is the mastery record repository.
var MasteryRecords = NewResource[domain.MasteryRecord](ListConfig{
	Table:             "mastery_records",
	SortableColumns:   []string{"status", "confidence", "last_assessed_at", "next_reinforcement_at", "created_at", "updated_at"},
	FilterableColumns: []string{"student_id", "standard_id", "status"},
})

// MasteryEvidences is the mastery evidence repository.
var MasteryEvidences = NewResource[domain.MasteryEvidence](ListConfig{
	Table:             "mastery_evidence",
	SearchColumns:     []string{"context", "source_ref", "migration_note"},
	SortableColumns:   []string{"kind", "evidence_class", "occurred_on", "created_at", "updated_at"},
	FilterableColumns: []string{"mastery_record_id", "kind", "evidence_class"},
})

// InstructionLogs is the instruction log repository. Instructional time
// arrives from outside the LMS (today, the TV server), so the log is searched
// by title and filtered by source and subject.
var InstructionLogs = NewResource[domain.InstructionLog](ListConfig{
	Table:             "instruction_logs",
	SearchColumns:     []string{"media_title", "notes", "source_ref"},
	SortableColumns:   []string{"occurred_on", "watched_seconds", "media_title", "source", "class", "created_at", "updated_at"},
	FilterableColumns: []string{"source", "class", "student_id", "occurred_on"},
})

// Assessments is the assessment repository.
var Assessments = NewResource[domain.Assessment](ListConfig{
	Table:             "assessments",
	SearchColumns:     []string{"title", "description"},
	SortableColumns:   []string{"title", "kind", "grade_level", "created_at", "updated_at"},
	FilterableColumns: []string{"kind", "subject_id", "curriculum_id", "grade_level"},
})

// AssessmentItems is the assessment item repository.
var AssessmentItems = NewResource[domain.AssessmentItem](ListConfig{
	Table:             "assessment_items",
	SearchColumns:     []string{"stem", "rationale"},
	SortableColumns:   []string{"position", "item_type", "difficulty", "points", "created_at", "updated_at"},
	FilterableColumns: []string{"assessment_id", "standard_id", "item_type", "difficulty"},
})

// AssessmentItemOptions is the item option repository.
var AssessmentItemOptions = NewResource[domain.AssessmentItemOption](ListConfig{
	Table:             "assessment_item_options",
	SearchColumns:     []string{"text", "feedback"},
	SortableColumns:   []string{"position", "correct", "created_at", "updated_at"},
	FilterableColumns: []string{"item_id", "correct"},
})

// AssessmentAttempts is the attempt repository.
var AssessmentAttempts = NewResource[domain.AssessmentAttempt](ListConfig{
	Table:             "assessment_attempts",
	SortableColumns:   []string{"status", "score", "started_at", "submitted_at", "created_at", "updated_at"},
	FilterableColumns: []string{"assessment_id", "student_id", "status"},
})

// ItemResponses is the item response repository.
var ItemResponses = NewResource[domain.ItemResponse](ListConfig{
	Table:             "item_responses",
	SearchColumns:     []string{"feedback"},
	SortableColumns:   []string{"is_correct", "points_awarded", "created_at", "updated_at"},
	FilterableColumns: []string{"attempt_id", "item_id", "is_correct"},
})
