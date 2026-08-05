package api_test

import (
	"context"
	"net/http"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/curriculum"
	"github.com/aleksclark/primer/server/internal/domain"
	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
	"github.com/aleksclark/primer/server/internal/testutil"
	"github.com/aleksclark/primer/server/internal/testutil/factory"
)

func TestListAndRevokeStudentDevices(t *testing.T) {
	t.Parallel()
	h, q := testutil.API(t)

	const password = "test-password-ok"
	ed := factory.EducatorWithPassword(t, q, password, factory.Override{
		"email": "device-diag@example.com",
		"role":  "parent",
	})
	student := factory.Student(t, q, factory.Override{"first_name": "Dev", "last_name": "Ice"})

	resp := h.Post("/auth/login", objMap{"email": ed.Email, "password": password})
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	parentToken := decode[objMap](t, resp.Body.Bytes())["token"].(string)
	parentAuth := "Authorization: Bearer " + parentToken

	resp = h.Post("/pairing-codes", objMap{"studentId": student.ID}, parentAuth)
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())
	code := decode[objMap](t, resp.Body.Bytes())["code"].(string)

	resp = h.Post("/student-devices/pair", objMap{"code": code, "deviceName": "ws-a"})
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())
	pair := decode[objMap](t, resp.Body.Bytes())
	deviceID := pair["deviceId"].(string)
	deviceToken := pair["token"].(string)
	require.NotEmpty(t, deviceID)
	require.NotEmpty(t, deviceToken)

	// Touch via authenticated call so lastSeen is set.
	resp = h.Get("/student/profile", "Authorization: Bearer "+deviceToken)
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())

	resp = h.Get("/student-devices", parentAuth)
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	list := decode[struct {
		Items []domain.StudentDevice `json:"items"`
	}](t, resp.Body.Bytes())
	require.NotEmpty(t, list.Items)
	found := false
	for _, d := range list.Items {
		if d.ID == deviceID {
			found = true
			assert.Equal(t, "ws-a", d.Name)
			assert.Equal(t, student.ID, d.StudentID)
			assert.Nil(t, d.RevokedAt)
			assert.NotNil(t, d.LastSeenAt)
		}
	}
	require.True(t, found, "device missing from list")

	resp = h.Get("/student-devices?studentId="+student.ID, parentAuth)
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	filtered := decode[struct {
		Items []domain.StudentDevice `json:"items"`
	}](t, resp.Body.Bytes())
	require.Len(t, filtered.Items, 1)
	assert.Equal(t, deviceID, filtered.Items[0].ID)

	resp = h.Post("/student-devices/"+deviceID+"/revoke", objMap{}, parentAuth)
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	revoked := decode[domain.StudentDevice](t, resp.Body.Bytes())
	require.NotNil(t, revoked.RevokedAt)

	// Device token no longer works.
	resp = h.Get("/student/profile", "Authorization: Bearer "+deviceToken)
	require.Equal(t, http.StatusUnauthorized, resp.Code)

	// List still shows revoked device for diagnostics.
	resp = h.Get("/student-devices", parentAuth)
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	after := decode[struct {
		Items []domain.StudentDevice `json:"items"`
	}](t, resp.Body.Bytes())
	var saw *domain.StudentDevice
	for i := range after.Items {
		if after.Items[i].ID == deviceID {
			saw = &after.Items[i]
		}
	}
	require.NotNil(t, saw)
	require.NotNil(t, saw.RevokedAt)

	// Double revoke → 404
	resp = h.Post("/student-devices/"+deviceID+"/revoke", objMap{}, parentAuth)
	require.Equal(t, http.StatusNotFound, resp.Code)
}

func TestAssignNextCreatesAssignment(t *testing.T) {
	t.Parallel()
	h, q := testutil.API(t)
	ctx := context.Background()

	const password = "test-password-ok"
	ed := factory.EducatorWithPassword(t, q, password, factory.Override{
		"email": "assign-next@example.com",
		"role":  "parent",
	})
	student := factory.Student(t, q, factory.Override{"first_name": "Next", "last_name": "Assign"})

	resp := h.Post("/auth/login", objMap{"email": ed.Email, "password": password})
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	parentToken := decode[objMap](t, resp.Body.Bytes())["token"].(string)
	parentAuth := "Authorization: Bearer " + parentToken

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
	require.NotEmpty(t, rev.ID)

	// assign-next by slug
	resp = h.Post("/students/"+student.ID+"/assign-next", objMap{
		"slug": "basic-navigation",
	}, parentAuth)
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())
	body := decode[struct {
		Assignment domain.StudentAssignment `json:"assignment"`
		Reason     string                   `json:"reason"`
		Created    bool                     `json:"created"`
	}](t, resp.Body.Bytes())
	assert.True(t, body.Created)
	assert.Equal(t, student.ID, body.Assignment.StudentID)
	assert.Equal(t, rev.ID, body.Assignment.ActivityRevisionID)
	assert.Contains(t, body.Reason, "slug:")

	// Second call returns already-open
	resp = h.Post("/students/"+student.ID+"/assign-next", objMap{
		"slug": "basic-navigation",
	}, parentAuth)
	require.True(t, resp.Code == http.StatusCreated || resp.Code == http.StatusOK, resp.Body.String())
	body2 := decode[struct {
		Assignment domain.StudentAssignment `json:"assignment"`
		Created    bool                     `json:"created"`
		Reason     string                   `json:"reason"`
	}](t, resp.Body.Bytes())
	assert.False(t, body2.Created)
	assert.Equal(t, body.Assignment.ID, body2.Assignment.ID)
	assert.Equal(t, "already-open", body2.Reason)
}

func TestAssignNextPrefersTypingReinforcement(t *testing.T) {
	t.Parallel()
	h, q := testutil.API(t)
	ctx := context.Background()

	const password = "test-password-ok"
	ed := factory.EducatorWithPassword(t, q, password, factory.Override{
		"email": "reinf-next@example.com",
		"role":  "parent",
	})
	student := factory.Student(t, q, factory.Override{"first_name": "Type", "last_name": "Reinf"})

	resp := h.Post("/auth/login", objMap{"email": ed.Email, "password": password})
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	parentToken := decode[objMap](t, resp.Body.Bytes())["token"].(string)
	parentAuth := "Authorization: Bearer " + parentToken

	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", ".."))
	_, err := curriculum.Publish(ctx, q, curriculum.PublishOptions{
		StandardsDir: filepath.Join(repoRoot, "curriculum", "standards"),
		Now:          time.Now().UTC(),
	})
	require.NoError(t, err)

	// Publish both terminal and typing activities.
	navDoc, err := contracts.LoadDocument(filepath.Join(repoRoot, "curriculum", "activities", "basic-navigation", "activity.yaml"))
	require.NoError(t, err)
	_, _, err = curriculum.PublishDocument(ctx, q, navDoc, time.Now().UTC())
	require.NoError(t, err)

	typeDoc, err := contracts.LoadDocument(filepath.Join(repoRoot, "curriculum", "activities", "command-typing-basics", "activity.yaml"))
	require.NoError(t, err)
	_, typeRev, err := curriculum.PublishDocument(ctx, q, typeDoc, time.Now().UTC())
	require.NoError(t, err)

	// Mastery due on a TYPE standard.
	var typeStdID string
	err = q.QueryRow(ctx, `SELECT id FROM standards WHERE code = $1`, "PRIMER.DL.6.TYPE.1").Scan(&typeStdID)
	require.NoError(t, err)

	past := time.Now().UTC().Add(-time.Hour)
	factory.MasteryRecord(t, q, factory.Override{
		"student_id":            student.ID,
		"standard_id":           typeStdID,
		"status":                "approaching",
		"confidence":            0.5,
		"next_reinforcement_at": past,
	})

	resp = h.Post("/students/"+student.ID+"/assign-next", objMap{}, parentAuth)
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())
	body := decode[struct {
		Assignment domain.StudentAssignment `json:"assignment"`
		Reason     string                   `json:"reason"`
		Created    bool                     `json:"created"`
	}](t, resp.Body.Bytes())
	assert.True(t, body.Created)
	assert.Equal(t, typeRev.ID, body.Assignment.ActivityRevisionID)
	assert.Contains(t, body.Reason, "reinforcement")
	assert.Contains(t, body.Reason, "TYPE")
}
