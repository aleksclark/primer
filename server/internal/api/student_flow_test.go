package api_test

import (
	"context"
	"net/http"
	"path/filepath"
	"runtime"
	"strings"
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

func TestStudentWorkEvidenceFlow(t *testing.T) {
	t.Parallel()
	h, q := testutil.API(t)
	ctx := context.Background()

	const password = "test-password-ok"
	ed := factory.EducatorWithPassword(t, q, password, factory.Override{
		"email": "parent-flow@example.com",
		"role":  "parent",
	})
	student := factory.Student(t, q, factory.Override{"first_name": "Flow", "last_name": "Student"})
	other := factory.Student(t, q, factory.Override{"first_name": "Other", "last_name": "Kid"})

	// 1. Parent login
	resp := h.Post("/auth/login", objMap{
		"email":    ed.Email,
		"password": password,
	})
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	login := decode[objMap](t, resp.Body.Bytes())
	parentToken := login["token"].(string)
	require.NotEmpty(t, parentToken)

	// 2. Publish sample activity
	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", ".."))
	doc, err := contracts.LoadDocument(filepath.Join(repoRoot, "curriculum", "activities", "basic-navigation", "activity.yaml"))
	require.NoError(t, err)
	// Ensure standards exist
	_, err = curriculum.Publish(ctx, q, curriculum.PublishOptions{
		StandardsDir: filepath.Join(repoRoot, "curriculum", "standards"),
		Now:          time.Now().UTC(),
	})
	require.NoError(t, err)
	_, rev, err := curriculum.PublishDocument(ctx, q, doc, time.Now().UTC())
	require.NoError(t, err)
	require.NotEmpty(t, rev.ID)

	parentAuth := "Authorization: Bearer " + parentToken

	// 3. Assign work (parent session)
	resp = h.Post("/assignments", objMap{
		"studentId":          student.ID,
		"activityRevisionId": rev.ID,
		"priority":           10,
	}, parentAuth)
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())
	asg := decode[objMap](t, resp.Body.Bytes())
	assignmentID := asg["id"].(string)

	// 4. Pairing code + device pair
	resp = h.Post("/pairing-codes", objMap{"studentId": student.ID}, parentAuth)
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())
	pc := decode[objMap](t, resp.Body.Bytes())
	code := pc["code"].(string)
	require.NotEmpty(t, code)

	resp = h.Post("/student-devices/pair", objMap{
		"code":       code,
		"deviceName": "test-ws",
	})
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())
	pair := decode[objMap](t, resp.Body.Bytes())
	deviceToken := pair["token"].(string)
	require.NotEmpty(t, deviceToken)
	deviceAuth := "Authorization: Bearer " + deviceToken

	// Second student device for isolation check
	resp = h.Post("/pairing-codes", objMap{"studentId": other.ID}, parentAuth)
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())
	code2 := decode[objMap](t, resp.Body.Bytes())["code"].(string)
	resp = h.Post("/student-devices/pair", objMap{"code": code2, "deviceName": "other-ws"})
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())
	otherAuth := "Authorization: Bearer " + decode[objMap](t, resp.Body.Bytes())["token"].(string)

	// 5. GET work returns assignment
	resp = h.Get("/student/work", deviceAuth)
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	work := decode[struct {
		Items []struct {
			Assignment domain.StudentAssignment `json:"assignment"`
			Activity   domain.LearningActivity  `json:"activity"`
		} `json:"items"`
	}](t, resp.Body.Bytes())
	require.NotEmpty(t, work.Items)
	found := false
	for _, it := range work.Items {
		if it.Assignment.ID == assignmentID {
			found = true
			assert.Equal(t, "basic-navigation", it.Activity.Slug)
		}
	}
	require.True(t, found, "assignment missing from work queue")

	// Profile
	resp = h.Get("/student/profile", deviceAuth)
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())

	// 6. Start session twice with same clientSessionId -> same session
	clientSessionID := uuid.NewString()
	resp = h.Post("/student/sessions", objMap{
		"clientSessionId": clientSessionID,
		"assignmentId":    assignmentID,
	}, deviceAuth)
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())
	sess1 := decode[objMap](t, resp.Body.Bytes())
	sessionID := sess1["id"].(string)

	resp = h.Post("/student/sessions", objMap{
		"clientSessionId": clientSessionID,
		"assignmentId":    assignmentID,
	}, deviceAuth)
	require.True(t, resp.Code == http.StatusCreated || resp.Code == http.StatusOK, resp.Body.String())
	sess2 := decode[objMap](t, resp.Body.Bytes())
	assert.Equal(t, sessionID, sess2["id"])

	// 7. Other device cannot access session
	resp = h.Post("/student/sessions/"+sessionID+"/events", objMap{
		"events": []objMap{},
	}, otherAuth)
	assert.Equal(t, http.StatusNotFound, resp.Code, resp.Body.String())

	// 8. Post event batch twice -> no dupes
	eventID := uuid.NewString()
	eventsBody := objMap{
		"events": []objMap{
			{
				"schemaVersion": "1",
				"eventId":       eventID,
				"type":          contracts.EventSessionStarted,
				"sequence":      0,
				"clientTime":    time.Now().UTC().Format(time.RFC3339Nano),
				"payload":       objMap{},
			},
			{
				"schemaVersion": "1",
				"eventId":       uuid.NewString(),
				"type":          contracts.EventTaskViewed,
				"sequence":      1,
				"clientTime":    time.Now().UTC().Format(time.RFC3339Nano),
				"payload":       objMap{"taskId": "orient"},
			},
		},
	}
	resp = h.Post("/student/sessions/"+sessionID+"/events", eventsBody, deviceAuth)
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	ack1 := decode[objMap](t, resp.Body.Bytes())
	assert.Equal(t, float64(1), ack1["acknowledgedSequence"])

	resp = h.Post("/student/sessions/"+sessionID+"/events", eventsBody, deviceAuth)
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	n, err := repo.CountSessionEvents(ctx, q, sessionID)
	require.NoError(t, err)
	assert.Equal(t, 2, n, "retry must not duplicate events")

	// Artifact metadata
	artID := uuid.NewString()
	resp = h.Post("/student/sessions/"+sessionID+"/artifacts", objMap{
		"schemaVersion": "1",
		"artifactId":    artID,
		"filename":      "note.txt",
		"mediaType":     "text/plain",
		"byteSize":      12,
		"sha256":        "aabbcc",
		"createdAt":     time.Now().UTC().Format(time.RFC3339Nano),
	}, deviceAuth)
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())

	// 9. Complete twice with same completionId -> same mastery once
	obs := passingObservations(doc)
	completionID := uuid.NewString()
	digest := "digest-test-1"
	completeBody := objMap{
		"schemaVersion": "1",
		"completionId":  completionID,
		"requestDigest": digest,
		"observations":  obs,
		"clientTime":    time.Now().UTC().Format(time.RFC3339Nano),
		"summary":       "done",
	}
	resp = h.Post("/student/sessions/"+sessionID+"/complete", completeBody, deviceAuth)
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	result1 := decode[contracts.CompletionResult](t, resp.Body.Bytes())
	require.True(t, result1.Accepted)
	require.NotEmpty(t, result1.EvidenceIDs)
	require.NotEmpty(t, result1.MasterySnapshot)

	resp = h.Post("/student/sessions/"+sessionID+"/complete", completeBody, deviceAuth)
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	result2 := decode[contracts.CompletionResult](t, resp.Body.Bytes())
	assert.Equal(t, result1.CompletionID, result2.CompletionID)
	assert.Equal(t, result1.EvidenceIDs, result2.EvidenceIDs)

	// Evidence count stable
	page, err := repo.MasteryEvidences.List(ctx, q, repo.ListParams{Limit: 100})
	require.NoError(t, err)
	var evidenceForSession int
	for _, e := range page.Items {
		if e.SourceRef != "" && contains(e.SourceRef, sessionID) {
			evidenceForSession++
		}
	}
	assert.Equal(t, len(result1.EvidenceIDs), evidenceForSession)

	// Mastery records exist for linked standards
	mrPage, err := repo.MasteryRecords.List(ctx, q, repo.ListParams{
		Limit:   50,
		Filters: map[string]any{"student_id": student.ID},
	})
	require.NoError(t, err)
	require.NotEmpty(t, mrPage.Items)
	for _, mr := range mrPage.Items {
		assert.Greater(t, mr.Confidence, 0.0)
		assert.NotEqual(t, "not_introduced", mr.Status)
	}
}

func TestParentLoginRejectsBadPassword(t *testing.T) {
	t.Parallel()
	h, q := testutil.API(t)
	ed := factory.EducatorWithPassword(t, q, "secret", factory.Override{"email": "badlogin@example.com"})
	resp := h.Post("/auth/login", objMap{"email": ed.Email, "password": "nope"})
	assert.Equal(t, http.StatusUnauthorized, resp.Code)
}

func passingObservations(doc *contracts.ActivityDocument) []objMap {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	var out []objMap
	for _, c := range doc.Content.Checks {
		out = append(out, objMap{
			"schemaVersion": "1",
			"checkId":       c.ID,
			"kind":          c.Kind,
			"passed":        true,
			"optional":      c.Optional,
			"observedAt":    now,
		})
	}
	return out
}

func contains(s, sub string) bool {
	return strings.Contains(s, sub)
}
