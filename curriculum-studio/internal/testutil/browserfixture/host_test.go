// This package is test-only. It is not imported by any Studio binary and is
// not a production BFF/session implementation or external identity acceptance.
package browserfixture_test

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/aleksclark/primer/curriculum-studio/internal/api"
	"github.com/aleksclark/primer/curriculum-studio/internal/authn"
	"github.com/aleksclark/primer/curriculum-studio/internal/domain"
	"github.com/aleksclark/primer/curriculum-studio/internal/fingerprint"
	"github.com/aleksclark/primer/curriculum-studio/internal/repo"
	"github.com/aleksclark/primer/curriculum-studio/internal/testutil"
	"github.com/aleksclark/primer/curriculum-studio/internal/testutil/factory"
	"github.com/aleksclark/primer/curriculum-studio/internal/testutil/jwttest"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// A real 32x32 PNG-backed ICO, rasterized from the adjacent SVG's existing
// house i-learn artwork. Embedded only in this test binary, never a Studio API.
//
//go:embed testdata/favicon.ico
var fixtureFavicon []byte

type persona struct {
	Name      string
	Subject   uuid.UUID
	Workspace uuid.UUID
	Role      string
}
type fixture struct {
	Server   *httptest.Server
	Personas map[string]persona
	Evidence map[string]any
}

func opaque(prefix string, id uuid.UUID) string {
	return prefix + strings.ReplaceAll(id.String(), "-", "")
}

func newFixture(t *testing.T, webRoot string) *fixture {
	t.Helper()
	// Never inherit an external DB override: this fixture owns disposable data.
	require.Empty(t, os.Getenv(testutil.TestDatabaseEnv), "unset STUDIO_TEST_DATABASE_URL; browser fixture must start its own testcontainer")
	pool := testutil.DB(t)
	ctx := t.Context()
	w1 := factory.Workspace(t, pool, func(w *domain.Workspace) { w.Name = "Workshop Authors" })
	w2 := factory.Workspace(t, pool, func(w *domain.Workspace) { w.Name = "Co-op Readers" })
	w3 := factory.Workspace(t, pool, func(w *domain.Workspace) { w.Name = "Unshared Workshop" })
	personas := map[string]persona{
		"author":   {Name: "Ada Author", Subject: uuid.New(), Workspace: w1.ID, Role: "author"},
		"reviewer": {Name: "Ruth Reviewer", Subject: uuid.New(), Workspace: w1.ID, Role: "reviewer"},
		"owner":    {Name: "Olivia Owner", Subject: uuid.New(), Workspace: w1.ID, Role: "owner"},
		"w2":       {Name: "Co-op Author", Subject: uuid.New(), Workspace: w2.ID, Role: "author"},
		"w3":       {Name: "Unshared Author", Subject: uuid.New(), Workspace: w3.ID, Role: "author"},
	}
	for _, p := range personas {
		factory.Membership(t, pool, func(m *domain.WorkspaceMembership) {
			m.WorkspaceID = p.Workspace
			m.SubjectRef = domain.HumanSubjectRef(p.Subject)
			m.Role = p.Role
			m.DisplayName = p.Name
		})
	}
	c, err := repo.NewCurriculumRepo(pool).Create(ctx, &domain.Curriculum{WorkspaceID: w1.ID, Slug: "s17-workshop-" + uuid.NewString(), Title: "Workshop geometry"})
	require.NoError(t, err)
	framework := factory.Framework(t, pool, func(f *domain.StandardFramework) { f.WorkspaceID = &w1.ID })
	standard := factory.CatalogStandard(t, pool, func(s *domain.CatalogStandard) {
		s.FrameworkID = framework.ID
		s.Code = "S17.SCALE"
		s.Description = "Draw and interpret scale plans"
	})
	graph := repo.NewPlanGraphRepo(pool)
	var current *domain.PlanRevision
	var library *domain.UnitLibraryEntry
	for n := 1; n <= 2; n++ {
		title := "Workshop geometry — baseline"
		if n == 2 {
			title = "Workshop geometry — draft"
		}
		revision, err := repo.NewPlanRevisionRepo(pool).Create(ctx, w1.ID, &domain.PlanRevision{CurriculumID: c.ID, Revision: n, Title: title})
		require.NoError(t, err)
		name := "Read a scale"
		code := "read-scale"
		if n == 2 {
			name = "Draw a scale plan"
			code = "draw-scale"
		}
		outcome, err := graph.CreateOutcome(ctx, w1.ID, &domain.Outcome{PlanRevisionID: revision.ID, Code: code, Title: name, MasteryCriteria: "Label every dimension and unit"})
		require.NoError(t, err)
		_, err = graph.CreateMapping(ctx, w1.ID, &domain.OutcomeStandardMapping{OutcomeID: outcome.ID, StandardID: standard.ID, Alignment: "addresses"})
		require.NoError(t, err)
		_, err = graph.CreateEvidenceRequirement(ctx, w1.ID, &domain.EvidenceRequirement{PlanRevisionID: revision.ID, OutcomeID: outcome.ID, Kind: "portfolio", Description: "Submit a labeled plan"})
		require.NoError(t, err)
		unit, err := graph.CreateUnit(ctx, w1.ID, &domain.Unit{PlanRevisionID: revision.ID, Code: "geometry", Title: "Workshop geometry unit"})
		require.NoError(t, err)
		_, err = graph.CreateUnitOutcome(ctx, w1.ID, &domain.UnitOutcome{UnitID: unit.ID, OutcomeID: outcome.ID, Role: "target"})
		require.NoError(t, err)
		if n == 2 {
			current = revision
			library, err = repo.NewUnitLibraryRepo(pool).SaveUnit(ctx, w1.ID, revision.ID, unit.ID, "Reusable workshop geometry")
			require.NoError(t, err)
		}
	}
	_, err = repo.NewTemplateRepo(pool).Create(ctx, w1.ID, "workshop-starter", "Workshop starter", "project_based_unit", domain.TemplateSeed{Outcomes: []domain.NamedSeed{{Code: "craft", Title: "Craftsmanship"}}, Units: []domain.NamedSeed{{Code: "first", Title: "First workshop", OutcomeCodes: []string{"craft"}}}})
	require.NoError(t, err)
	snapshot := json.RawMessage(`{"fixtureSeed":"S17 materialized item; no generator/provider invocation"}`)
	hash, err := fingerprint.Hash(snapshot)
	require.NoError(t, err)
	run, err := repo.NewMaterializationRunRepo(pool).Create(ctx, &domain.MaterializationRun{WorkspaceID: w1.ID, PlanRevisionID: current.ID, Status: "ready", InputSnapshot: snapshot, InputFingerprint: hash, RequestedBySubjectRef: domain.HumanSubjectRef(personas["author"].Subject)})
	require.NoError(t, err)
	item, err := repo.NewMaterializedItemRepo(pool).Create(ctx, w1.ID, &domain.MaterializedItem{RunID: run.ID, PlanRevisionID: current.ID, Title: "Scale drawing practice", Kind: "lesson", Body: json.RawMessage(`{"text":"Fixture content: draw a labeled 1:10 scale plan."}`)})
	require.NoError(t, err)
	evidence := map[string]any{"curriculumId": opaque("cur_", c.ID), "draftRevisionId": opaque("prev_", current.ID), "itemId": opaque("mit_", item.ID), "runId": opaque("mat_", run.ID), "libraryId": opaque("lib_", library.ID), "workspaces": map[string]string{"w1": opaque("ws_", w1.ID), "w2": opaque("ws_", w2.ID), "w3": opaque("ws_", w3.ID)}}
	key := jwttest.GenerateKey(t)
	jwks := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(jwttest.JWKSDocument(key))
	}))
	t.Cleanup(jwks.Close)
	validator, err := authn.NewValidator(authn.Options{Issuer: jwttest.Issuer, Audience: jwttest.Audience, JWKSURL: jwks.URL})
	require.NoError(t, err)
	_, apiHandler := api.New(pool, api.Options{Validator: validator})
	// Only the opaque fixture-session edge uses memory. All product state lives
	// in PostgreSQL. Tokens are minted server-side, validated by the real API and
	// never returned to the browser or printed to logs.
	sessions := map[string]string{}
	var mu sync.Mutex
	var server *httptest.Server
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if r.Host != strings.TrimPrefix(server.URL, "http://") {
			http.Error(w, "fixture host mismatch", 403)
			return
		}
		if r.Method != "GET" && r.Method != "HEAD" && r.Header.Get("Origin") != server.URL {
			http.Error(w, "fixture same-origin request required", 403)
			return
		}
		if r.URL.Path == "/favicon.ico" {
			if r.Method != http.MethodGet && r.Method != http.MethodHead {
				w.Header().Set("Allow", "GET, HEAD")
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			// Keep the fixture's no-store policy: no cached failures or cross-run
			// assumptions. Only this exact static path is public.
			w.Header().Set("Content-Type", "image/vnd.microsoft.icon")
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("Content-Length", strconv.Itoa(len(fixtureFavicon)))
			if r.Method == http.MethodGet {
				_, _ = w.Write(fixtureFavicon)
			}
			return
		}
		if r.URL.Path == "/_fixture" || r.URL.Path == "/_fixture/" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			require.NoError(t, template.Must(template.New("fixture").Parse(`<!DOCTYPE html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>S17 loopback test fixture</title><link rel="icon" type="image/vnd.microsoft.icon" sizes="32x32" href="/favicon.ico"></head><body><h1>S17 loopback test fixture</h1><p>Not live identity/BFF acceptance. Choose a fixture principal in each independent browser context.</p><form action="/_fixture/session" method="post"><label>Persona <select name="persona">{{range $key,$p := .}}<option value="{{$key}}">{{$p.Name}} ({{$p.Role}})</option>{{end}}</select></label><button>Start fixture session</button></form><p><a href="/_fixture/evidence">Fixture IDs and workspaces</a></p></body></html>`)).Execute(w, personas))
			return
		}
		if r.URL.Path == "/_fixture/evidence" && r.Method == "GET" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(evidence)
			return
		}
		if r.URL.Path == "/_fixture/session" && r.Method == "POST" {
			r.Body = http.MaxBytesReader(w, r.Body, 2048)
			if err := r.ParseForm(); err != nil {
				http.Error(w, "invalid form", 400)
				return
			}
			which := r.PostForm.Get("persona")
			if _, ok := personas[which]; !ok {
				http.Error(w, "unknown fixture persona", 400)
				return
			}
			id := uuid.NewString()
			mu.Lock()
			sessions[id] = which
			mu.Unlock()
			http.SetCookie(w, &http.Cookie{Name: "studio_s17_fixture", Value: id, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: 14400})
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/studio/v1/") {
			cookie, err := r.Cookie("studio_s17_fixture")
			if err != nil {
				http.Error(w, "Choose a test persona at /_fixture/", 401)
				return
			}
			mu.Lock()
			which, ok := sessions[cookie.Value]
			mu.Unlock()
			if !ok {
				http.Error(w, "fixture session expired", 401)
				return
			}
			token := jwttest.Mint(t, key, jwttest.ValidHumanClaims(time.Now(), personas[which].Subject.String()))
			req := r.Clone(r.Context())
			req.Header = r.Header.Clone()
			req.Header.Del("Cookie")
			req.Header.Set("Authorization", "Bearer "+token)
			apiHandler.ServeHTTP(w, req)
			return
		}
		if r.Method != "GET" && r.Method != "HEAD" {
			http.Error(w, "method not allowed", 405)
			return
		}
		if webRoot == "" {
			http.Error(w, "SPA not supplied in API composition test", 404)
			return
		}
		http.FileServer(http.Dir(webRoot)).ServeHTTP(w, r)
	})
	server = httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return &fixture{Server: server, Personas: personas, Evidence: evidence}
}

func TestFixtureSessionsUseRealValidatorAndLocalMemberships(t *testing.T) {
	f := newFixture(t, "")
	for _, which := range []string{"author", "reviewer", "w2", "w3"} {
		jar, err := cookiejar.New(nil)
		require.NoError(t, err)
		client := &http.Client{Jar: jar}
		req, err := http.NewRequest("POST", f.Server.URL+"/_fixture/session", strings.NewReader(url.Values{"persona": {which}}.Encode()))
		require.NoError(t, err)
		req.Header.Set("Origin", f.Server.URL)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		response, err := client.Do(req)
		require.NoError(t, err)
		response.Body.Close()
		response, err = client.Get(f.Server.URL + "/studio/v1/auth/me")
		require.NoError(t, err)
		require.Equal(t, 200, response.StatusCode)
		var body struct{ SubjectRef string }
		require.NoError(t, json.NewDecoder(response.Body).Decode(&body))
		response.Body.Close()
		require.Equal(t, domain.HumanSubjectRef(f.Personas[which].Subject), body.SubjectRef)
		response, err = client.Get(f.Server.URL + "/studio/v1/favicon.ico")
		require.NoError(t, err)
		response.Body.Close()
		require.Equal(t, http.StatusNotFound, response.StatusCode, "authenticated API paths must not become favicon aliases")
	}
	req, err := http.NewRequest("POST", f.Server.URL+"/_fixture/session", strings.NewReader("persona=owner"))
	require.NoError(t, err)
	req.Header.Set("Origin", "https://attacker.invalid")
	response, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	response.Body.Close()
	require.Equal(t, 403, response.StatusCode)
	response, err = http.Get(f.Server.URL + "/studio/v1/auth/me")
	require.NoError(t, err)
	response.Body.Close()
	require.Equal(t, 401, response.StatusCode)
}

func TestBrowserFixtureHost(t *testing.T) {
	if os.Getenv("STUDIO_BROWSER_FIXTURE") != "1" {
		t.Skip("explicit loopback browser fixture launcher only")
	}
	root := os.Getenv("STUDIO_FIXTURE_WEB_ROOT")
	require.NotEmpty(t, root)
	_, err := os.Stat(filepath.Join(root, "index.html"))
	require.NoError(t, err, "build the real Studio SPA first")
	f := newFixture(t, root)
	dir := os.Getenv("STUDIO_FIXTURE_DIR")
	require.NotEmpty(t, dir)
	f.Evidence["baseUrl"] = f.Server.URL
	raw, err := json.MarshalIndent(f.Evidence, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "fixture.json"), raw, 0600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "database-url"), []byte(testutil.URL(t)), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "host.pid"), []byte(fmt.Sprint(os.Getpid())), 0600))
	fmt.Printf("S17_FIXTURE_URL=%s\nS17_FIXTURE_DIR=%s\n", f.Server.URL, dir)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()
}
