package export_test

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/curriculum-studio/internal/api"
	"github.com/aleksclark/primer/curriculum-studio/internal/artifacts"
	"github.com/aleksclark/primer/curriculum-studio/internal/authn"
	"github.com/aleksclark/primer/curriculum-studio/internal/domain"
	studioexport "github.com/aleksclark/primer/curriculum-studio/internal/export"
	"github.com/aleksclark/primer/curriculum-studio/internal/fingerprint"
	"github.com/aleksclark/primer/curriculum-studio/internal/repo"
	"github.com/aleksclark/primer/curriculum-studio/internal/testutil"
	"github.com/aleksclark/primer/curriculum-studio/internal/testutil/factory"
	"github.com/aleksclark/primer/curriculum-studio/internal/testutil/jwttest"
)

type exportFixture struct {
	pool                              *pgxpool.Pool
	store                             *artifacts.FsStore
	dir                               string
	server                            *httptest.Server
	token, viewerToken, foreignToken  string
	workspace, revision, run, subject uuid.UUID
}

func newExportFixture(t *testing.T) *exportFixture {
	t.Helper()
	ctx := t.Context()
	f := &exportFixture{pool: testutil.DB(t), dir: t.TempDir(), subject: uuid.New()}
	var err error
	f.store, err = artifacts.NewFsStore(f.dir)
	require.NoError(t, err)
	t.Cleanup(func() { f.store.Close() })
	ws := factory.Workspace(t, f.pool)
	f.workspace = ws.ID
	viewer, foreign := uuid.New(), uuid.New()
	factory.SeedMembership(t, f.pool, ws.ID, domain.HumanSubjectRef(f.subject), domain.MembershipRoleAuthor)
	factory.SeedMembership(t, f.pool, ws.ID, domain.HumanSubjectRef(viewer), domain.MembershipRoleViewer)
	otherWS := factory.Workspace(t, f.pool)
	factory.SeedMembership(t, f.pool, otherWS.ID, domain.HumanSubjectRef(foreign), domain.MembershipRoleAuthor)
	key := jwttest.GenerateKey(t)
	jwks := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(jwttest.JWKSDocument(key))
	}))
	t.Cleanup(jwks.Close)
	now := time.Now().UTC().Truncate(time.Second)
	validator, err := authn.NewValidator(authn.Options{Issuer: jwttest.Issuer, Audience: jwttest.Audience, JWKSURL: jwks.URL})
	require.NoError(t, err)
	f.token = jwttest.Mint(t, key, jwttest.ValidHumanClaims(now, f.subject.String()))
	f.viewerToken = jwttest.Mint(t, key, jwttest.ValidHumanClaims(now, viewer.String()))
	f.foreignToken = jwttest.Mint(t, key, jwttest.ValidHumanClaims(now, foreign.String()))
	_, handler := api.New(f.pool, api.Options{Validator: validator, Artifacts: f.store})
	f.server = httptest.NewServer(handler)
	t.Cleanup(f.server.Close)
	cur, err := repo.NewCurriculumRepo(f.pool).Create(ctx, &domain.Curriculum{WorkspaceID: ws.ID, Slug: "exports-" + uuid.NewString(), Title: "Mathematics"})
	require.NoError(t, err)
	rev, err := repo.NewPlanRevisionRepo(f.pool).Create(ctx, ws.ID, &domain.PlanRevision{CurriculumID: cur.ID, Revision: 1, Title: "Fractions & ratios"})
	require.NoError(t, err)
	f.revision = rev.ID
	graphs := repo.NewPlanGraphRepo(f.pool)
	_, err = graphs.CreateObjective(ctx, ws.ID, &domain.Objective{PlanRevisionID: rev.ID, Code: "O1", Title: "Reason precisely"})
	require.NoError(t, err)
	_, err = graphs.CreateOutcome(ctx, ws.ID, &domain.Outcome{PlanRevisionID: rev.ID, Code: "O1.1", Title: "Compare fractions"})
	require.NoError(t, err)
	_, err = graphs.CreateUnit(ctx, ws.ID, &domain.Unit{PlanRevisionID: rev.ID, Code: "U1", Title: "Kitchen mathematics"})
	require.NoError(t, err)
	_, err = graphs.CreateSchedulingConstraint(ctx, ws.ID, &domain.SchedulingConstraint{PlanRevisionID: rev.ID, Kind: "calendar_window", Payload: json.RawMessage(`{"start":"2026-09-01T09:00:00-04:00","end":"2026-09-01T10:00:00-04:00","title":"Fractions, kitchen"}`)})
	require.NoError(t, err)
	require.NoError(t, repo.NewPlanRevisionRepo(f.pool).Publish(ctx, ws.ID, rev.ID, domain.HumanSubjectRef(f.subject)))
	snapshot := json.RawMessage(`{"fixture":"export"}`)
	fp, err := fingerprint.Hash(snapshot)
	require.NoError(t, err)
	run, err := repo.NewMaterializationRunRepo(f.pool).Create(ctx, &domain.MaterializationRun{WorkspaceID: ws.ID, PlanRevisionID: rev.ID, InputSnapshot: snapshot, InputFingerprint: fp})
	require.NoError(t, err)
	f.run = run.ID
	_, err = repo.NewMaterializationRunRepo(f.pool).Start(ctx, ws.ID, run.ID)
	require.NoError(t, err)
	_, err = repo.NewMaterializedItemRepo(f.pool).Create(ctx, ws.ID, &domain.MaterializedItem{RunID: run.ID, PlanRevisionID: rev.ID, Kind: "lesson", Title: "Measure flour", Body: json.RawMessage(`{"text":"Compare one half and one quarter."}`)})
	require.NoError(t, err)
	_, err = repo.NewMaterializationRunRepo(f.pool).Ready(ctx, ws.ID, run.ID)
	require.NoError(t, err)
	return f
}
func (f *exportFixture) request(t *testing.T, method, path, token string, body any) (int, http.Header, []byte) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		require.NoError(t, err)
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(t.Context(), method, f.server.URL+path, reader)
	require.NoError(t, err)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := f.server.Client().Do(req)
	require.NoError(t, err)
	data, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	require.NoError(t, err)
	return resp.StatusCode, resp.Header, data
}
func (f *exportFixture) create(t *testing.T, format string) api.ExportJob {
	t.Helper()
	code, _, b := f.request(t, "POST", "/studio/v1/revisions/"+f.revision.String()+"/exports", f.token, map[string]string{"format": format, "materializationId": f.run.String()})
	require.Equal(t, http.StatusAccepted, code, string(b))
	var job api.ExportJob
	require.NoError(t, json.Unmarshal(b, &job))
	require.Equal(t, api.MaterializationStatus("ready"), job.Status)
	return job
}
func TestP13FormatsFSRoundtripAndAuthorization(t *testing.T) {
	f := newExportFixture(t)
	for _, format := range []string{"markdown", "pdf", "docx", "csv_coverage", "json_bundle", "ical"} {
		t.Run(format, func(t *testing.T) {
			job := f.create(t, format)
			code, headers, data := f.request(t, "GET", job.DownloadURL, f.token, nil)
			require.Equal(t, 200, code, string(data))
			require.NotEmpty(t, data)
			require.Contains(t, headers.Get("Content-Disposition"), "attachment")
			require.Equal(t, "private, no-store", headers.Get("Cache-Control"))
			key := strings.TrimPrefix(job.ArtifactURI, "obj:")
			stored, err := os.ReadFile(filepath.Join(f.dir, key))
			require.NoError(t, err)
			require.Equal(t, stored, data)
			// A fresh FsStore instance sees the bytes, not a process-local cache.
			reopened, err := artifacts.NewFsStore(f.dir)
			require.NoError(t, err)
			r, err := reopened.Get(t.Context(), key)
			require.NoError(t, err)
			got, err := io.ReadAll(r)
			r.Close()
			reopened.Close()
			require.NoError(t, err)
			require.Equal(t, data, got)
			sum := sha256.Sum256(data)
			require.Equal(t, hex.EncodeToString(sum[:]), job.Checksum)
			switch format {
			case "markdown":
				require.Contains(t, headers.Get("Content-Type"), "text/markdown")
				require.Contains(t, string(data), "Measure flour")
			case "pdf":
				require.Equal(t, "application/pdf", headers.Get("Content-Type"))
				require.True(t, bytes.HasPrefix(data, []byte("%PDF-")))
				require.Contains(t, string(data), "Measure flour")
			case "docx":
				require.Contains(t, headers.Get("Content-Type"), "wordprocessingml.document")
				z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
				require.NoError(t, err)
				found := false
				for _, file := range z.File {
					if file.Name == "word/document.xml" {
						r, err := file.Open()
						require.NoError(t, err)
						xml, err := io.ReadAll(r)
						r.Close()
						require.NoError(t, err)
						require.Contains(t, string(xml), "Measure flour")
						found = true
					}
				}
				require.True(t, found)
			case "csv_coverage":
				require.Contains(t, headers.Get("Content-Type"), "text/csv")
				rows, err := csv.NewReader(bytes.NewReader(data)).ReadAll()
				require.NoError(t, err)
				require.Greater(t, len(rows), 1)
				require.Equal(t, "report_id", rows[0][0])
				require.Contains(t, string(data), "OUTCOME_UNMAPPED")
				reportID, err := uuid.Parse(rows[1][0])
				require.NoError(t, err)
				report, err := repo.NewValidationReportRepo(f.pool).GetReport(t.Context(), f.workspace, reportID)
				require.NoError(t, err)
				require.Equal(t, f.revision, report.PlanRevisionID)
			case "json_bundle":
				require.Equal(t, "application/json", headers.Get("Content-Type"))
				require.True(t, json.Valid(data))
				require.Contains(t, string(data), "studio.download.v1")
				require.Contains(t, string(data), "Measure flour")
				require.NotContains(t, string(data), "InputSnapshot")
			case "ical":
				require.Contains(t, headers.Get("Content-Type"), "text/calendar")
				require.Contains(t, string(data), "BEGIN:VEVENT\r\n")
				require.Contains(t, string(data), "DTSTART:20260901T130000Z")
				require.Contains(t, string(data), `SUMMARY:Fractions\, kitchen`)
			}
			for _, path := range []string{job.DownloadURL, job.ManifestURL, "/studio/v1/exports/" + job.ID} {
				code, _, _ = f.request(t, "GET", path, f.foreignToken, nil)
				require.Equal(t, 404, code)
				code, _, _ = f.request(t, "GET", path, "", nil)
				require.Equal(t, 401, code)
				code, _, _ = f.request(t, "GET", path, f.viewerToken, nil)
				require.Equal(t, 200, code)
			}
		})
	}
	var byteColumns int
	require.NoError(t, f.pool.QueryRow(t.Context(), `SELECT count(*) FROM information_schema.columns WHERE table_schema='curriculum_studio' AND table_name='exports' AND data_type='bytea'`).Scan(&byteColumns))
	require.Zero(t, byteColumns)
	code, _, _ := f.request(t, "POST", "/studio/v1/revisions/"+f.revision.String()+"/exports", f.viewerToken, map[string]string{"format": "pdf"})
	require.Equal(t, 403, code)
}
func TestP13Provenance(t *testing.T) {
	f := newExportFixture(t)
	job := f.create(t, "json_bundle")
	code, _, b := f.request(t, "GET", job.ManifestURL, f.token, nil)
	require.Equal(t, 200, code)
	var manifest studioexport.Manifest
	require.NoError(t, json.Unmarshal(b, &manifest))
	require.Equal(t, f.revision, manifest.PlanRevisionID)
	require.NotNil(t, manifest.MaterializationRunID)
	require.Equal(t, f.run, *manifest.MaterializationRunID)
	require.Equal(t, domain.HumanSubjectRef(f.subject), manifest.CreatedBy)
	require.Equal(t, job.Checksum, manifest.Checksum)
	id, err := uuid.Parse(strings.TrimPrefix(job.ID, "exp_"))
	require.NoError(t, err)
	row, err := repo.NewExportRepo(f.pool).Get(t.Context(), f.workspace, id)
	require.NoError(t, err)
	require.Equal(t, &f.run, row.RunID)
	require.Equal(t, &f.revision, row.PlanRevisionID)
	require.Equal(t, manifest.CreatedBy, row.RequestedBySubjectRef)
	// A real run from another revision must not be attached to this export.
	cur, err := repo.NewCurriculumRepo(f.pool).Create(t.Context(), &domain.Curriculum{WorkspaceID: f.workspace, Slug: uuid.NewString(), Title: "Other"})
	require.NoError(t, err)
	rev, err := repo.NewPlanRevisionRepo(f.pool).Create(t.Context(), f.workspace, &domain.PlanRevision{CurriculumID: cur.ID, Revision: 1, Title: "Other"})
	require.NoError(t, err)
	code, _, _ = f.request(t, "POST", "/studio/v1/revisions/"+rev.ID.String()+"/exports", f.token, map[string]string{"format": "json_bundle", "materializationId": f.run.String()})
	require.Equal(t, 400, code)
	// Revision-only export has no fabricated run id.
	code, _, b = f.request(t, "POST", "/studio/v1/revisions/"+f.revision.String()+"/exports", f.token, map[string]string{"format": "markdown"})
	require.Equal(t, 202, code)
	require.NoError(t, json.Unmarshal(b, &job))
	code, _, b = f.request(t, "GET", job.ManifestURL, f.token, nil)
	require.Equal(t, 200, code)
	require.NotContains(t, string(b), "materialization_run_id")
}
func TestP13DownloadStoreFail(t *testing.T) {
	f := newExportFixture(t)
	job := f.create(t, "markdown")
	key := strings.TrimPrefix(job.ArtifactURI, "obj:")
	require.NoError(t, f.store.Put(t.Context(), key, []byte("corrupted object"), "text/markdown"))
	code, _, b := f.request(t, "GET", job.DownloadURL, f.token, nil)
	require.Equal(t, 503, code)
	require.Contains(t, string(b), "checksum mismatch")
	require.NoError(t, f.store.Delete(t.Context(), key))
	code, _, _ = f.request(t, "GET", job.DownloadURL, f.token, nil)
	require.Equal(t, 503, code)
}

func TestP13StoreFail(t *testing.T) {
	f := newExportFixture(t)
	// A real filesystem obstruction causes Put to fail; no fake Store involved.
	require.NoError(t, os.WriteFile(filepath.Join(f.dir, "exports"), []byte("not a directory"), 0600))
	code, _, b := f.request(t, "POST", "/studio/v1/revisions/"+f.revision.String()+"/exports", f.token, map[string]string{"format": "pdf"})
	require.Equal(t, 503, code, string(b))
	require.Contains(t, string(b), "export failed")
	rows, err := repo.NewExportRepo(f.pool).ListByWorkspace(t.Context(), f.workspace)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "failed", rows[0].Status)
	require.Empty(t, rows[0].ArtifactRef)
	require.Empty(t, rows[0].Checksum)
	require.NotNil(t, rows[0].CompletedAt)
	id := "exp_" + strings.ReplaceAll(rows[0].ID.String(), "-", "")
	code, _, b = f.request(t, "GET", "/studio/v1/exports/"+id, f.token, nil)
	require.Equal(t, 200, code)
	require.Contains(t, string(b), "errorMessage")
	require.Contains(t, string(b), `"status":"failed"`)
	code, _, _ = f.request(t, "GET", "/studio/v1/exports/"+id+"/download", f.token, nil)
	require.Equal(t, 409, code)
}
