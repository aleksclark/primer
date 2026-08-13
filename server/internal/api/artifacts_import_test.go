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
	"github.com/aleksclark/primer/server/internal/repo"
	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
	"github.com/aleksclark/primer/server/internal/testutil"
	"github.com/aleksclark/primer/server/internal/testutil/factory"
)

func TestArtifactUploadPromoteContinuity(t *testing.T) {
	t.Parallel()
	h, q := testutil.API(t)
	ctx := context.Background()

	const password = "test-password-ok"
	ed := factory.EducatorWithPassword(t, q, password, factory.Override{
		"email": "portfolio-parent@example.com",
		"role":  "parent",
	})
	student := factory.Student(t, q, factory.Override{"first_name": "Port", "last_name": "Folio"})

	// Login
	resp := h.Post("/auth/login", objMap{"email": ed.Email, "password": password})
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	parentToken := decode[objMap](t, resp.Body.Bytes())["token"].(string)
	parentAuth := "Authorization: Bearer " + parentToken

	// Publish activity with artifacts enabled.
	doc := &contracts.ActivityDocument{
		SchemaVersion: contracts.SchemaVersion,
		Slug:          "artifact-capstone",
		Title:         "Artifact Capstone",
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
				Enabled:       true,
				MaxFiles:      5,
				MaxBytesEach:  1024,
				MaxBytesTotal: 4096,
				AllowedTypes:  []string{"text/plain"},
				RetainDays:    30,
			},
		},
	}
	_, rev, err := curriculum.PublishDocument(ctx, q, doc, time.Now().UTC())
	require.NoError(t, err)
	require.NotNil(t, rev)

	resp = h.Post("/assignments", objMap{
		"studentId": student.ID, "activityRevisionId": rev.ID, "priority": 1,
	}, parentAuth)
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())
	asgID := decode[objMap](t, resp.Body.Bytes())["id"].(string)

	// Pair device
	resp = h.Post("/pairing-codes", objMap{"studentId": student.ID}, parentAuth)
	require.Equal(t, http.StatusCreated, resp.Code)
	code := decode[objMap](t, resp.Body.Bytes())["code"].(string)
	resp = h.Post("/student-devices/pair", objMap{"code": code, "deviceName": "ws"})
	require.Equal(t, http.StatusCreated, resp.Code)
	deviceAuth := "Authorization: Bearer " + decode[objMap](t, resp.Body.Bytes())["token"].(string)

	resp = h.Post("/student/sessions", objMap{
		"clientSessionId": uuid.NewString(),
		"assignmentId":    asgID,
	}, deviceAuth)
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())
	sessionID := decode[objMap](t, resp.Body.Bytes())["id"].(string)

	payload := []byte("field report v1")
	sum := sha256.Sum256(payload)
	digest := hex.EncodeToString(sum[:])
	artID := uuid.NewString()

	// Reserve
	resp = h.Post("/student/sessions/"+sessionID+"/artifacts/reserve", objMap{
		"schemaVersion": "1",
		"artifactId":    artID,
		"filename":      "report.txt",
		"mediaType":     "text/plain",
		"byteSize":      len(payload),
		"sha256":        digest,
		"createdAt":     time.Now().UTC().Format(time.RFC3339Nano),
	}, deviceAuth)
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())
	reserved := decode[objMap](t, resp.Body.Bytes())
	assert.Equal(t, domain.ArtifactStatusReserved, reserved["status"])

	// Idempotent reserve
	resp = h.Post("/student/sessions/"+sessionID+"/artifacts/reserve", objMap{
		"schemaVersion": "1",
		"artifactId":    artID,
		"filename":      "report.txt",
		"mediaType":     "text/plain",
		"byteSize":      len(payload),
		"sha256":        digest,
		"createdAt":     time.Now().UTC().Format(time.RFC3339Nano),
	}, deviceAuth)
	require.True(t, resp.Code == http.StatusCreated || resp.Code == http.StatusOK, resp.Body.String())

	// Reject oversized
	resp = h.Post("/student/sessions/"+sessionID+"/artifacts/reserve", objMap{
		"schemaVersion": "1",
		"artifactId":    uuid.NewString(),
		"filename":      "big.txt",
		"mediaType":     "text/plain",
		"byteSize":      10_000,
		"sha256":        hex.EncodeToString(make([]byte, 32)),
		"createdAt":     time.Now().UTC().Format(time.RFC3339Nano),
	}, deviceAuth)
	assert.Equal(t, http.StatusBadRequest, resp.Code, resp.Body.String())

	// Upload
	resp = h.Post("/student/sessions/"+sessionID+"/artifacts/upload", objMap{
		"artifactId":    artID,
		"contentBase64": base64.StdEncoding.EncodeToString(payload),
		"sha256":        digest,
	}, deviceAuth)
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())
	uploaded := decode[objMap](t, resp.Body.Bytes())
	assert.Equal(t, true, uploaded["bytesStored"])
	assert.Equal(t, domain.ArtifactStatusUploaded, uploaded["status"])
	assert.NotEmpty(t, uploaded["storagePath"])

	// Upload retry idempotent
	resp = h.Post("/student/sessions/"+sessionID+"/artifacts/upload", objMap{
		"artifactId":    artID,
		"contentBase64": base64.StdEncoding.EncodeToString(payload),
	}, deviceAuth)
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())

	// Promote to fixture bundle
	resp = h.Post("/portfolio/promote", objMap{
		"artifactId":  artID,
		"title":       "report.txt",
		"destination": "fixture_bundle",
	}, parentAuth)
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())
	promoted := decode[objMap](t, resp.Body.Bytes())
	require.NotNil(t, promoted["bundle"])
	bundle := promoted["bundle"].(map[string]any)
	bundleID := bundle["id"].(string)

	// Bind continuity on a second assignment of same revision
	resp = h.Post("/assignments", objMap{
		"studentId": student.ID, "activityRevisionId": rev.ID, "priority": 2,
	}, parentAuth)
	require.Equal(t, http.StatusCreated, resp.Code)
	asg2 := decode[objMap](t, resp.Body.Bytes())["id"].(string)

	resp = h.Post("/assignments/"+asg2+"/continuity", objMap{
		"studentId":      student.ID,
		"continuityMode": "optional_previous",
		"bundleId":       bundleID,
	}, parentAuth)
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())

	resp = h.Post("/student/sessions", objMap{
		"clientSessionId": uuid.NewString(),
		"assignmentId":    asg2,
	}, deviceAuth)
	require.Equal(t, http.StatusCreated, resp.Code)
	sess2 := decode[objMap](t, resp.Body.Bytes())["id"].(string)

	resp = h.Get("/student/sessions/"+sess2+"/continuity", deviceAuth)
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	cont := decode[objMap](t, resp.Body.Bytes())
	assert.Equal(t, "optional_previous", cont["mode"])
	require.NotNil(t, cont["bundle"])
}

func TestCurriculumImportAPIPlanApply(t *testing.T) {
	t.Parallel()
	h, q := testutil.API(t)
	_ = q

	const password = "test-password-ok"
	ed := factory.EducatorWithPassword(t, q, password, factory.Override{
		"email": "import-parent@example.com",
		"role":  "parent",
	})
	resp := h.Post("/auth/login", objMap{"email": ed.Email, "password": password})
	require.Equal(t, http.StatusOK, resp.Code)
	parentAuth := "Authorization: Bearer " + decode[objMap](t, resp.Body.Bytes())["token"].(string)

	bundle := objMap{
		"schemaVersion": "1",
		"sourceLabel":   "api-test",
		"standards": []objMap{{
			"code": "PRIMER.DL.6.NAV.9", "source": "custom", "subjectCode": "digital-literacy",
			"domain": "nav", "cluster": "api", "description": "api import std", "gradeLevel": 6,
		}},
		"activities": []objMap{{
			"schemaVersion": "1",
			"slug":          "api-import-act",
			"title":         "API Import Act",
			"summary":       "s",
			"kind":          "terminal",
			"subjectCode":   "digital-literacy",
			"standards":     []objMap{{"code": "PRIMER.DL.6.NAV.9", "role": "primary"}},
			"content": objMap{
				"objective": "o", "instructions": "i",
				"terminal": objMap{
					"runtimeProfile": "coreutils-basic",
					"fixtures":       []objMap{{"path": "home", "type": "directory"}},
				},
				"tasks":  []objMap{{"id": "t1", "title": "T", "instructions": "g", "completion": objMap{"checkId": "c1"}}},
				"checks": []objMap{{"id": "c1", "kind": "file_exists", "params": objMap{"path": "home"}}},
			},
		}},
	}

	resp = h.Post("/curriculum/import/plan", bundle, parentAuth)
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	plan := decode[objMap](t, resp.Body.Bytes())
	require.Equal(t, true, plan["valid"], plan)
	digest := plan["bundleDigest"].(string)
	require.NotEmpty(t, digest)

	// Plan did not create activity
	page, err := repo.LearningActivities.List(context.Background(), q, repo.ListParams{
		Limit: 5, Filters: map[string]any{"slug": "api-import-act"},
	})
	require.NoError(t, err)
	assert.Equal(t, 0, page.TotalCount)

	resp = h.Post("/curriculum/import/apply", objMap{
		"bundle":       bundle,
		"bundleDigest": digest,
		"sourceLabel":  "api-test",
	}, parentAuth)
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	apply1 := decode[objMap](t, resp.Body.Bytes())
	manifest := apply1["manifest"].(map[string]any)
	assert.Equal(t, digest, manifest["bundleDigest"])
	assert.Empty(t, manifest["enrolledStudents"])
	assert.Empty(t, manifest["assignedStudents"])

	page, err = repo.LearningActivities.List(context.Background(), q, repo.ListParams{
		Limit: 5, Filters: map[string]any{"slug": "api-import-act"},
	})
	require.NoError(t, err)
	assert.Equal(t, 1, page.TotalCount)

	// Idempotent re-apply
	resp = h.Post("/curriculum/import/apply", objMap{
		"bundle": bundle, "bundleDigest": digest,
	}, parentAuth)
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	apply2 := decode[objMap](t, resp.Body.Bytes())
	m2 := apply2["manifest"].(map[string]any)
	assert.Equal(t, true, m2["idempotentReplay"])

	// Drift rejected
	resp = h.Post("/curriculum/import/apply", objMap{
		"bundle": bundle, "bundleDigest": "00" + digest[2:],
	}, parentAuth)
	assert.Equal(t, http.StatusBadRequest, resp.Code, resp.Body.String())
}
