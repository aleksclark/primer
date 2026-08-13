package overseer

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/domain"
	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
)

func TestRuntimeProfileFromContent(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "", runtimeProfileFromContent(nil))
	assert.Equal(t, "", runtimeProfileFromContent(map[string]any{"objective": "x"}))
	assert.Equal(t, "", runtimeProfileFromContent(map[string]any{"terminal": "not-a-map"}))
	assert.Equal(t, "", runtimeProfileFromContent(map[string]any{"terminal": map[string]any{}}))
	assert.Equal(t, "coreutils-basic", runtimeProfileFromContent(map[string]any{
		"terminal": map[string]any{"runtimeProfile": "  coreutils-basic  "},
	}))
	assert.Equal(t, "text-processing", runtimeProfileFromContent(map[string]any{
		"terminal": map[string]any{"runtime_profile": " text-processing "},
	}))
	// camelCase wins when both present
	assert.Equal(t, "a", runtimeProfileFromContent(map[string]any{
		"terminal": map[string]any{"runtimeProfile": "a", "runtime_profile": "b"},
	}))
}

func TestRuntimeProfilesFromCaps(t *testing.T) {
	t.Parallel()
	assert.Empty(t, runtimeProfilesFromCaps(nil))
	assert.Empty(t, runtimeProfilesFromCaps(map[string]any{"other": true}))
	assert.Empty(t, runtimeProfilesFromCaps(map[string]any{"runtimeProfiles": "bad"}))

	fromAny := runtimeProfilesFromCaps(map[string]any{
		"runtimeProfiles": []any{"coreutils-basic", "", 3, "text-processing"},
	})
	assert.True(t, fromAny["coreutils-basic"])
	assert.True(t, fromAny["text-processing"])
	assert.False(t, fromAny[""])

	fromStr := runtimeProfilesFromCaps(map[string]any{
		"runtimeProfiles": []string{"coreutils-basic", "", "x"},
	})
	assert.True(t, fromStr["coreutils-basic"])
	assert.True(t, fromStr["x"])
	assert.Len(t, fromStr, 2)
}

func TestPrerequisiteSatisfied(t *testing.T) {
	t.Parallel()
	completed := map[string]bool{"a1": true}
	mastery := map[string]string{"STD.1": "mastered"}

	// default / empty requirement → completed
	assert.True(t, prerequisiteSatisfied(domain.CurriculumActivityPrerequisite{
		RequiresSlug: "a1",
	}, completed, mastery))
	assert.False(t, prerequisiteSatisfied(domain.CurriculumActivityPrerequisite{
		RequiresSlug: "missing",
	}, completed, mastery))

	assert.True(t, prerequisiteSatisfied(domain.CurriculumActivityPrerequisite{
		RequiresSlug: "a1", Requirement: contracts.PrereqCompleted,
	}, completed, mastery))

	// approaching: completion is enough
	assert.True(t, prerequisiteSatisfied(domain.CurriculumActivityPrerequisite{
		RequiresSlug: "a1", Requirement: contracts.PrereqApproaching,
	}, completed, mastery))
	assert.False(t, prerequisiteSatisfied(domain.CurriculumActivityPrerequisite{
		RequiresSlug: "missing", Requirement: contracts.PrereqApproaching,
	}, completed, mastery))

	// mastered: completion alone is never enough (strict MVP)
	assert.False(t, prerequisiteSatisfied(domain.CurriculumActivityPrerequisite{
		RequiresSlug: "a1", Requirement: contracts.PrereqMastered,
	}, completed, mastery))
	assert.False(t, prerequisiteSatisfied(domain.CurriculumActivityPrerequisite{
		RequiresSlug: "missing", Requirement: contracts.PrereqMastered,
	}, completed, mastery))

	// unknown requirement falls back to completed
	assert.True(t, prerequisiteSatisfied(domain.CurriculumActivityPrerequisite{
		RequiresSlug: "a1", Requirement: "custom-unknown",
	}, completed, mastery))
}

func TestEvaluateMembershipStatuses(t *testing.T) {
	t.Parallel()
	rev := "rev-1"
	m := domain.CurriculumActivity{ActivitySlug: "lesson-2", ActivityRevisionID: &rev}
	enActive := domain.Enrollment{Status: "active"}
	enPaused := domain.Enrollment{Status: "paused"}
	enWithdrawn := domain.Enrollment{Status: "withdrawn"}

	// paused enrollment
	st := evaluateMembership(m, enPaused, nil, nil, nil, nil, nil, nil, nil)
	require.False(t, st.Eligible)
	assert.Equal(t, "blocked", st.Status)
	require.NotEmpty(t, st.BlockingReasons)
	assert.Equal(t, BlockEnrollmentPaused, st.BlockingReasons[0].Code)

	// inactive enrollment
	st = evaluateMembership(m, enWithdrawn, nil, nil, nil, nil, nil, nil, nil)
	require.False(t, st.Eligible)
	assert.Equal(t, BlockEnrollmentInactive, st.BlockingReasons[0].Code)
	assert.Contains(t, st.BlockingReasons[0].Message, "withdrawn")

	// missing activity revision
	st = evaluateMembership(domain.CurriculumActivity{ActivitySlug: "x"}, enActive, nil, nil, nil, nil, nil, nil, nil)
	require.False(t, st.Eligible)
	assert.True(t, hasCode(st.BlockingReasons, BlockMissingActivityRev))

	// already completed
	st = evaluateMembership(m, enActive, nil, nil, map[string]bool{"lesson-2": true}, nil, nil, nil, nil)
	assert.Equal(t, "completed", st.Status)
	assert.Equal(t, BlockAlreadyCompleted, st.BlockingReasons[0].Code)
	assert.False(t, st.Eligible)

	// already open/assigned
	st = evaluateMembership(m, enActive, nil, nil, nil, map[string]bool{"lesson-2": true}, nil, nil, nil)
	assert.Equal(t, "assigned", st.Status)
	assert.Equal(t, BlockAlreadyOpen, st.BlockingReasons[0].Code)

	// prerequisite unmet
	st = evaluateMembership(m, enActive, []domain.CurriculumActivityPrerequisite{{
		ActivitySlug: "lesson-2", RequiresSlug: "lesson-1", Requirement: contracts.PrereqCompleted,
	}}, nil, nil, nil, nil, nil, nil)
	assert.Equal(t, "blocked", st.Status)
	assert.True(t, hasCode(st.BlockingReasons, BlockPrerequisiteUnmet))
	assert.Equal(t, "lesson-1", st.BlockingReasons[0].Requires)

	// pending parent review on prerequisite
	st = evaluateMembership(m, enActive, []domain.CurriculumActivityPrerequisite{{
		ActivitySlug: "lesson-2", RequiresSlug: "lesson-1", Requirement: contracts.PrereqCompleted,
	}}, nil, map[string]bool{"lesson-1": true}, nil, map[string]bool{"lesson-1": true}, nil, nil)
	assert.Equal(t, "review_needed", st.Status)
	assert.True(t, hasCode(st.BlockingReasons, BlockParentReviewPending))

	// parent_review gate on the activity itself while pending
	st = evaluateMembership(m, enActive, nil, []domain.CurriculumActivityGate{{
		ActivitySlug: "lesson-2", Kind: contracts.GateParentReview,
	}}, nil, nil, map[string]bool{"lesson-2": true}, nil, nil)
	assert.Equal(t, "review_needed", st.Status)
	assert.True(t, hasCode(st.BlockingReasons, BlockParentReviewPending))

	// evidence gate unmet
	st = evaluateMembership(m, enActive, nil, []domain.CurriculumActivityGate{{
		ActivitySlug: "lesson-2", Kind: contracts.GateEvidence, Standards: []string{"PRIMER.DL.6.NAV.1"},
	}}, nil, nil, nil, map[string]string{"PRIMER.DL.6.NAV.1": "in_progress"}, nil)
	assert.Equal(t, "blocked", st.Status)
	assert.True(t, hasCode(st.BlockingReasons, BlockEvidenceGateUnmet))

	// evidence gate met via approaching
	st = evaluateMembership(m, enActive, nil, []domain.CurriculumActivityGate{{
		ActivitySlug: "lesson-2", Kind: contracts.GateEvidence, Standards: []string{"PRIMER.DL.6.NAV.1"},
	}}, nil, nil, nil, map[string]string{"PRIMER.DL.6.NAV.1": "approaching"}, nil)
	assert.True(t, st.Eligible)
	assert.Equal(t, "eligible", st.Status)

	// pin override skips prereq
	enPin := domain.Enrollment{Status: "active", PinnedActivitySlug: "lesson-2"}
	st = evaluateMembership(m, enPin, []domain.CurriculumActivityPrerequisite{{
		ActivitySlug: "lesson-2", RequiresSlug: "lesson-1", Requirement: contracts.PrereqCompleted,
	}}, nil, nil, nil, nil, nil, nil)
	assert.True(t, st.Eligible)

	// override slug skips prereq
	st = evaluateMembership(m, enActive, []domain.CurriculumActivityPrerequisite{{
		ActivitySlug: "lesson-2", RequiresSlug: "lesson-1",
	}}, nil, nil, nil, nil, nil, map[string]bool{"lesson-2": true})
	assert.True(t, st.Eligible)

	// fully eligible baseline
	st = evaluateMembership(m, enActive, nil, nil, nil, nil, nil, nil, nil)
	assert.True(t, st.Eligible)
	assert.Equal(t, "eligible", st.Status)
}

func TestHasCodeAndDedupeBlockReasons(t *testing.T) {
	t.Parallel()
	blocks := []BlockReason{
		{Code: "a", Activity: "x", Message: "m1"},
		{Code: "b", Activity: "y", Message: "m2"},
	}
	assert.True(t, hasCode(blocks, "a"))
	assert.False(t, hasCode(blocks, "z"))
	assert.False(t, hasCode(nil, "a"))

	in := []BlockReason{
		{Code: "a", Activity: "x", Message: "m"},
		{Code: "a", Activity: "x", Message: "m"},
		{Code: "a", Activity: "x", Message: "other"},
		{Code: "b", Activity: "y", Requires: "r", Message: "m"},
	}
	out := dedupeBlockReasons(in)
	require.Len(t, out, 3)
	assert.Equal(t, "a", out[0].Code)
	assert.Equal(t, "other", out[1].Message)
	assert.Equal(t, "b", out[2].Code)
}
