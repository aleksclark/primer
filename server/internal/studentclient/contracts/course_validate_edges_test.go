package contracts_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
)

func validCourse() *contracts.CourseDocument {
	return &contracts.CourseDocument{
		SchemaVersion:  contracts.CourseSchemaVersion,
		Slug:           "sample-course",
		Title:          "Sample",
		SubjectCode:    "digital-literacy",
		RevisionPolicy: contracts.RevisionPolicyLatestPublished,
		PacingReference: &contracts.CoursePacing{
			NominalWeeks: 4, NominalDaysPerWeek: 3, NominalMinutesPerDay: 30,
		},
		ContinuityDefaults: &contracts.ContinuityPolicy{Mode: contracts.ContinuityFresh},
		Activities: []contracts.CourseActivityRef{
			{Order: 1, Slug: "basic-navigation", File: "activities/basic-navigation/activity.yaml"},
			{Order: 2, Slug: "file-organization", Capstone: true, Continuity: &contracts.ContinuityPolicy{Mode: contracts.ContinuityOptionalPrevious}},
		},
		Modules: []contracts.CourseModule{{
			ID: "mod-1", Title: "Unit 1", Activities: []string{"basic-navigation", "file-organization"},
		}},
		Prerequisites: []contracts.CoursePrerequisite{{
			Activity: "file-organization", Requires: []string{"basic-navigation"}, Requirement: contracts.PrereqCompleted,
		}},
		Gates: []contracts.CourseGate{{
			Activity: "file-organization", Kind: contracts.GateParentReview, Description: "review",
			Standards: []string{"PRIMER.DL.6.NAV.1"},
		}},
		Remediation: []contracts.CourseRemediation{{
			ForActivity: "basic-navigation", BranchSlug: "file-organization", Kind: "remediation",
		}},
	}
}

func TestValidateCourseDocumentEdges(t *testing.T) {
	t.Parallel()
	require.NoError(t, contracts.ValidateCourseDocument(validCourse()))

	require.Error(t, contracts.ValidateCourseDocument(nil))

	doc := validCourse()
	doc.SchemaVersion = "9"
	require.Error(t, contracts.ValidateCourseDocument(doc))

	doc = validCourse()
	doc.Slug = "BAD SLUG"
	require.Error(t, contracts.ValidateCourseDocument(doc))

	doc = validCourse()
	doc.Title = "  "
	require.Error(t, contracts.ValidateCourseDocument(doc))

	doc = validCourse()
	doc.SubjectCode = ""
	require.Error(t, contracts.ValidateCourseDocument(doc))

	doc = validCourse()
	doc.RevisionPolicy = "unknown"
	require.Error(t, contracts.ValidateCourseDocument(doc))

	doc = validCourse()
	doc.PacingReference = &contracts.CoursePacing{NominalWeeks: -1}
	require.Error(t, contracts.ValidateCourseDocument(doc))

	doc = validCourse()
	doc.ContinuityDefaults = &contracts.ContinuityPolicy{Mode: ""}
	require.Error(t, contracts.ValidateCourseDocument(doc))

	doc = validCourse()
	doc.ContinuityDefaults = &contracts.ContinuityPolicy{Mode: "weird"}
	require.Error(t, contracts.ValidateCourseDocument(doc))

	doc = validCourse()
	doc.Activities = nil
	require.Error(t, contracts.ValidateCourseDocument(doc))

	doc = validCourse()
	doc.Activities[0].Order = 0
	require.Error(t, contracts.ValidateCourseDocument(doc))

	doc = validCourse()
	doc.Activities[1].Order = 1 // duplicate order
	require.Error(t, contracts.ValidateCourseDocument(doc))

	doc = validCourse()
	doc.Activities[0].Slug = "Bad!"
	require.Error(t, contracts.ValidateCourseDocument(doc))

	doc = validCourse()
	doc.Activities[1].Slug = "basic-navigation" // dup slug
	require.Error(t, contracts.ValidateCourseDocument(doc))

	doc = validCourse()
	doc.Activities[0].File = "/abs/path"
	require.Error(t, contracts.ValidateCourseDocument(doc))

	doc = validCourse()
	doc.Activities[0].File = "../escape"
	require.Error(t, contracts.ValidateCourseDocument(doc))

	doc = validCourse()
	doc.Activities[0].Continuity = &contracts.ContinuityPolicy{Mode: "nope"}
	require.Error(t, contracts.ValidateCourseDocument(doc))

	// Non-contiguous orders.
	doc = validCourse()
	doc.Activities[1].Order = 3
	require.Error(t, contracts.ValidateCourseDocument(doc))

	// Module edges.
	doc = validCourse()
	doc.Modules[0].ID = "bad id"
	require.Error(t, contracts.ValidateCourseDocument(doc))

	doc = validCourse()
	doc.Modules = append(doc.Modules, contracts.CourseModule{ID: "mod-1", Title: "dup"})
	require.Error(t, contracts.ValidateCourseDocument(doc))

	doc = validCourse()
	doc.Modules[0].Title = " "
	require.Error(t, contracts.ValidateCourseDocument(doc))

	doc = validCourse()
	doc.Modules[0].Activities = []string{"missing-slug"}
	require.Error(t, contracts.ValidateCourseDocument(doc))

	// Prerequisites.
	doc = validCourse()
	doc.Prerequisites[0].Activity = "nope"
	require.Error(t, contracts.ValidateCourseDocument(doc))

	doc = validCourse()
	doc.Prerequisites[0].Requires = nil
	require.Error(t, contracts.ValidateCourseDocument(doc))

	doc = validCourse()
	doc.Prerequisites[0].Requirement = "maybe"
	require.Error(t, contracts.ValidateCourseDocument(doc))

	doc = validCourse()
	doc.Prerequisites[0].Requires = []string{"nope"}
	require.Error(t, contracts.ValidateCourseDocument(doc))

	doc = validCourse()
	doc.Prerequisites[0].Requires = []string{"file-organization"} // self
	doc.Prerequisites[0].Activity = "file-organization"
	require.Error(t, contracts.ValidateCourseDocument(doc))

	// Cycle a->b->a
	doc = validCourse()
	doc.Prerequisites = []contracts.CoursePrerequisite{
		{Activity: "basic-navigation", Requires: []string{"file-organization"}},
		{Activity: "file-organization", Requires: []string{"basic-navigation"}},
	}
	require.Error(t, contracts.ValidateCourseDocument(doc))

	// Gates.
	doc = validCourse()
	doc.Gates[0].Activity = "nope"
	require.Error(t, contracts.ValidateCourseDocument(doc))

	doc = validCourse()
	doc.Gates[0].Kind = "mystery"
	require.Error(t, contracts.ValidateCourseDocument(doc))

	doc = validCourse()
	doc.Gates[0].Standards = []string{"not a code"}
	require.Error(t, contracts.ValidateCourseDocument(doc))

	// Remediation.
	doc = validCourse()
	doc.Remediation[0].ForActivity = "nope"
	require.Error(t, contracts.ValidateCourseDocument(doc))

	doc = validCourse()
	doc.Remediation[0].BranchSlug = "Bad!"
	require.Error(t, contracts.ValidateCourseDocument(doc))

	doc = validCourse()
	doc.Remediation[0].Kind = "other"
	require.Error(t, contracts.ValidateCourseDocument(doc))

	// Valid evidence gate + reinforcement remediation + pinned digest policy.
	doc = validCourse()
	doc.RevisionPolicy = contracts.RevisionPolicyPinnedDigest
	doc.Gates[0].Kind = contracts.GateEvidence
	doc.Remediation[0].Kind = "reinforcement"
	doc.Prerequisites[0].Requirement = contracts.PrereqMastered
	require.NoError(t, contracts.ValidateCourseDocument(doc))

	// Defaults helpers.
	_ = contracts.DefaultTerminalEvidencePolicy()
	_ = contracts.DefaultTypingEvidencePolicy()

	// TaskKindOrDefault
	if got := contracts.TaskKindOrDefault(contracts.Task{}); got != contracts.TaskKindAction {
		t.Fatalf("empty kind default: %q", got)
	}
	if got := contracts.TaskKindOrDefault(contracts.Task{Kind: contracts.TaskKindShortResponse}); got != contracts.TaskKindShortResponse {
		t.Fatalf("explicit kind: %q", got)
	}
}
