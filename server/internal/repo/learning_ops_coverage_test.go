package repo_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/domain"
	"github.com/aleksclark/primer/server/internal/repo"
	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
	"github.com/aleksclark/primer/server/internal/testutil"
	"github.com/aleksclark/primer/server/internal/testutil/factory"
)

func TestLearningDraftPublishPolicyAndSafeContent(t *testing.T) {
	t.Parallel()
	q := testutil.NewSavepointQuerier(testutil.Tx(t))
	ctx := context.Background()
	subj := factory.Subject(t, q, factory.Override{"code": "digital-literacy"})
	std := factory.Standard(t, q, factory.Override{
		"subject_id": subj.ID,
		"code":       "PRIMER.DL.6.NAV.1",
	})

	// ParseEvidencePolicy edges.
	_, ok := repo.ParseEvidencePolicy(nil)
	assert.False(t, ok)
	_, ok = repo.ParseEvidencePolicy(map[string]any{})
	assert.False(t, ok)
	_, ok = repo.ParseEvidencePolicy(map[string]any{"version": 1})
	assert.False(t, ok)
	pol, ok := repo.ParseEvidencePolicy(map[string]any{
		"version": 1,
		"statusRequirements": map[string]any{
			"mastered": []any{domain.EvidenceProceduralContinuous},
		},
	})
	assert.True(t, ok)
	assert.Equal(t, 1, pol.Version)

	// Draft activity + publish revision.
	slug := "draft-act-" + uuid.NewString()[:8]
	act, err := repo.CreateDraftActivity(ctx, q, slug, "Draft Title", "summary", contracts.KindTerminal, &subj.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.ActivityStatusDraft, act.Status)

	// Without subject id.
	act2, err := repo.CreateDraftActivity(ctx, q, slug+"-b", "T2", "", contracts.KindTyping, nil)
	require.NoError(t, err)
	assert.Equal(t, contracts.KindTyping, act2.Kind)

	content := contracts.ActivityContent{
		Objective:    "learn",
		Instructions: "do it",
		Blocks: []contracts.InstructionBlock{
			{Kind: contracts.BlockParentNote, Text: "secret parent note"},
			{Kind: contracts.BlockProse, Text: "student body"},
		},
		Tasks: []contracts.Task{{
			ID: "t1", Title: "Task", Instructions: "go",
			Completion: contracts.CheckTree{CheckID: "c1"},
		}},
		Checks: []contracts.Check{{
			ID: "c1", Kind: contracts.CheckFileExists,
			Params: map[string]any{"path": "a.txt"},
		}},
	}
	now := time.Now().UTC()
	rev, err := repo.PublishDraftRevision(ctx, q, act.ID, content, "", []contracts.StandardRef{
		{Code: "PRIMER.DL.6.NAV.1", Role: "", Weight: 0},
		{Code: "PRIMER.DL.6.NAV.1", Role: contracts.StandardRoleReinforcement}, // dup skipped
	}, map[string]string{"PRIMER.DL.6.NAV.1": std.ID}, now)
	require.NoError(t, err)
	require.NotNil(t, rev)
	assert.Equal(t, 1, rev.Revision)

	// Unknown standard rejected (different content so digest path is not short-circuited).
	contentBad := content
	contentBad.Objective = "unknown-std-path"
	_, err = repo.PublishDraftRevision(ctx, q, act.ID, contentBad, contracts.SchemaVersion, []contracts.StandardRef{
		{Code: "NOPE.1"},
	}, map[string]string{}, now)
	require.Error(t, err)

	// Idempotent same digest.
	rev2, err := repo.PublishDraftRevision(ctx, q, act.ID, content, contracts.SchemaVersion, nil, nil, now)
	require.NoError(t, err)
	assert.Equal(t, rev.ID, rev2.ID)

	// List revision standards.
	links, err := repo.ListRevisionStandards(ctx, q, rev.ID)
	require.NoError(t, err)
	require.Len(t, links, 1)
	assert.Equal(t, contracts.StandardRolePrimary, links[0].Role)
	assert.Equal(t, 1.0, links[0].Weight)

	// Explicit evidence policy on link.
	std2 := factory.Standard(t, q, factory.Override{
		"subject_id": subj.ID,
		"code":       "PRIMER.DL.6.NAV.2",
	})
	customPol := contracts.DefaultTerminalEvidencePolicy()
	customPol.Version = 2
	content2 := content
	content2.Objective = "learn more"
	rev3, err := repo.PublishDraftRevision(ctx, q, act.ID, content2, contracts.SchemaVersion, []contracts.StandardRef{
		{Code: "PRIMER.DL.6.NAV.2", Role: contracts.StandardRoleReinforcement, Weight: 2, EvidencePolicy: &customPol},
	}, map[string]string{"PRIMER.DL.6.NAV.2": std2.ID}, now.Add(time.Second))
	require.NoError(t, err)
	assert.GreaterOrEqual(t, rev3.Revision, 2)
	assert.NotEqual(t, rev.ID, rev3.ID)

	// Student-safe content strips parent notes.
	safe := repo.StudentSafeRevisionContent(rev.Content)
	require.NotNil(t, safe)
	blocks, _ := safe["blocks"].([]any)
	// Should not contain parent_note kinds.
	for _, b := range blocks {
		m, _ := b.(map[string]any)
		assert.NotEqual(t, contracts.BlockParentNote, m["kind"])
	}
	// Nil / no blocks / already safe.
	assert.Nil(t, repo.StudentSafeRevisionContent(nil))
	same := repo.StudentSafeRevisionContent(map[string]any{"objective": "x"})
	assert.Equal(t, "x", same["objective"])
	// Invalid content returns original.
	bad := map[string]any{"tasks": "not-an-array"}
	out := repo.StudentSafeRevisionContent(bad)
	assert.Equal(t, bad, out)

	// CreateAssignmentFull with enrollment provenance.
	student := factory.Student(t, q)
	en := factory.Enrollment(t, q, factory.Override{"student_id": student.ID})
	enID := en.ID
	asg, err := repo.CreateAssignmentFull(ctx, q, repo.AssignmentCreate{
		StudentID:          student.ID,
		ActivityRevisionID: rev.ID,
		EnrollmentID:       &enID,
		SelectionReason:    "course-next",
		Priority:           7,
		Reason:             "assign",
	})
	require.NoError(t, err)
	require.NotNil(t, asg.EnrollmentID)
	assert.Equal(t, en.ID, *asg.EnrollmentID)
	assert.Equal(t, "course-next", asg.SelectionReason)
}

func TestTypingDefaultPolicyOnPublish(t *testing.T) {
	t.Parallel()
	q := testutil.NewSavepointQuerier(testutil.Tx(t))
	ctx := context.Background()
	subj := factory.Subject(t, q)
	std := factory.Standard(t, q, factory.Override{"subject_id": subj.ID, "code": "PRIMER.TYPE.TEST.1"})
	act, err := repo.CreateDraftActivity(ctx, q, "type-"+uuid.NewString()[:8], "Type", "", contracts.KindTyping, &subj.ID)
	require.NoError(t, err)
	content := contracts.ActivityContent{
		Objective: "type",
		Tasks: []contracts.Task{{
			ID: "t1", Title: "T", Instructions: "type",
			Completion: contracts.CheckTree{CheckID: "c1"},
		}},
		Checks: []contracts.Check{{ID: "c1", Kind: contracts.CheckFileExists, Params: map[string]any{"path": "a"}}},
	}
	rev, err := repo.PublishDraftRevision(ctx, q, act.ID, content, contracts.SchemaVersion, []contracts.StandardRef{
		{Code: "PRIMER.TYPE.TEST.1"},
	}, map[string]string{"PRIMER.TYPE.TEST.1": std.ID}, time.Now().UTC())
	require.NoError(t, err)
	links, err := repo.ListRevisionStandards(ctx, q, rev.ID)
	require.NoError(t, err)
	require.Len(t, links, 1)
	pol, ok := repo.ParseEvidencePolicy(links[0].EvidencePolicy)
	require.True(t, ok)
	// Typing default should differ from empty.
	assert.Greater(t, pol.Version, 0)
}
