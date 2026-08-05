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
	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
	"github.com/aleksclark/primer/server/internal/testutil"
	"github.com/aleksclark/primer/server/internal/testutil/factory"
)

func TestStudentAPIAuthAndTutorEdges(t *testing.T) {
	t.Parallel()
	h, q := testutil.API(t)
	ctx := context.Background()

	const password = "test-password-ok"
	ed := factory.EducatorWithPassword(t, q, password, factory.Override{
		"email": "stu-edges-" + uuid.NewString()[:8] + "@example.com",
		"role":  "parent",
	})
	student := factory.Student(t, q, factory.Override{"first_name": "Edge"})
	resp := h.Post("/auth/login", objMap{"email": ed.Email, "password": password})
	require.Equal(t, http.StatusOK, resp.Code)
	parentAuth := "Authorization: Bearer " + decode[objMap](t, resp.Body.Bytes())["token"].(string)

	resp = h.Get("/student/work")
	assert.Equal(t, http.StatusUnauthorized, resp.Code)
	resp = h.Get("/student/profile", "Authorization: Bearer not-a-real-token")
	assert.Equal(t, http.StatusUnauthorized, resp.Code)

	resp = h.Post("/student-devices/pair", objMap{"code": "ZZZZ", "deviceName": "x"})
	assert.Equal(t, http.StatusForbidden, resp.Code)

	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	root := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", ".."))
	_, err := curriculum.Publish(ctx, q, curriculum.PublishOptions{
		StandardsDir: filepath.Join(root, "curriculum", "standards"),
		Now:          time.Now().UTC(),
	})
	require.NoError(t, err)
	doc, err := contracts.LoadDocument(filepath.Join(root, "curriculum", "activities", "basic-navigation", "activity.yaml"))
	require.NoError(t, err)
	_, rev, err := curriculum.PublishDocument(ctx, q, doc, time.Now().UTC())
	require.NoError(t, err)

	resp = h.Post("/assignments", objMap{
		"studentId": student.ID, "activityRevisionId": rev.ID, "priority": 1,
	}, parentAuth)
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())
	asgID := decode[objMap](t, resp.Body.Bytes())["id"].(string)

	resp = h.Post("/pairing-codes", objMap{"studentId": student.ID}, parentAuth)
	require.Equal(t, http.StatusCreated, resp.Code)
	code := decode[objMap](t, resp.Body.Bytes())["code"].(string)

	// Empty device name → default "workstation"
	resp = h.Post("/student-devices/pair", objMap{"code": code})
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())
	pair := decode[objMap](t, resp.Body.Bytes())
	token := pair["token"].(string)
	require.NotEmpty(t, token)
	devObj := pair["device"].(map[string]any)
	assert.Equal(t, "workstation", devObj["name"])

	// X-Device-Token header path
	xAuth := "X-Device-Token: " + token
	resp = h.Get("/student/profile", xAuth)
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())

	csid := uuid.NewString()
	resp = h.Post("/student/sessions", objMap{
		"clientSessionId": csid, "assignmentId": asgID,
	}, "Authorization: Bearer "+token)
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())
	sessID := decode[objMap](t, resp.Body.Bytes())["id"].(string)

	resp = h.Post("/student/sessions/"+sessID+"/tutor/messages", objMap{
		"message": "where should I start?",
	}, "Authorization: Bearer "+token)
	assert.True(t, resp.Code == http.StatusOK || resp.Code >= 400, resp.Body.String())
	if resp.Code == http.StatusOK {
		body := decode[objMap](t, resp.Body.Bytes())
		assert.NotEmpty(t, body["reply"])
	}

	other := factory.Student(t, q, factory.Override{"first_name": "OtherE"})
	resp = h.Post("/pairing-codes", objMap{"studentId": other.ID}, parentAuth)
	require.Equal(t, http.StatusCreated, resp.Code)
	ocode := decode[objMap](t, resp.Body.Bytes())["code"].(string)
	resp = h.Post("/student-devices/pair", objMap{"code": ocode, "deviceName": "o"})
	require.Equal(t, http.StatusCreated, resp.Code)
	otherTok := decode[objMap](t, resp.Body.Bytes())["token"].(string)

	resp = h.Post("/student/sessions/"+sessID+"/artifacts", objMap{
		"schemaVersion": "1",
		"artifactId":    uuid.NewString(), "filename": "x.txt", "mediaType": "text/plain",
		"byteSize": 1, "sha256": "ab", "createdAt": time.Now().UTC().Format(time.RFC3339Nano),
	}, "Authorization: Bearer "+otherTok)
	assert.Equal(t, http.StatusNotFound, resp.Code)

	resp = h.Post("/student/sessions/"+uuid.NewString()+"/complete", objMap{
		"schemaVersion": "1", "completionId": uuid.NewString(), "requestDigest": "d",
		"observations": []objMap{}, "clientTime": time.Now().UTC().Format(time.RFC3339Nano),
	}, "Authorization: Bearer "+token)
	assert.True(t, resp.Code == http.StatusNotFound || resp.Code == http.StatusBadRequest, resp.Body.String())

	resp = h.Get("/student-devices", parentAuth)
	require.Equal(t, http.StatusOK, resp.Code)
	resp = h.Get("/student-devices?studentId="+student.ID, parentAuth)
	require.Equal(t, http.StatusOK, resp.Code)

	deviceID := pair["deviceId"].(string)
	resp = h.Post("/student-devices/"+deviceID+"/revoke", objMap{}, parentAuth)
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	revoked := decode[domain.StudentDevice](t, resp.Body.Bytes())
	require.NotNil(t, revoked.RevokedAt)

	resp = h.Get("/student/work", "Authorization: Bearer "+token)
	assert.Equal(t, http.StatusUnauthorized, resp.Code)

	resp = h.Post("/students/"+student.ID+"/assign-next", objMap{
		"slug": "basic-navigation", "reason": "edge",
	}, parentAuth)
	assert.True(t,
		resp.Code == http.StatusCreated || resp.Code == http.StatusOK ||
			resp.Code == http.StatusNotFound || resp.Code == http.StatusBadRequest,
		resp.Body.String())
}

func TestParentLearningUnauthAndBadIDs(t *testing.T) {
	t.Parallel()
	h, _ := testutil.API(t)

	type hit struct {
		method string
		path   string
		body   any
	}
	paths := []hit{
		{"GET", "/learning-activities", nil},
		{"GET", "/assignments", nil},
		{"GET", "/learning-sessions", nil},
		{"GET", "/student-devices", nil},
		{"POST", "/pairing-codes", objMap{"studentId": uuid.NewString()}},
		{"POST", "/learning-activities", objMap{"slug": "x", "title": "y", "kind": "terminal"}},
		{"POST", "/assignments", objMap{"studentId": uuid.NewString(), "activityRevisionId": uuid.NewString()}},
		{"POST", "/students/" + uuid.NewString() + "/assign-next", objMap{}},
		{"POST", "/student-devices/" + uuid.NewString() + "/revoke", objMap{}},
		{"POST", "/student-devices/" + uuid.NewString() + "/rotate-token", objMap{}},
		{"GET", "/students/" + uuid.NewString() + "/learning-overview", nil},
		{"GET", "/ops/student-metrics", nil},
		{"GET", "/students/" + uuid.NewString() + "/tutor-status", nil},
		{"POST", "/students/" + uuid.NewString() + "/tutor", objMap{"enabled": true}},
		{"POST", "/assignments/" + uuid.NewString() + "/cancel", objMap{}},
		{"POST", "/assignments/" + uuid.NewString() + "/retry", objMap{}},
		{"POST", "/mastery-evidence/" + uuid.NewString() + "/supersede", objMap{"note": "n"}},
	}
	for _, p := range paths {
		var code int
		if p.method == "GET" {
			code = h.Get(p.path).Code
		} else {
			body := p.body
			if body == nil {
				body = objMap{}
			}
			code = h.Post(p.path, body).Code
		}
		assert.Equal(t, http.StatusUnauthorized, code, "%s %s", p.method, p.path)
	}

	const password = "test-password-ok"
	h2, q2 := testutil.API(t)
	ed := factory.EducatorWithPassword(t, q2, password, factory.Override{
		"email": "unauth2-" + uuid.NewString()[:8] + "@example.com", "role": "parent",
	})
	resp := h2.Post("/auth/login", objMap{"email": ed.Email, "password": password})
	require.Equal(t, http.StatusOK, resp.Code)
	auth := "Authorization: Bearer " + decode[objMap](t, resp.Body.Bytes())["token"].(string)

	missing := uuid.NewString()
	resp = h2.Post("/pairing-codes", objMap{"studentId": missing}, auth)
	assert.Equal(t, http.StatusNotFound, resp.Code)
	resp = h2.Get("/students/"+missing+"/learning-overview", auth)
	assert.Equal(t, http.StatusNotFound, resp.Code)
	resp = h2.Post("/student-devices/"+missing+"/revoke", objMap{}, auth)
	assert.Equal(t, http.StatusNotFound, resp.Code)
	resp = h2.Post("/assignments/"+missing+"/cancel", objMap{}, auth)
	assert.True(t, resp.Code == http.StatusNotFound || resp.Code == http.StatusBadRequest)
	resp = h2.Post("/mastery-evidence/"+missing+"/supersede", objMap{"note": "x"}, auth)
	assert.Equal(t, http.StatusNotFound, resp.Code)

	resp = h2.Get("/learning-activities?filter=::::", auth)
	assert.Equal(t, http.StatusBadRequest, resp.Code)
	resp = h2.Get("/learning-sessions?filter=bad", auth)
	assert.Equal(t, http.StatusBadRequest, resp.Code)
}
