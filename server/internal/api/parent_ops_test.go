package api_test

import (
	"context"
	"net/http"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/curriculum"
	"github.com/aleksclark/primer/server/internal/domain"
	"github.com/aleksclark/primer/server/internal/repo"
	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
	"github.com/aleksclark/primer/server/internal/testutil"
	"github.com/aleksclark/primer/server/internal/testutil/factory"
)

func TestCancelAndRetryAssignment(t *testing.T) {
	t.Parallel()
	h, q := testutil.API(t)
	ctx := context.Background()

	const password = "test-password-ok"
	ed := factory.EducatorWithPassword(t, q, password, factory.Override{
		"email": "cancel-retry@example.com",
		"role":  "parent",
	})
	student := factory.Student(t, q, factory.Override{"first_name": "Can", "last_name": "Cel"})

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
	doc, err := contracts.LoadDocument(filepath.Join(repoRoot, "curriculum", "activities", "basic-navigation", "activity.yaml"))
	require.NoError(t, err)
	_, rev, err := curriculum.PublishDocument(ctx, q, doc, time.Now().UTC())
	require.NoError(t, err)

	resp = h.Post("/assignments", objMap{
		"studentId":          student.ID,
		"activityRevisionId": rev.ID,
		"priority":           5,
	}, parentAuth)
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())
	asgID := decode[objMap](t, resp.Body.Bytes())["id"].(string)

	resp = h.Post("/assignments/"+asgID+"/cancel", objMap{}, parentAuth)
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	cancelled := decode[domain.StudentAssignment](t, resp.Body.Bytes())
	assert.Equal(t, domain.AssignmentCancelled, cancelled.State)

	// Idempotent cancel
	resp = h.Post("/assignments/"+asgID+"/cancel", objMap{}, parentAuth)
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())

	resp = h.Post("/assignments/"+asgID+"/retry", objMap{"reason": "try again"}, parentAuth)
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())
	retry := decode[domain.StudentAssignment](t, resp.Body.Bytes())
	assert.NotEqual(t, asgID, retry.ID)
	assert.Equal(t, student.ID, retry.StudentID)
	assert.Equal(t, rev.ID, retry.ActivityRevisionID)
	assert.Equal(t, domain.AssignmentAvailable, retry.State)
	assert.Equal(t, "try again", retry.Reason)
}

func TestLearningOverviewAndMetrics(t *testing.T) {
	t.Parallel()
	h, q := testutil.API(t)
	ctx := context.Background()

	const password = "test-password-ok"
	ed := factory.EducatorWithPassword(t, q, password, factory.Override{
		"email": "overview-metrics@example.com",
		"role":  "parent",
	})
	student := factory.Student(t, q, factory.Override{"first_name": "Over", "last_name": "View"})

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
	doc, err := contracts.LoadDocument(filepath.Join(repoRoot, "curriculum", "activities", "basic-navigation", "activity.yaml"))
	require.NoError(t, err)
	_, rev, err := curriculum.PublishDocument(ctx, q, doc, time.Now().UTC())
	require.NoError(t, err)

	resp = h.Post("/assignments", objMap{
		"studentId":          student.ID,
		"activityRevisionId": rev.ID,
	}, parentAuth)
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())

	resp = h.Post("/pairing-codes", objMap{"studentId": student.ID}, parentAuth)
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())
	code := decode[objMap](t, resp.Body.Bytes())["code"].(string)
	resp = h.Post("/student-devices/pair", objMap{"code": code, "deviceName": "overview-ws"})
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())
	deviceID := decode[objMap](t, resp.Body.Bytes())["deviceId"].(string)

	resp = h.Get("/students/"+student.ID+"/learning-overview", parentAuth)
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	ov := decode[struct {
		Student         domain.Student             `json:"student"`
		Devices         []domain.StudentDevice     `json:"devices"`
		OpenAssignments []domain.StudentAssignment `json:"openAssignments"`
		Tutor           struct {
			Enabled             bool   `json:"enabled"`
			Provider            string `json:"provider"`
			StudentNotesDisable bool   `json:"studentNotesDisable"`
		} `json:"tutor"`
		TutorNotesDisable bool `json:"tutorNotesDisable"`
	}](t, resp.Body.Bytes())
	assert.Equal(t, student.ID, ov.Student.ID)
	require.Len(t, ov.Devices, 1)
	assert.Equal(t, deviceID, ov.Devices[0].ID)
	require.NotEmpty(t, ov.OpenAssignments)
	assert.False(t, ov.TutorNotesDisable)
	assert.NotEmpty(t, ov.Tutor.Provider)

	resp = h.Get("/ops/student-metrics", parentAuth)
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	m := decode[struct {
		DevicesActive      int `json:"devicesActive"`
		DevicesRevoked     int `json:"devicesRevoked"`
		AssignmentsOpen    int `json:"assignmentsOpen"`
		SessionsActive     int `json:"sessionsActive"`
		CompletionsLast24h int `json:"completionsLast24h"`
	}](t, resp.Body.Bytes())
	assert.GreaterOrEqual(t, m.DevicesActive, 1)
	assert.GreaterOrEqual(t, m.AssignmentsOpen, 1)
	assert.GreaterOrEqual(t, m.DevicesRevoked, 0)
}

func TestTutorToggleAndRotateDevice(t *testing.T) {
	t.Parallel()
	h, q := testutil.API(t)

	const password = "test-password-ok"
	ed := factory.EducatorWithPassword(t, q, password, factory.Override{
		"email": "tutor-rotate@example.com",
		"role":  "parent",
	})
	student := factory.Student(t, q, factory.Override{"first_name": "Tut", "last_name": "Or"})

	resp := h.Post("/auth/login", objMap{"email": ed.Email, "password": password})
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	parentAuth := "Authorization: Bearer " + decode[objMap](t, resp.Body.Bytes())["token"].(string)

	resp = h.Post("/students/"+student.ID+"/tutor", objMap{"enabled": false}, parentAuth)
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	st := decode[domain.Student](t, resp.Body.Bytes())
	assert.Contains(t, strings.ToLower(st.Notes), "tutor:off")

	resp = h.Get("/students/"+student.ID+"/tutor-status", parentAuth)
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	status := decode[objMap](t, resp.Body.Bytes())
	assert.Equal(t, true, status["studentNotesDisable"])
	assert.Equal(t, false, status["enabled"])

	resp = h.Post("/students/"+student.ID+"/tutor", objMap{"enabled": true}, parentAuth)
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	st = decode[domain.Student](t, resp.Body.Bytes())
	assert.NotContains(t, strings.ToLower(st.Notes), "tutor:off")

	// Pair then rotate
	resp = h.Post("/pairing-codes", objMap{"studentId": student.ID}, parentAuth)
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())
	code := decode[objMap](t, resp.Body.Bytes())["code"].(string)
	resp = h.Post("/student-devices/pair", objMap{"code": code, "deviceName": "rotate-ws"})
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())
	pair := decode[objMap](t, resp.Body.Bytes())
	deviceID := pair["deviceId"].(string)
	deviceToken := pair["token"].(string)

	resp = h.Post("/student-devices/"+deviceID+"/rotate-token", objMap{}, parentAuth)
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	rot := decode[struct {
		Device domain.StudentDevice `json:"device"`
		Code   string               `json:"code"`
	}](t, resp.Body.Bytes())
	require.NotNil(t, rot.Device.RevokedAt)
	require.NotEmpty(t, rot.Code)

	// Old token dead
	resp = h.Get("/student/profile", "Authorization: Bearer "+deviceToken)
	require.Equal(t, http.StatusUnauthorized, resp.Code)

	// New code pairs a fresh device
	resp = h.Post("/student-devices/pair", objMap{"code": rot.Code, "deviceName": "rotate-ws-2"})
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())
}

func TestSupersedeMasteryEvidence(t *testing.T) {
	t.Parallel()
	h, q := testutil.API(t)
	ctx := context.Background()

	const password = "test-password-ok"
	ed := factory.EducatorWithPassword(t, q, password, factory.Override{
		"email": "supersede@example.com",
		"role":  "parent",
	})
	student := factory.Student(t, q, factory.Override{"first_name": "Super", "last_name": "Sede"})
	std := factory.Standard(t, q)

	resp := h.Post("/auth/login", objMap{"email": ed.Email, "password": password})
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	parentAuth := "Authorization: Bearer " + decode[objMap](t, resp.Body.Bytes())["token"].(string)

	rec, err := repo.MasteryRecords.Create(ctx, q, map[string]any{
		"student_id":  student.ID,
		"standard_id": std.ID,
		"status":      "in_progress",
		"confidence":  0.4,
	})
	require.NoError(t, err)

	ev, err := repo.MasteryEvidences.Create(ctx, q, map[string]any{
		"mastery_record_id": rec.ID,
		"kind":              "continuous",
		"occurred_on":       time.Now().UTC(),
		"context":           "session completion",
		"source_ref":        "session:test-1",
	})
	require.NoError(t, err)

	resp = h.Post("/mastery-evidence/"+ev.ID+"/supersede", objMap{
		"note": "parent observed incomplete work",
	}, parentAuth)
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	body := decode[struct {
		Original    domain.MasteryEvidence `json:"original"`
		Replacement domain.MasteryEvidence `json:"replacement"`
	}](t, resp.Body.Bytes())
	assert.Contains(t, strings.ToLower(body.Original.Context), "superseded by parent")
	assert.Equal(t, rec.ID, body.Replacement.MasteryRecordID)
	assert.Contains(t, body.Replacement.Context, "parent observed incomplete work")
	assert.NotEqual(t, body.Original.ID, body.Replacement.ID)

	// Second supersede of same row fails
	resp = h.Post("/mastery-evidence/"+ev.ID+"/supersede", objMap{"note": "again"}, parentAuth)
	require.Equal(t, http.StatusBadRequest, resp.Code, resp.Body.String())
}

func TestListAssignmentsAndSessionsParent(t *testing.T) {
	t.Parallel()
	h, q := testutil.API(t)
	ctx := context.Background()

	const password = "test-password-ok"
	ed := factory.EducatorWithPassword(t, q, password, factory.Override{
		"email": "list-asg@example.com",
		"role":  "parent",
	})
	student := factory.Student(t, q)

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
	doc, err := contracts.LoadDocument(filepath.Join(repoRoot, "curriculum", "activities", "basic-navigation", "activity.yaml"))
	require.NoError(t, err)
	_, rev, err := curriculum.PublishDocument(ctx, q, doc, time.Now().UTC())
	require.NoError(t, err)

	resp = h.Post("/assignments", objMap{
		"studentId":          student.ID,
		"activityRevisionId": rev.ID,
	}, parentAuth)
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())

	resp = h.Get("/assignments", parentAuth)
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	list := decode[struct {
		Items []domain.StudentAssignment `json:"items"`
	}](t, resp.Body.Bytes())
	require.NotEmpty(t, list.Items)

	resp = h.Get("/learning-sessions", parentAuth)
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
}
