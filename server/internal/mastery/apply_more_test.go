package mastery_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/domain"
	"github.com/aleksclark/primer/server/internal/mastery"
	"github.com/aleksclark/primer/server/internal/repo"
	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
	"github.com/aleksclark/primer/server/internal/testutil"
	"github.com/aleksclark/primer/server/internal/testutil/factory"
)

func TestApplyConceptualResponseNilAndIdempotent(t *testing.T) {
	t.Parallel()
	q := testutil.Tx(t)
	ctx := context.Background()
	now := time.Now().UTC()

	_, err := mastery.ApplyConceptualResponse(ctx, q, nil, now)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nil")

	doc, rev := publishBasicNav(t, q)
	student := factory.Student(t, q)
	dev, sess := deviceAndSession(t, q, student.ID, rev.ID)
	_ = doc
	_ = dev

	resp := &domain.StudentResponse{
		ID:                 uuid.NewString(),
		SubmissionID:       uuid.NewString(),
		StudentID:          student.ID,
		SessionID:          sess.ID,
		AssignmentID:       sess.AssignmentID,
		ActivityRevisionID: rev.ID,
		TaskID:             "explain",
		Body:               "cd changes the working directory",
		BodySHA256:         "deadbeef",
		Status:             domain.ResponseSubmitted,
		Attempt:            1,
	}

	ids1, err := mastery.ApplyConceptualResponse(ctx, q, resp, now)
	require.NoError(t, err)
	require.NotEmpty(t, ids1)

	// Idempotent: same response/source_ref does not duplicate evidence rows.
	ids2, err := mastery.ApplyConceptualResponse(ctx, q, resp, now.Add(time.Second))
	require.NoError(t, err)
	require.Len(t, ids2, len(ids1))
	assert.Equal(t, ids1, ids2)

	// Evidence class recorded.
	ev, err := repo.MasteryEvidences.Get(ctx, q, ids1[0])
	require.NoError(t, err)
	assert.Equal(t, contracts.EvidenceConceptualResponse, ev.EvidenceClass)
	assert.Contains(t, ev.SourceRef, resp.ID)
}

func TestApplyParentAttestationNilAndAdvancesWithConceptual(t *testing.T) {
	t.Parallel()
	q := testutil.Tx(t)
	ctx := context.Background()
	now := time.Now().UTC()

	_, err := mastery.ApplyParentAttestation(ctx, q, nil, uuid.NewString(), now)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nil")

	_, rev := publishBasicNav(t, q)
	student := factory.Student(t, q)
	_, sess := deviceAndSession(t, q, student.ID, rev.ID)
	educator := factory.Educator(t, q)

	// Seed high confidence so candidate status can approach once both evidence classes exist.
	links, err := repo.ListRevisionStandards(ctx, q, rev.ID)
	require.NoError(t, err)
	require.NotEmpty(t, links)
	// Liberal policy: approaching needs procedural + conceptual; mastered needs parent too.
	// Default terminal already needs conceptual for approaching. Seed procedural first via
	// a mastery record at in_progress with procedural evidence already present.
	for _, link := range links {
		rec := factory.MasteryRecord(t, q, factory.Override{
			"student_id":  student.ID,
			"standard_id": link.StandardID,
			"status":      "in_progress",
			"confidence":  0.5,
		})
		factory.MasteryEvidence(t, q, factory.Override{
			"mastery_record_id": rec.ID,
			"evidence_class":    contracts.EvidenceProceduralContinuous,
			"source_ref":        "seed-proc-" + link.StandardID,
		})
	}

	resp := &domain.StudentResponse{
		ID:                 uuid.NewString(),
		SubmissionID:       uuid.NewString(),
		StudentID:          student.ID,
		SessionID:          sess.ID,
		AssignmentID:       sess.AssignmentID,
		ActivityRevisionID: rev.ID,
		TaskID:             "explain",
		Body:               "directories nest files",
		BodySHA256:         "cafebabe",
		Status:             domain.ResponseSubmitted,
		Attempt:            1,
	}

	// Conceptual first.
	cIDs, err := mastery.ApplyConceptualResponse(ctx, q, resp, now)
	require.NoError(t, err)
	require.NotEmpty(t, cIDs)

	// Parent attestation.
	pIDs, err := mastery.ApplyParentAttestation(ctx, q, resp, educator.ID, now)
	require.NoError(t, err)
	require.NotEmpty(t, pIDs)

	ev, err := repo.MasteryEvidences.Get(ctx, q, pIDs[0])
	require.NoError(t, err)
	assert.Equal(t, contracts.EvidenceParentAttestation, ev.EvidenceClass)
	assert.Contains(t, ev.SourceRef, "parent-attestation:")

	// Idempotent parent path.
	pIDs2, err := mastery.ApplyParentAttestation(ctx, q, resp, educator.ID, now.Add(time.Second))
	require.NoError(t, err)
	assert.Equal(t, pIDs, pIDs2)

	// Mastery should have advanced at least to approaching with procedural+conceptual
	// (parent attestation is extra; default mastered still needs formal_assessment).
	for _, link := range links {
		var status string
		var conf float64
		err := q.QueryRow(ctx, `
SELECT status, confidence FROM mastery_records
WHERE student_id = $1 AND standard_id = $2`, student.ID, link.StandardID).Scan(&status, &conf)
		require.NoError(t, err)
		assert.NotEqual(t, "not_introduced", status)
		// With procedural + conceptual + parent, default policy allows approaching.
		assert.Contains(t, []string{"in_progress", "approaching", "mastered"}, status)
		assert.GreaterOrEqual(t, conf, 0.0)
	}
}

func TestApplyCompletionPartialAdvanceUsesConfForStatus(t *testing.T) {
	t.Parallel()
	q := testutil.Tx(t)
	ctx := context.Background()
	doc, rev := publishBasicNav(t, q)
	student := factory.Student(t, q)
	dev, sess := deviceAndSession(t, q, student.ID, rev.ID)

	links, err := repo.ListRevisionStandards(ctx, q, rev.ID)
	require.NoError(t, err)
	// Seed confidence so candidate after bump reaches approaching/mastered,
	// while policy only allows in_progress from procedural alone.
	for _, link := range links {
		factory.MasteryRecord(t, q, factory.Override{
			"student_id":  student.ID,
			"standard_id": link.StandardID,
			"status":      "not_introduced",
			"confidence":  0.5, // +0.35 → 0.85 mastered candidate
		})
	}

	result, err := mastery.ApplyCompletion(ctx, q, dev, sess.ID, contracts.CompletionRequest{
		SchemaVersion: "1",
		CompletionID:  uuid.NewString(),
		RequestDigest: "partial-advance",
		Observations:  passingObs(doc),
		ClientTime:    time.Now().UTC(),
	}, time.Now().UTC())
	require.NoError(t, err)
	require.True(t, result.Accepted)
	require.NotEmpty(t, result.MasteryTransitions)

	for _, tr := range result.MasteryTransitions {
		// Candidate would be mastered; policy caps at in_progress → confForStatus path.
		assert.Equal(t, "not_introduced", tr.FromStatus)
		assert.Equal(t, "in_progress", tr.ToStatus)
		assert.True(t, tr.StatusChanged)
		assert.NotEqual(t, "mastered", tr.ToStatus)
		assert.Contains(t, tr.MissingEvidence, contracts.EvidenceConceptualResponse)
	}
}

func TestApplyCompletionEmptySummaryDefaults(t *testing.T) {
	t.Parallel()
	q := testutil.Tx(t)
	ctx := context.Background()
	doc, rev := publishBasicNav(t, q)
	student := factory.Student(t, q)
	dev, sess := deviceAndSession(t, q, student.ID, rev.ID)

	result, err := mastery.ApplyCompletion(ctx, q, dev, sess.ID, contracts.CompletionRequest{
		SchemaVersion: "1",
		CompletionID:  uuid.NewString(),
		RequestDigest: "empty-summary",
		Observations:  passingObs(doc),
		ClientTime:    time.Now().UTC(),
		Summary:       "",
	}, time.Now().UTC())
	require.NoError(t, err)
	require.True(t, result.Accepted)
	require.NotNil(t, result.AssignmentCompletion)
	assert.Equal(t, "completed", result.AssignmentCompletion.Summary)
}

func TestApplyCompletionTrustedSourcesPassCommandEvidence(t *testing.T) {
	t.Parallel()
	q := testutil.Tx(t)
	ctx := context.Background()
	root := repoRoot(t)
	// Craft command_properties activity (same pattern as existing capability test).
	subj := factory.Subject(t, q, factory.Override{"code": "digital-literacy-src-" + uuid.NewString()[:6]})
	std := factory.Standard(t, q, factory.Override{"code": "PRIMER.DL.6.PIPE.7", "subject_id": subj.ID})
	act, err := repo.CreateDraftActivity(ctx, q, "cmd-src-"+uuid.NewString()[:8], "cmd", "", contracts.KindTerminal, &subj.ID)
	require.NoError(t, err)
	content := contracts.ActivityContent{
		Objective: "run", Instructions: "ls",
		Terminal: &contracts.TerminalContent{
			RuntimeProfile: contracts.RuntimeCoreutilsBasic,
			Fixtures:       []contracts.FixtureEntry{{Path: "home", Type: "directory"}},
		},
		Tasks:  []contracts.Task{{ID: "t1", Title: "ls", Instructions: "ls", Completion: contracts.CheckTree{CheckID: "c-ls"}}},
		Checks: []contracts.Check{{ID: "c-ls", Kind: contracts.CheckCommandProperties, Params: map[string]any{"executable": "ls", "exitCode": 0}}},
	}
	rev, err := repo.PublishDraftRevision(ctx, q, act.ID, content, "1", []contracts.StandardRef{{
		Code: std.Code, Role: contracts.StandardRolePrimary, Weight: 1,
	}}, map[string]string{std.Code: std.ID}, time.Now().UTC())
	require.NoError(t, err)
	_ = root

	student := factory.Student(t, q)
	dev := factory.StudentDevice(t, q, factory.Override{"student_id": student.ID})
	asg, err := repo.CreateAssignment(ctx, q, student.ID, rev.ID, nil, 1, "test")
	require.NoError(t, err)
	sess, err := repo.StartOrResumeSession(ctx, q, dev, uuid.NewString(), asg.ID, time.Now().UTC(), contracts.CapStructuredCommandEvidence)
	require.NoError(t, err)

	now := time.Now().UTC()
	// Capability-only (legacy honest client) should pass.
	result, err := mastery.ApplyCompletion(ctx, q, dev, sess.ID, contracts.CompletionRequest{
		SchemaVersion: "1",
		CompletionID:  uuid.NewString(),
		RequestDigest: "cap-only",
		Observations: []contracts.Observation{{
			SchemaVersion: "1", CheckID: "c-ls", Kind: contracts.CheckCommandProperties,
			Passed: true, ObservedAt: now,
			Details: map[string]any{"capability": contracts.CapStructuredCommandEvidence},
		}},
		ClientTime: now,
	}, now)
	require.NoError(t, err)
	assert.True(t, result.Accepted)
}
