package mastery_test

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/curriculum"
	"github.com/aleksclark/primer/server/internal/domain"
	"github.com/aleksclark/primer/server/internal/mastery"
	"github.com/aleksclark/primer/server/internal/repo"
	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
	"github.com/aleksclark/primer/server/internal/testutil"
	"github.com/aleksclark/primer/server/internal/testutil/factory"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	return filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", ".."))
}

func publishBasicNav(t *testing.T, q repo.Querier) (*contracts.ActivityDocument, *domain.LearningActivityRevision) {
	t.Helper()
	ctx := context.Background()
	root := repoRoot(t)
	_, err := curriculum.Publish(ctx, q, curriculum.PublishOptions{
		StandardsDir: filepath.Join(root, "curriculum", "standards"),
		Now:          time.Now().UTC(),
	})
	require.NoError(t, err)
	doc, err := contracts.LoadDocument(filepath.Join(root, "curriculum", "activities", "basic-navigation", "activity.yaml"))
	require.NoError(t, err)
	_, rev, err := curriculum.PublishDocument(ctx, q, doc, time.Now().UTC())
	require.NoError(t, err)
	return doc, rev
}

func deviceAndSession(t *testing.T, q repo.Querier, studentID, revID string) (*domain.StudentDevice, *domain.LearningSession) {
	t.Helper()
	ctx := context.Background()
	dev := factory.StudentDevice(t, q, factory.Override{"student_id": studentID})
	asg, err := repo.CreateAssignment(ctx, q, studentID, revID, nil, 1, "test")
	require.NoError(t, err)
	sess, err := repo.StartOrResumeSession(ctx, q, dev, uuid.NewString(), asg.ID, time.Now().UTC())
	require.NoError(t, err)
	return dev, sess
}

func passingObs(doc *contracts.ActivityDocument) []contracts.Observation {
	now := time.Now().UTC()
	var out []contracts.Observation
	for _, c := range doc.Content.Checks {
		out = append(out, contracts.Observation{
			SchemaVersion: "1",
			CheckID:       c.ID,
			Kind:          c.Kind,
			Passed:        true,
			Optional:      c.Optional,
			ObservedAt:    now,
			Details: map[string]any{
				"structuredCommandEvidence": true,
				"capability":                contracts.CapStructuredCommandEvidence,
				"source":                    "structured",
			},
		})
	}
	return out
}

func TestCompletionIdempotentAssignmentAndMasteryDiffer(t *testing.T) {
	t.Parallel()
	q := testutil.Tx(t)
	ctx := context.Background()
	doc, rev := publishBasicNav(t, q)
	student := factory.Student(t, q)
	dev, sess := deviceAndSession(t, q, student.ID, rev.ID)

	req := contracts.CompletionRequest{
		SchemaVersion:  "1",
		CompletionID:   uuid.NewString(),
		RequestDigest:  "digest-1",
		Observations:   passingObs(doc),
		ClientTime:     time.Now().UTC(),
		Summary:        "done",
	}
	r1, err := mastery.ApplyCompletion(ctx, q, dev, sess.ID, req, time.Now().UTC())
	require.NoError(t, err)
	require.True(t, r1.Accepted)
	require.NotNil(t, r1.AssignmentCompletion)
	assert.Equal(t, domain.AssignmentCompleted, r1.AssignmentCompletion.State)
	require.NotEmpty(t, r1.MasteryTransitions)
	for _, tr := range r1.MasteryTransitions {
		assert.Equal(t, contracts.EvidenceProceduralContinuous, tr.EvidenceClass)
		// Default terminal policy: procedural alone may reach in_progress, not mastered.
		assert.NotEqual(t, "mastered", tr.ToStatus)
		assert.Contains(t, tr.AcceptedEvidence, contracts.EvidenceProceduralContinuous)
	}

	r2, err := mastery.ApplyCompletion(ctx, q, dev, sess.ID, req, time.Now().UTC())
	require.NoError(t, err)
	assert.Equal(t, r1.CompletionID, r2.CompletionID)
	assert.Equal(t, r1.EvidenceIDs, r2.EvidenceIDs)
	assert.Equal(t, r1.AssignmentCompletion.State, r2.AssignmentCompletion.State)
}

func TestProceduralCannotSatisfyConceptualPolicy(t *testing.T) {
	t.Parallel()
	q := testutil.Tx(t)
	ctx := context.Background()
	doc, rev := publishBasicNav(t, q)
	student := factory.Student(t, q)
	dev, sess := deviceAndSession(t, q, student.ID, rev.ID)

	// Force a stricter policy on links: approaching requires conceptual_response.
	links, err := repo.ListRevisionStandards(ctx, q, rev.ID)
	require.NoError(t, err)
	require.NotEmpty(t, links)
	strict := repo.EvidencePolicyMap(contracts.EvidencePolicy{
		Version: 1,
		StatusRequirements: map[string][]string{
			"in_progress": {contracts.EvidenceProceduralContinuous},
			"approaching": {contracts.EvidenceProceduralContinuous, contracts.EvidenceConceptualResponse},
			"mastered":    {contracts.EvidenceProceduralContinuous, contracts.EvidenceConceptualResponse, contracts.EvidenceFormalAssessment},
		},
	})
	for _, link := range links {
		_, err := q.Exec(ctx, `UPDATE learning_activity_revision_standards SET evidence_policy = $2 WHERE id = $1`, link.ID, strict)
		require.NoError(t, err)
	}

	// Seed high confidence so candidate would be approaching/mastered without policy.
	for _, link := range links {
		factory.MasteryRecord(t, q, factory.Override{
			"student_id":  student.ID,
			"standard_id": link.StandardID,
			"status":      "in_progress",
			"confidence":  0.5,
		})
	}

	req := contracts.CompletionRequest{
		SchemaVersion: "1",
		CompletionID:  uuid.NewString(),
		RequestDigest: "digest-conceptual",
		Observations:  passingObs(doc),
		ClientTime:    time.Now().UTC(),
	}
	result, err := mastery.ApplyCompletion(ctx, q, dev, sess.ID, req, time.Now().UTC())
	require.NoError(t, err)
	require.True(t, result.Accepted)
	require.NotNil(t, result.AssignmentCompletion)

	for _, tr := range result.MasteryTransitions {
		assert.Contains(t, tr.AcceptedEvidence, contracts.EvidenceProceduralContinuous)
		assert.NotContains(t, tr.ToStatus, "mastered")
		// Confidence candidate would exceed approaching threshold; policy must block it.
		assert.NotEqual(t, "approaching", tr.ToStatus)
		assert.NotEqual(t, "mastered", tr.ToStatus)
		assert.Contains(t, tr.MissingEvidence, contracts.EvidenceConceptualResponse)
	}

	// Evidence retained.
	page, err := repo.MasteryEvidences.List(ctx, q, repo.ListParams{Limit: 100})
	require.NoError(t, err)
	var n int
	for _, e := range page.Items {
		if e.EvidenceClass == contracts.EvidenceProceduralContinuous && e.SourceRef != "" {
			n++
		}
	}
	assert.Greater(t, n, 0)
}

func TestCancelledAssignmentNeverCreatesEvidence(t *testing.T) {
	t.Parallel()
	q := testutil.Tx(t)
	ctx := context.Background()
	doc, rev := publishBasicNav(t, q)
	student := factory.Student(t, q)
	dev := factory.StudentDevice(t, q, factory.Override{"student_id": student.ID})
	asg, err := repo.CreateAssignment(ctx, q, student.ID, rev.ID, nil, 1, "test")
	require.NoError(t, err)
	sess, err := repo.StartOrResumeSession(ctx, q, dev, uuid.NewString(), asg.ID, time.Now().UTC())
	require.NoError(t, err)
	_, err = repo.CancelAssignment(ctx, q, asg.ID)
	require.NoError(t, err)

	_, err = mastery.ApplyCompletion(ctx, q, dev, sess.ID, contracts.CompletionRequest{
		SchemaVersion: "1",
		CompletionID:  uuid.NewString(),
		RequestDigest: "x",
		Observations:  passingObs(doc),
		ClientTime:    time.Now().UTC(),
	}, time.Now().UTC())
	require.Error(t, err)

	page, err := repo.MasteryEvidences.List(ctx, q, repo.ListParams{Limit: 50})
	require.NoError(t, err)
	for _, e := range page.Items {
		assert.NotContains(t, e.SourceRef, sess.ID)
	}
}

func TestTypingCompletionAffectsOnlyTypingStandards(t *testing.T) {
	t.Parallel()
	q := testutil.Tx(t)
	ctx := context.Background()
	root := repoRoot(t)
	_, err := curriculum.Publish(ctx, q, curriculum.PublishOptions{
		StandardsDir:  filepath.Join(root, "curriculum", "standards"),
		ActivitiesDir: filepath.Join(root, "curriculum", "activities"),
		Now:           time.Now().UTC(),
	})
	require.NoError(t, err)

	doc, err := contracts.LoadDocument(filepath.Join(root, "curriculum", "activities", "command-typing-basics", "activity.yaml"))
	require.NoError(t, err)
	_, rev, err := curriculum.PublishDocument(ctx, q, doc, time.Now().UTC())
	require.NoError(t, err)

	student := factory.Student(t, q)
	// Seed NAV mastery
	stdPage, err := repo.Standards.List(ctx, q, repo.ListParams{Limit: 200, Search: "PRIMER.DL.6.NAV.1"})
	require.NoError(t, err)
	var navID string
	for _, s := range stdPage.Items {
		if s.Code == "PRIMER.DL.6.NAV.1" {
			navID = s.ID
			break
		}
	}
	require.NotEmpty(t, navID)
	navRec := factory.MasteryRecord(t, q, factory.Override{
		"student_id": student.ID, "standard_id": navID, "status": "in_progress", "confidence": 0.4,
	})

	dev, sess := deviceAndSession(t, q, student.ID, rev.ID)
	result, err := mastery.ApplyCompletion(ctx, q, dev, sess.ID, contracts.CompletionRequest{
		SchemaVersion: "1",
		CompletionID:  uuid.NewString(),
		RequestDigest: "typing-1",
		Observations:  passingObs(doc),
		ClientTime:    time.Now().UTC(),
	}, time.Now().UTC())
	require.NoError(t, err)
	for _, tr := range result.MasteryTransitions {
		assert.Contains(t, tr.StandardCode, ".TYPE.")
	}
	got, err := repo.MasteryRecords.Get(ctx, q, navRec.ID)
	require.NoError(t, err)
	assert.Equal(t, 0.4, got.Confidence)
	assert.Equal(t, "in_progress", got.Status)
}

func TestRunnerWithoutCapabilityRejected(t *testing.T) {
	t.Parallel()
	q := testutil.Tx(t)
	ctx := context.Background()
	root := repoRoot(t)
	_, err := curriculum.Publish(ctx, q, curriculum.PublishOptions{
		StandardsDir: filepath.Join(root, "curriculum", "standards"),
		Now:          time.Now().UTC(),
	})
	require.NoError(t, err)

	// Craft a revision whose required completion is only command_properties.
	subj := factory.Subject(t, q, factory.Override{"code": "digital-literacy-cap"})
	std := factory.Standard(t, q, factory.Override{"code": "PRIMER.DL.6.PIPE.9", "subject_id": subj.ID})
	act, err := repo.CreateDraftActivity(ctx, q, "cmd-only-"+uuid.NewString()[:8], "cmd only", "", contracts.KindTerminal, &subj.ID)
	require.NoError(t, err)
	content := contracts.ActivityContent{
		Objective:    "run a command",
		Instructions: "run ls",
		Terminal: &contracts.TerminalContent{
			RuntimeProfile: contracts.RuntimeCoreutilsBasic,
			Fixtures:       []contracts.FixtureEntry{{Path: "home", Type: "directory"}},
		},
		Tasks: []contracts.Task{{
			ID: "t1", Title: "ls", Instructions: "ls",
			Completion: contracts.CheckTree{CheckID: "c-ls"},
		}},
		Checks: []contracts.Check{{
			ID: "c-ls", Kind: contracts.CheckCommandProperties,
			Params: map[string]any{"executable": "ls", "exitCode": 0},
		}},
	}
	rev, err := repo.PublishDraftRevision(ctx, q, act.ID, content, "1", []contracts.StandardRef{{
		Code: std.Code, Role: contracts.StandardRolePrimary, Weight: 1,
	}}, map[string]string{std.Code: std.ID}, time.Now().UTC())
	require.NoError(t, err)

	student := factory.Student(t, q)
	dev := factory.StudentDevice(t, q, factory.Override{"student_id": student.ID})
	asg, err := repo.CreateAssignment(ctx, q, student.ID, rev.ID, nil, 1, "test")
	require.NoError(t, err)

	_, err = repo.StartOrResumeSession(ctx, q, dev, uuid.NewString(), asg.ID, time.Now().UTC())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "structured_command_evidence")

	// With capability, session starts.
	sess, err := repo.StartOrResumeSession(ctx, q, dev, uuid.NewString(), asg.ID, time.Now().UTC(), contracts.CapStructuredCommandEvidence)
	require.NoError(t, err)
	require.NotEmpty(t, sess.ID)
}

func TestEvidenceMigrationFieldsPresent(t *testing.T) {
	t.Parallel()
	q := testutil.Tx(t)
	ctx := context.Background()
	doc, rev := publishBasicNav(t, q)
	student := factory.Student(t, q)
	dev, sess := deviceAndSession(t, q, student.ID, rev.ID)
	result, err := mastery.ApplyCompletion(ctx, q, dev, sess.ID, contracts.CompletionRequest{
		SchemaVersion: "1",
		CompletionID:  uuid.NewString(),
		RequestDigest: "mig",
		Observations:  passingObs(doc),
		ClientTime:    time.Now().UTC(),
	}, time.Now().UTC())
	require.NoError(t, err)
	require.NotEmpty(t, result.EvidenceIDs)

	ev, err := repo.MasteryEvidences.Get(ctx, q, result.EvidenceIDs[0])
	require.NoError(t, err)
	assert.Equal(t, contracts.EvidenceProceduralContinuous, ev.EvidenceClass)
	assert.NotEmpty(t, ev.Provenance)
	assert.Equal(t, 1, ev.PolicyVersion)
}

func TestParentOverviewEvidenceStatusAfterProceduralCompletion(t *testing.T) {
	t.Parallel()
	q := testutil.Tx(t)
	ctx := context.Background()
	doc, rev := publishBasicNav(t, q)
	student := factory.Student(t, q)
	dev, sess := deviceAndSession(t, q, student.ID, rev.ID)

	_, err := mastery.ApplyCompletion(ctx, q, dev, sess.ID, contracts.CompletionRequest{
		SchemaVersion: "1",
		CompletionID:  uuid.NewString(),
		RequestDigest: "parent-ev",
		Observations:  passingObs(doc),
		ClientTime:    time.Now().UTC(),
	}, time.Now().UTC())
	require.NoError(t, err)

	ov, err := repo.GetStudentLearningOverview(ctx, q, student.ID, 10)
	require.NoError(t, err)
	require.NotEmpty(t, ov.EvidenceStatuses)

	var sawProcedural bool
	for _, st := range ov.EvidenceStatuses {
		assert.NotEqual(t, "mastered", st.MasteryStatus, "procedural alone must not master %s", st.StandardCode)
		assert.False(t, st.FormalMastery)
		if st.ProceduralAccepted {
			sawProcedural = true
			assert.Contains(t, st.AcceptedEvidenceClasses, contracts.EvidenceProceduralContinuous)
			assert.True(t, st.AdditionalEvidenceRequired || len(st.MissingEvidenceClasses) > 0)
			assert.Contains(t, []string{
				repo.EvidenceStatusProceduralAccepted,
				repo.EvidenceStatusAdditionalEvidenceReq,
			}, st.EvidenceStatus)
			assert.Contains(t, st.MissingEvidenceClasses, contracts.EvidenceConceptualResponse)
		}
	}
	assert.True(t, sawProcedural, "expected at least one procedural evidence status")
}
