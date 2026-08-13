package api_test

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/http"
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

func TestArtifactPortfolioListAndCrossStudentDenied(t *testing.T) {
	t.Parallel()
	h, q := testutil.API(t)
	ctx := context.Background()

	const password = "portfolio-list-ok"
	ed := factory.EducatorWithPassword(t, q, password, factory.Override{
		"email": "portfolio-list@example.com",
		"role":  "parent",
	})
	student := factory.Student(t, q, factory.Override{"first_name": "Art", "last_name": "List"})
	other := factory.Student(t, q, factory.Override{"first_name": "Oth", "last_name": "Er"})

	resp := h.Post("/auth/login", objMap{"email": ed.Email, "password": password})
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	parentToken := decode[objMap](t, resp.Body.Bytes())["token"].(string)
	parentAuth := parentAuthHeader(parentToken)

	doc := &contracts.ActivityDocument{
		SchemaVersion: contracts.SchemaVersion,
		Slug:          "artifact-list-cap",
		Title:         "Artifact List Cap",
		Summary:       "upload evidence",
		Kind:          contracts.KindTerminal,
		SubjectCode:   "digital-literacy",
		Standards: []contracts.StandardRef{
			{Code: "PRIMER.DL.6.NAV.1", Role: contracts.StandardRolePrimary},
		},
		Content: contracts.ActivityContent{
			Objective:    "ship",
			Instructions: "build",
			Terminal: &contracts.TerminalContent{
				RuntimeProfile: contracts.RuntimeCoreutilsBasic,
				Fixtures: []contracts.FixtureEntry{
					{Path: "home", Type: contracts.FixtureDirectory},
				},
			},
			Tasks: []contracts.Task{{
				ID: "t1", Title: "T", Instructions: "go",
				Completion: contracts.CheckTree{CheckID: "c1"},
			}},
			Checks: []contracts.Check{{
				ID: "c1", Kind: contracts.CheckFileExists,
				Params: map[string]any{"path": "home"},
			}},
			Artifacts: &contracts.ArtifactPolicy{
				Enabled: true, MaxFiles: 5, MaxBytesEach: 2048, MaxBytesTotal: 8192,
				AllowedTypes: []string{"text/plain"}, RetainDays: 14,
			},
		},
	}
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
	resp = h.Post("/student-devices/pair", objMap{"code": code, "deviceName": "ws-list"})
	require.Equal(t, http.StatusCreated, resp.Code)
	deviceAuth := parentAuthHeader(decode[objMap](t, resp.Body.Bytes())["token"].(string))

	// Pair other student device for isolation checks.
	resp = h.Post("/pairing-codes", objMap{"studentId": other.ID}, parentAuth)
	require.Equal(t, http.StatusCreated, resp.Code)
	otherCode := decode[objMap](t, resp.Body.Bytes())["code"].(string)
	resp = h.Post("/student-devices/pair", objMap{"code": otherCode, "deviceName": "ws-other"})
	require.Equal(t, http.StatusCreated, resp.Code)
	otherAuth := parentAuthHeader(decode[objMap](t, resp.Body.Bytes())["token"].(string))

	resp = h.Post("/student/sessions", objMap{
		"clientSessionId": uuid.NewString(),
		"assignmentId":    asgID,
	}, deviceAuth)
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())
	sessionID := decode[objMap](t, resp.Body.Bytes())["id"].(string)

	// Cross-student cannot reserve on foreign session.
	resp = h.Post("/student/sessions/"+sessionID+"/artifacts/reserve", objMap{
		"schemaVersion": "1",
		"artifactId":    uuid.NewString(),
		"filename":      "x.txt",
		"mediaType":     "text/plain",
		"byteSize":      1,
		"sha256":        hex.EncodeToString(make([]byte, 32)),
		"createdAt":     time.Now().UTC().Format(time.RFC3339Nano),
	}, otherAuth)
	assert.Equal(t, http.StatusNotFound, resp.Code, resp.Body.String())

	// Cross-student continuity lookup denied.
	resp = h.Get("/student/sessions/"+sessionID+"/continuity", otherAuth)
	assert.Equal(t, http.StatusNotFound, resp.Code, resp.Body.String())

	payload := []byte("portfolio note")
	sum := sha256.Sum256(payload)
	digest := hex.EncodeToString(sum[:])
	artID := uuid.NewString()

	resp = h.Post("/student/sessions/"+sessionID+"/artifacts/reserve", objMap{
		"schemaVersion": "1",
		"artifactId":    artID,
		"filename":      "note.txt",
		"mediaType":     "text/plain",
		"byteSize":      len(payload),
		"sha256":        digest,
		"createdAt":     time.Now().UTC().Format(time.RFC3339Nano),
	}, deviceAuth)
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())

	// Bad base64 / digest mismatch on upload.
	resp = h.Post("/student/sessions/"+sessionID+"/artifacts/upload", objMap{
		"artifactId":    artID,
		"contentBase64": "%%%not-base64%%%",
		"sha256":        digest,
	}, deviceAuth)
	assert.Equal(t, http.StatusBadRequest, resp.Code, resp.Body.String())

	resp = h.Post("/student/sessions/"+sessionID+"/artifacts/upload", objMap{
		"artifactId":    artID,
		"contentBase64": base64.StdEncoding.EncodeToString(payload),
		"sha256":        hex.EncodeToString(make([]byte, 32)),
	}, deviceAuth)
	assert.Equal(t, http.StatusBadRequest, resp.Code, resp.Body.String())

	resp = h.Post("/student/sessions/"+sessionID+"/artifacts/upload", objMap{
		"artifactId":    artID,
		"contentBase64": base64.StdEncoding.EncodeToString(payload),
		"sha256":        digest,
	}, deviceAuth)
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())

	// Promote to portfolio (not fixture bundle).
	resp = h.Post("/portfolio/promote", objMap{
		"artifactId":  artID,
		"title":       "Field note",
		"destination": "portfolio",
	}, parentAuth)
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())
	promoted := decode[objMap](t, resp.Body.Bytes())
	item := promoted["item"].(map[string]any)
	assert.Equal(t, "Field note", item["title"])
	assert.Nil(t, promoted["bundle"])

	// List portfolio for student.
	resp = h.Get("/students/"+student.ID+"/portfolio", parentAuth)
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	list := decode[struct {
		Items []domain.PortfolioItem `json:"items"`
		Total int                    `json:"totalCount"`
	}](t, resp.Body.Bytes())
	require.GreaterOrEqual(t, list.Total, 1)
	require.NotEmpty(t, list.Items)
	assert.Equal(t, "Field note", list.Items[0].Title)

	// Empty portfolio for other student.
	resp = h.Get("/students/"+other.ID+"/portfolio", parentAuth)
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	empty := decode[struct {
		Items []domain.PortfolioItem `json:"items"`
		Total int                    `json:"totalCount"`
	}](t, resp.Body.Bytes())
	assert.Equal(t, 0, empty.Total)
	assert.Empty(t, empty.Items)

	// Unknown session continuity → 404
	resp = h.Get("/student/sessions/"+uuid.NewString()+"/continuity", deviceAuth)
	assert.Equal(t, http.StatusNotFound, resp.Code, resp.Body.String())

	// Unauthenticated promote denied.
	resp = h.Post("/portfolio/promote", objMap{"artifactId": artID, "destination": "portfolio"})
	assert.True(t, resp.Code == http.StatusUnauthorized || resp.Code == http.StatusForbidden, resp.Body.String())
}

func TestCurriculumImportPlanInvalidBundle(t *testing.T) {
	t.Parallel()
	h, q := testutil.API(t)

	const password = "import-invalid"
	ed := factory.EducatorWithPassword(t, q, password, factory.Override{
		"email": "import-invalid@example.com",
		"role":  "parent",
	})
	resp := h.Post("/auth/login", objMap{"email": ed.Email, "password": password})
	require.Equal(t, http.StatusOK, resp.Code)
	parentAuth := parentAuthHeader(decode[objMap](t, resp.Body.Bytes())["token"].(string))

	// Invalid activity kind / missing required fields → plan.valid false or 4xx.
	resp = h.Post("/curriculum/import/plan", objMap{
		"schemaVersion": "1",
		"sourceLabel":   "bad",
		"activities": []objMap{{
			"schemaVersion": "1",
			"slug":          "not-valid!!!",
			"title":         "",
			"kind":          "not-a-kind",
			"subjectCode":   "digital-literacy",
			"standards":     []objMap{},
			"content":       objMap{},
		}},
	}, parentAuth)
	// Handler returns 200 with valid=false for planned-but-invalid, or 400/422 on hard fail.
	if resp.Code == http.StatusOK {
		plan := decode[objMap](t, resp.Body.Bytes())
		assert.Equal(t, false, plan["valid"], plan)
	} else {
		assert.True(t, resp.Code == http.StatusBadRequest || resp.Code == http.StatusUnprocessableEntity, resp.Body.String())
	}

	// Unauthenticated plan denied.
	resp = h.Post("/curriculum/import/plan", objMap{"schemaVersion": "1"})
	assert.True(t, resp.Code == http.StatusUnauthorized || resp.Code == http.StatusForbidden, resp.Body.String())
}
