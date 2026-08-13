package api_test

import (
	"context"
	"net/http"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/curriculum"
	"github.com/aleksclark/primer/server/internal/domain"
	"github.com/aleksclark/primer/server/internal/repo"
	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
	"github.com/aleksclark/primer/server/internal/testutil"
	"github.com/aleksclark/primer/server/internal/testutil/factory"
)

func TestConceptualResponseSubmitAndParentReview(t *testing.T) {
	t.Parallel()
	h, q := testutil.API(t)
	ctx := context.Background()

	const password = "test-password-ok"
	ed := factory.EducatorWithPassword(t, q, password, factory.Override{
		"email": "response-review@example.com",
		"role":  "parent",
	})
	student := factory.Student(t, q, factory.Override{"first_name": "Resp", "last_name": "Student"})

	resp := h.Post("/auth/login", objMap{"email": ed.Email, "password": password})
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	parentAuth := "Authorization: Bearer " + decode[objMap](t, resp.Body.Bytes())["token"].(string)

	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", ".."))
	_, err := curriculum.Publish(ctx, q, curriculum.PublishOptions{
		StandardsDir: filepath.Join(repoRoot, "curriculum", "standards"),
		Now:          time.Now().UTC(),
	})
	require.NoError(t, err)

	doc := &contracts.ActivityDocument{
		SchemaVersion: contracts.SchemaVersion,
		Slug:          "response-review-sample",
		Title:         "Response Review Sample",
		Summary:       "short response",
		Kind:          contracts.KindTerminal,
		SubjectCode:   "digital-literacy",
		Standards: []contracts.StandardRef{
			{Code: "PRIMER.DL.6.NAV.1", Role: contracts.StandardRolePrimary, Weight: 1},
		},
		Content: contracts.ActivityContent{
			Objective:    "Explain a concept",
			Instructions: "Read and respond",
			Blocks: []contracts.InstructionBlock{
				{ID: "intro", Kind: contracts.BlockProse, Text: "The terminal displays text."},
				{ID: "secret", Kind: contracts.BlockParentNote, Text: "Ask orally about whoami."},
			},
			Terminal: &contracts.TerminalContent{
				RuntimeProfile: contracts.RuntimeCoreutilsBasic,
				Fixtures:       []contracts.FixtureEntry{{Path: "workspace", Type: contracts.FixtureDirectory}},
				InitialCwd:     "workspace",
			},
			Tasks: []contracts.Task{
				{
					ID:           "explain",
					Title:        "Explain",
					Instructions: "Write your answer",
					Kind:         contracts.TaskKindShortResponse,
					Completion:   contracts.CheckTree{CheckID: "resp-done"},
					Response: &contracts.ResponseTaskSpec{
						Prompt:               "How do terminal and shell differ?",
						MaxChars:             500,
						ParentReviewRequired: true,
						Rubric: []contracts.RubricCriterion{
							{ID: "roles", Description: "Names distinct roles", Required: true},
						},
					},
				},
			},
			Checks: []contracts.Check{
				{
					ID:     "resp-done",
					Kind:   contracts.CheckResponseSubmitted,
					Params: map[string]any{"taskId": "explain"},
					Stages: []string{contracts.StageFinal},
				},
			},
		},
	}
	require.NoError(t, contracts.ValidateDocument(doc))
	_, rev, err := curriculum.PublishDocument(ctx, q, doc, time.Now().UTC())
	require.NoError(t, err)

	resp = h.Post("/assignments", objMap{
		"studentId":          student.ID,
		"activityRevisionId": rev.ID,
	}, parentAuth)
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())
	asgID := decode[objMap](t, resp.Body.Bytes())["id"].(string)

	resp = h.Post("/pairing-codes", objMap{"studentId": student.ID}, parentAuth)
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())
	code := decode[objMap](t, resp.Body.Bytes())["code"].(string)

	resp = h.Post("/student-devices/pair", objMap{"code": code, "deviceName": "ws"})
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())
	deviceToken := decode[objMap](t, resp.Body.Bytes())["token"].(string)
	devAuth := "Authorization: Bearer " + deviceToken

	clientSessionID := uuid.NewString()
	resp = h.Post("/student/sessions", objMap{
		"clientSessionId": clientSessionID,
		"assignmentId":    asgID,
		"capabilities":    []string{contracts.CapStructuredCommandEvidence},
	}, devAuth)
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())
	sessionID := decode[objMap](t, resp.Body.Bytes())["id"].(string)

	submissionID := uuid.NewString()
	body := "The terminal shows text; the shell interprets commands. The prompt is not typed."
	resp = h.Post("/student/sessions/"+sessionID+"/responses", objMap{
		"schemaVersion": "1",
		"submissionId":  submissionID,
		"taskId":        "explain",
		"body":          body,
		"clientTime":    time.Now().UTC().Format(time.RFC3339Nano),
	}, devAuth)
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())
	submit1 := decode[objMap](t, resp.Body.Bytes())
	responseID := submit1["responseId"].(string)
	require.NotEmpty(t, responseID)
	assert.Equal(t, domain.ResponseSubmitted, submit1["status"])
	assert.Equal(t, true, submit1["reviewRequired"])

	// Idempotent retry
	resp = h.Post("/student/sessions/"+sessionID+"/responses", objMap{
		"schemaVersion": "1",
		"submissionId":  submissionID,
		"taskId":        "explain",
		"body":          body,
		"clientTime":    time.Now().UTC().Format(time.RFC3339Nano),
	}, devAuth)
	require.True(t, resp.Code == http.StatusOK || resp.Code == http.StatusCreated, resp.Body.String())
	assert.Equal(t, responseID, decode[objMap](t, resp.Body.Bytes())["responseId"])

	// Tutor-looking text rejected
	resp = h.Post("/student/sessions/"+sessionID+"/responses", objMap{
		"schemaVersion": "1",
		"submissionId":  uuid.NewString(),
		"taskId":        "explain",
		"body":          "As your tutor I recommend saying the terminal displays...",
		"clientTime":    time.Now().UTC().Format(time.RFC3339Nano),
	}, devAuth)
	require.Equal(t, http.StatusBadRequest, resp.Code, resp.Body.String())

	// Student work queue must not expose parent_note blocks.
	resp = h.Get("/student/work", devAuth)
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	workBody := decode[objMap](t, resp.Body.Bytes())
	workItems, _ := workBody["items"].([]any)
	require.NotEmpty(t, workItems)
	foundParentNoteOnWire := false
	for _, rawItem := range workItems {
		item, _ := rawItem.(map[string]any)
		rev, _ := item["revision"].(map[string]any)
		content, _ := rev["content"].(map[string]any)
		blocks, _ := content["blocks"].([]any)
		for _, rawBlock := range blocks {
			b, _ := rawBlock.(map[string]any)
			if b["kind"] == contracts.BlockParentNote {
				foundParentNoteOnWire = true
			}
		}
	}
	assert.False(t, foundParentNoteOnWire, "parent_note must not be delivered on student work")

	// Parent list/detail
	resp = h.Get("/response-reviews", parentAuth)
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	listBody := decode[objMap](t, resp.Body.Bytes())
	items := listBody["items"].([]any)
	require.NotEmpty(t, items)

	resp = h.Get("/response-reviews/"+responseID, parentAuth)
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	detail := decode[objMap](t, resp.Body.Bytes())
	assert.Equal(t, body, detail["response"].(map[string]any)["body"])
	// Parent notes present for parent; student blocks exclude them
	parentNotes, _ := detail["parentNotes"].([]any)
	require.NotEmpty(t, parentNotes)
	studentBlocks, _ := detail["studentBlocks"].([]any)
	for _, b := range studentBlocks {
		assert.NotEqual(t, contracts.BlockParentNote, b.(map[string]any)["kind"])
	}

	// Accept → parent_attestation
	resp = h.Post("/response-reviews/"+responseID+"/decision", objMap{
		"decision": "accept",
		"criteria": []objMap{{"id": "roles", "accepted": true}},
	}, parentAuth)
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	dec := decode[objMap](t, resp.Body.Bytes())
	assert.Equal(t, domain.ResponseAccepted, dec["response"].(map[string]any)["status"])
	evIDs, _ := dec["evidenceIds"].([]any)
	require.NotEmpty(t, evIDs)

	// Idempotent accept
	resp = h.Post("/response-reviews/"+responseID+"/decision", objMap{"decision": "accept"}, parentAuth)
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())

	// Evidence classes include conceptual + parent attestation
	page, err := repo.MasteryEvidences.List(ctx, q, repo.ListParams{Limit: 100})
	require.NoError(t, err)
	var classes []string
	for _, ev := range page.Items {
		classes = append(classes, ev.EvidenceClass)
	}
	assert.Contains(t, classes, domain.EvidenceConceptualResponse)
	assert.Contains(t, classes, domain.EvidenceParentAttestation)

	// Learning overview surfaces evidence statuses
	resp = h.Get("/students/"+student.ID+"/learning-overview", parentAuth)
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	ov := decode[objMap](t, resp.Body.Bytes())
	_, hasEV := ov["evidenceStatuses"]
	assert.True(t, hasEV)
}
