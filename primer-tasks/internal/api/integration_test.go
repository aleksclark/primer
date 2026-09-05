package api

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	tasksdb "primer-tasks/internal/db"
)

const (
	tenantA = "00000000-0000-0000-0000-0000000000a1"
	tenantB = "00000000-0000-0000-0000-0000000000b1"
)

func integrationPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)

	dsn := os.Getenv("TASKS_TEST_DATABASE_URL")
	if dsn == "" {
		if err := exec.Command("docker", "info").Run(); err != nil {
			if os.Getenv("PRIMER_TASKS_COVERAGE_GATE") == "1" {
				t.Fatalf("Docker is required for the Tasks coverage gate: %v", err)
			}
			t.Skipf("Docker is unavailable; skipping PostgreSQL integration tests: %v", err)
		}
		container, err := tcpostgres.Run(ctx, "postgres:17-alpine",
			tcpostgres.WithDatabase("primer_tasks_test"),
			tcpostgres.WithUsername("tasks"),
			tcpostgres.WithPassword("tasks"),
			testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(60*time.Second)),
		)
		if err != nil {
			t.Fatalf("start PostgreSQL testcontainer: %v", err)
		}
		t.Cleanup(func() { _ = container.Terminate(context.Background()) })
		dsn, err = container.ConnectionString(ctx, "sslmode=disable")
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := tasksdb.SafeDatabaseName(dsn); err != nil {
		t.Fatalf("unsafe integration database: %v", err)
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := pool.Ping(ctx); err != nil {
		t.Fatal(err)
	}
	if err := tasksdb.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	return pool
}

func seedIntegration(t *testing.T, pool *pgxpool.Pool) (alice, bob string) {
	t.Helper()
	ctx := context.Background()
	_, err := pool.Exec(ctx, `INSERT INTO tenants(id,name) VALUES ($1,'Household A'),($2,'Household B') ON CONFLICT DO NOTHING`, tenantA, tenantB)
	if err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO parent_memberships(tenant_id,subject_ref,role) VALUES ($1,'parent-a','admin'),($2,'parent-b','admin') ON CONFLICT DO NOTHING`, tenantA, tenantB)
	if err != nil {
		t.Fatal(err)
	}
	alice, bob = uuid.NewString(), uuid.NewString()
	_, err = pool.Exec(ctx, `INSERT INTO students(id,tenant_id,display_name) VALUES ($1,$3,'Alice'),($2,$4,'Bob')`, alice, bob, tenantA, tenantB)
	if err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO bff_sessions(handle_hash,tenant_id,subject_ref,session_kind,expires_at) VALUES ($1,$2,'parent-a','parent',now()+interval '1 hour') ON CONFLICT (handle_hash) DO UPDATE SET tenant_id=EXCLUDED.tenant_id,subject_ref=EXCLUDED.subject_ref,session_kind=EXCLUDED.session_kind,expires_at=EXCLUDED.expires_at,revoked_at=NULL`, hash("parent-a"), tenantA)
	if err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO bff_sessions(handle_hash,tenant_id,subject_ref,session_kind,expires_at) VALUES ($1,$2,'parent-b','parent',now()+interval '1 hour') ON CONFLICT (handle_hash) DO UPDATE SET tenant_id=EXCLUDED.tenant_id,subject_ref=EXCLUDED.subject_ref,session_kind=EXCLUDED.session_kind,expires_at=EXCLUDED.expires_at,revoked_at=NULL`, hash("parent-b"), tenantB)
	if err != nil {
		t.Fatal(err)
	}
	return alice, bob
}

func requestJSON(t *testing.T, h http.Handler, method, path, cookie, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if cookie != "" {
		req.AddCookie(&http.Cookie{Name: "tasks_parent", Value: cookie})
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func requestBearer(t *testing.T, h http.Handler, method, path, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func pairingResponse(t *testing.T, rec *httptest.ResponseRecorder) (code string, pairingID string) {
	t.Helper()
	var v struct {
		Code      string    `json:"code"`
		PairingID uuid.UUID `json:"pairingId"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatal(err)
	}
	return v.Code, v.PairingID.String()
}

func TestPostgresPairingReplayTenantAndArchiveBoundaries(t *testing.T) {
	pool := integrationPool(t)
	alice, bob := seedIntegration(t, pool)
	s := NewWithAuth(pool, "test", AuthConfig{SessionSecret: []byte("integration state secret"), IssuerSecret: []byte("integration issuer secret")})
	h := s.Routes()

	// Public routes remain available while every tenant-bound route requires a session.
	for _, tc := range []struct {
		method, path string
		want         int
	}{
		{http.MethodGet, "/health", http.StatusOK},
		{http.MethodGet, "/auth/callback", http.StatusBadRequest},
		{http.MethodGet, "/students", http.StatusUnauthorized},
		{http.MethodGet, "/student/profile", http.StatusUnauthorized},
		{http.MethodGet, "/device/profile", http.StatusUnauthorized},
		{http.MethodGet, "/device/checklist", http.StatusUnauthorized},
		{http.MethodPost, "/health", http.StatusMethodNotAllowed},
	} {
		rec := requestJSON(t, h, tc.method, tc.path, "", "")
		if rec.Code != tc.want {
			t.Fatalf("%s %s = %d, want %d", tc.method, tc.path, rec.Code, tc.want)
		}
	}

	// Parent A can see only Alice; a cross-tenant object is indistinguishable from missing.
	if rec := requestJSON(t, h, http.MethodGet, "/students", "parent-a", ""); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), alice) || strings.Contains(rec.Body.String(), bob) {
		t.Fatalf("tenant-scoped list = (%d, %s)", rec.Code, rec.Body.String())
	}
	if rec := requestJSON(t, h, http.MethodGet, "/students?q=Ali&limit=0&offset=-1", "parent-a", ""); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), alice) {
		t.Fatalf("filtered student list = (%d, %s)", rec.Code, rec.Body.String())
	}
	if rec := requestJSON(t, h, http.MethodGet, "/students/"+alice, "parent-a", ""); rec.Code != http.StatusOK {
		t.Fatalf("student get = %d, want 200", rec.Code)
	}
	if rec := requestJSON(t, h, http.MethodGet, "/students/"+bob, "parent-a", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("cross-tenant get = %d, want 404", rec.Code)
	}
	if rec := requestJSON(t, h, http.MethodPatch, "/students/"+bob, "parent-a", `{"displayName":"Nope"}`); rec.Code != http.StatusNotFound {
		t.Fatalf("cross-tenant update = %d, want 404", rec.Code)
	}
	if rec := requestJSON(t, h, http.MethodPost, "/students/"+bob+"/pairing", "parent-a", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("cross-tenant pairing = %d, want 404", rec.Code)
	}

	// Invalid and valid CRUD requests exercise the same route boundary used by the SPA.
	if rec := requestJSON(t, h, http.MethodPost, "/students", "parent-a", `{}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("empty student create = %d, want 400", rec.Code)
	}
	if rec := requestJSON(t, h, http.MethodPost, "/students", "parent-a", `not-json`); rec.Code != http.StatusBadRequest {
		t.Fatalf("malformed student create = %d, want 400", rec.Code)
	}
	if rec := requestJSON(t, h, http.MethodPost, "/students", "parent-a", `{"displayName":"Charlie"}`); rec.Code != http.StatusCreated {
		t.Fatalf("student create = %d, want 201", rec.Code)
	}
	if rec := requestJSON(t, h, http.MethodPost, "/students", "parent-a", `{"displayName":"charlie"}`); rec.Code != http.StatusConflict {
		t.Fatalf("duplicate student create = %d, want 409", rec.Code)
	}
	if rec := requestJSON(t, h, http.MethodPatch, "/students/"+alice, "parent-a", `{}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("empty student update = %d, want 400", rec.Code)
	}
	if rec := requestJSON(t, h, http.MethodPatch, "/students/"+alice, "parent-a", `{"displayName":"Alice Updated"}`); rec.Code != http.StatusOK {
		t.Fatalf("student update = %d, want 200", rec.Code)
	}

	// Issuing a pairing persists a code, and claiming it is transactional and single-use.
	pairRec := requestJSON(t, h, http.MethodPost, "/students/"+alice+"/pairing", "parent-a", "")
	if pairRec.Code != http.StatusOK {
		t.Fatalf("issue pairing = %d: %s", pairRec.Code, pairRec.Body.String())
	}
	code, _ := pairingResponse(t, pairRec)
	if len(code) != 32 {
		t.Fatalf("pairing code length = %d, want 32 hex chars", len(code))
	}
	for _, r := range code {
		if (r < '0' || r > '9') && (r < 'A' || r > 'F') {
			t.Fatalf("pairing code is not uppercase hex: %q", code)
		}
	}
	claim := requestJSON(t, h, http.MethodPost, "/student/pair", "", `{"code":"`+code+`"}`)
	if claim.Code != http.StatusOK {
		t.Fatalf("browser pair = %d: %s", claim.Code, claim.Body.String())
	}
	studentCookie := claim.Result().Cookies()[0].Value
	if session := requestJSON(t, h, http.MethodGet, "/auth/session", "parent-a", ""); session.Code != http.StatusOK || !strings.Contains(session.Body.String(), tenantA) {
		t.Fatalf("parent session = %d: %s", session.Code, session.Body.String())
	}
	checkReq := httptest.NewRequest(http.MethodGet, "/student/checklist", nil)
	checkReq.AddCookie(&http.Cookie{Name: "tasks_student", Value: studentCookie})
	checkRec := httptest.NewRecorder()
	h.ServeHTTP(checkRec, checkReq)
	if checkRec.Code != http.StatusOK {
		t.Fatalf("student checklist = %d", checkRec.Code)
	}
	if replay := requestJSON(t, h, http.MethodPost, "/student/pair", "", `{"code":"`+code+`"}`); replay.Code != http.StatusGone {
		t.Fatalf("pairing replay = %d, want 410", replay.Code)
	}
	profileReq := httptest.NewRequest(http.MethodGet, "/student/profile", nil)
	profileReq.AddCookie(&http.Cookie{Name: "tasks_student", Value: studentCookie})
	profileRec := httptest.NewRecorder()
	h.ServeHTTP(profileRec, profileReq)
	if profileRec.Code != http.StatusOK || !strings.Contains(profileRec.Body.String(), alice) {
		t.Fatalf("student profile = %d: %s", profileRec.Code, profileRec.Body.String())
	}
	deviceCodeForProfile, _ := pairingResponse(t, requestJSON(t, h, http.MethodPost, "/students/"+alice+"/pairing", "parent-a", ""))
	deviceRecForPair := requestJSON(t, h, http.MethodPost, "/device/pair", "", `{"code":"`+deviceCodeForProfile+`"}`)
	var profileDevice struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(deviceRecForPair.Body.Bytes(), &profileDevice); err != nil {
		t.Fatal(err)
	}
	deviceProfileRec := requestBearer(t, h, http.MethodGet, "/device/profile", profileDevice.Token)
	if deviceProfileRec.Code != http.StatusOK || !strings.Contains(deviceProfileRec.Body.String(), alice) {
		t.Fatalf("device profile = %d: %s", deviceProfileRec.Code, deviceProfileRec.Body.String())
	}
	deviceChecklistRec := requestBearer(t, h, http.MethodGet, "/device/checklist", profileDevice.Token)
	var deviceChecklist Checklist
	if err := json.Unmarshal(deviceChecklistRec.Body.Bytes(), &deviceChecklist); err != nil || deviceChecklistRec.Code != http.StatusOK || len(deviceChecklist.Items) != 0 {
		t.Fatalf("device checklist = %d: %s", deviceChecklistRec.Code, deviceChecklistRec.Body.String())
	}

	// Browser and device credentials are intentionally not interchangeable.
	if rec := requestBearer(t, h, http.MethodGet, "/student/profile", profileDevice.Token); rec.Code != http.StatusUnauthorized {
		t.Fatalf("bearer on student profile = %d, want 401", rec.Code)
	}
	if rec := requestBearer(t, h, http.MethodGet, "/student/checklist", profileDevice.Token); rec.Code != http.StatusUnauthorized {
		t.Fatalf("bearer on student checklist = %d, want 401", rec.Code)
	}
	for _, path := range []string{"/device/profile", "/device/checklist"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.AddCookie(&http.Cookie{Name: "tasks_student", Value: studentCookie})
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("browser cookie on %s = %d, want 401", path, rec.Code)
		}
	}
	// Two simultaneous claims still produce exactly one credential, proving the
	// UPDATE ... claimed_at transaction is the replay boundary rather than a check-then-set race.
	pairRec = requestJSON(t, h, http.MethodPost, "/students/"+alice+"/pairing", "parent-a", "")
	code, _ = pairingResponse(t, pairRec)
	var wg sync.WaitGroup
	results := make(chan int, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- requestJSON(t, h, http.MethodPost, "/device/pair", "", `{"code":"`+code+`"}`).Code
		}()
	}
	wg.Wait()
	close(results)
	var ok, gone int
	for status := range results {
		if status == http.StatusOK {
			ok++
		} else if status == http.StatusGone {
			gone++
		}
	}
	if ok != 1 || gone != 1 {
		t.Fatalf("concurrent pairing statuses = ok:%d gone:%d", ok, gone)
	}
	if logout := requestJSON(t, h, http.MethodPost, "/auth/logout", "parent-a", ""); logout.Code != http.StatusOK {
		t.Fatalf("logout = %d", logout.Code)
	}
	if afterLogout := requestJSON(t, h, http.MethodGet, "/auth/session", "parent-a", ""); afterLogout.Code != http.StatusUnauthorized {
		t.Fatalf("session after logout = %d, want 401", afterLogout.Code)
	}

	// Parent B's student is archived in one transaction; every credential and
	// unclaimed code for that student is revoked, and subsequent access is denied.
	issue := func(path string) string {
		rec := requestJSON(t, h, http.MethodPost, path, "parent-b", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("issue %s = %d: %s", path, rec.Code, rec.Body.String())
		}
		code, _ := pairingResponse(t, rec)
		return code
	}
	browserCode, deviceCode := issue("/students/"+bob+"/pairing"), issue("/students/"+bob+"/pairing")
	_ = issue("/students/" + bob + "/pairing") // an unclaimed code must be revoked by archival
	browser := requestJSON(t, h, http.MethodPost, "/student/pair", "", `{"code":"`+browserCode+`"}`)
	device := requestJSON(t, h, http.MethodPost, "/device/pair", "", `{"code":"`+deviceCode+`"}`)
	if browser.Code != http.StatusOK || device.Code != http.StatusOK {
		t.Fatalf("pre-archive credentials = browser:%d device:%d", browser.Code, device.Code)
	}
	deviceToken := struct {
		Token string `json:"token"`
	}{}
	if err := json.Unmarshal(device.Body.Bytes(), &deviceToken); err != nil {
		t.Fatal(err)
	}
	archive := requestJSON(t, h, http.MethodDelete, "/students/"+bob, "parent-b", "")
	if archive.Code != http.StatusNoContent {
		t.Fatalf("archive = %d: %s", archive.Code, archive.Body.String())
	}
	var archived, deviceRevoked, sessionRevoked, codeRevoked int
	err := pool.QueryRow(context.Background(), `SELECT count(*) FROM students WHERE id=$1 AND archived_at IS NOT NULL`, bob).Scan(&archived)
	if err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{
		`SELECT count(*) FROM student_devices WHERE student_id=$1 AND revoked_at IS NOT NULL`,
		`SELECT count(*) FROM student_sessions WHERE student_id=$1 AND revoked_at IS NOT NULL`,
		`SELECT count(*) FROM pairing_codes WHERE student_id=$1 AND revoked_at IS NOT NULL`,
	} {
		var n int
		if err := pool.QueryRow(context.Background(), query, bob).Scan(&n); err != nil {
			t.Fatal(err)
		}
		switch query {
		case `SELECT count(*) FROM student_devices WHERE student_id=$1 AND revoked_at IS NOT NULL`:
			deviceRevoked = n
		case `SELECT count(*) FROM student_sessions WHERE student_id=$1 AND revoked_at IS NOT NULL`:
			sessionRevoked = n
		default:
			codeRevoked = n
		}
	}
	if archived != 1 || deviceRevoked != 1 || sessionRevoked != 1 || codeRevoked != 1 {
		t.Fatalf("archive revocations = archived:%d device:%d sessions:%d codes:%d", archived, deviceRevoked, sessionRevoked, codeRevoked)
	}
	if rec := requestJSON(t, h, http.MethodPost, "/students/"+bob+"/pairing", "parent-b", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("pair archived student = %d, want 404", rec.Code)
	}
	if rec := requestJSON(t, h, http.MethodDelete, "/students/"+bob, "parent-b", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("archive archived student = %d, want 404", rec.Code)
	}
	deviceReq := httptest.NewRequest(http.MethodGet, "/device/profile", nil)
	deviceReq.Header.Set("Authorization", "Bearer "+deviceToken.Token)
	deviceRec := httptest.NewRecorder()
	h.ServeHTTP(deviceRec, deviceReq)
	if deviceRec.Code != http.StatusUnauthorized {
		t.Fatalf("revoked device profile = %d, want 401", deviceRec.Code)
	}
}

func signedIntegrationToken(secret, issuer, audience, subject string, exp int64) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	payload, _ := json.Marshal(map[string]any{"iss": issuer, "aud": audience, "sub": subject, "exp": exp})
	body := base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(header + "." + body))
	return header + "." + body + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func TestPostgresAuthorizationStateAndCallbackReplay(t *testing.T) {
	pool := integrationPool(t)
	_, _ = seedIntegration(t, pool)
	const secret = "callback issuer secret"
	var issuer *httptest.Server
	issuer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/oauth/token" {
			http.NotFound(w, r)
			return
		}
		if err := r.ParseForm(); err != nil || r.Form.Get("code") != "good-code" || r.Form.Get("code_verifier") == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"id_token":%q}`, signedIntegrationToken(secret, issuer.URL, "tasks", "parent-a", time.Now().Add(time.Minute).Unix()))
	}))
	defer issuer.Close()
	s := NewWithAuth(pool, "test", AuthConfig{Mode: "test", IssuerURL: issuer.URL, PublicIssuerURL: issuer.URL, ClientID: "tasks", RedirectURL: "https://tasks.test/auth/callback", SessionSecret: []byte("callback state secret"), IssuerSecret: []byte(secret)})
	h := s.Routes()

	login := httptest.NewRecorder()
	loginRequest := httptest.NewRequest(http.MethodGet, "/auth/login?principal=parent-a&return_to=https://evil.example", nil)
	loginRequest.Host = "127.0.0.1:39077"
	h.ServeHTTP(login, loginRequest)
	if login.Code != http.StatusFound || !strings.HasPrefix(login.Header().Get("Location"), "http://127.0.0.1:39077/issuer/oauth/authorize?") {
		t.Fatalf("login = %d %s", login.Code, login.Header().Get("Location"))
	}
	fqdn := httptest.NewRecorder()
	fqdnReq := httptest.NewRequest(http.MethodGet, "/auth/login?principal=parent-a", nil)
	fqdnReq.Header.Set("X-Forwarded-Host", "web.household.primer-tasks.test")
	fqdnReq.Header.Set("X-Forwarded-Proto", "https")
	h.ServeHTTP(fqdn, fqdnReq)
	if fqdn.Code != http.StatusFound || !strings.HasPrefix(fqdn.Header().Get("Location"), "https://web.household.primer-tasks.test/issuer/oauth/authorize?") {
		t.Fatalf("stacklane login = %d %s", fqdn.Code, fqdn.Header().Get("Location"))
	}
	stateCookie := login.Result().Cookies()[0]
	state := stateCookie.Value
	if state == "" {
		t.Fatal("login did not bind state cookie")
	}
	missingCode := httptest.NewRequest(http.MethodGet, "/auth/callback?state="+state, nil)
	missingCode.AddCookie(stateCookie)
	missingCodeRec := httptest.NewRecorder()
	h.ServeHTTP(missingCodeRec, missingCode)
	if missingCodeRec.Code != http.StatusBadRequest {
		t.Fatalf("callback without code = %d, want 400", missingCodeRec.Code)
	}
	callback := httptest.NewRequest(http.MethodGet, "/auth/callback?code=good-code&state="+state, nil)
	callback.AddCookie(stateCookie)
	callbackRec := httptest.NewRecorder()
	h.ServeHTTP(callbackRec, callback)
	if callbackRec.Code != http.StatusFound || callbackRec.Header().Get("Location") != "/parent/students" {
		t.Fatalf("callback = %d %s", callbackRec.Code, callbackRec.Header().Get("Location"))
	}
	if replay := httptest.NewRecorder(); func() int {
		h.ServeHTTP(replay, callback)
		return replay.Code
	}() != http.StatusBadRequest {
		t.Fatalf("callback replay = %d, want 400", replay.Code)
	}
	if mismatch := requestJSON(t, h, http.MethodGet, "/auth/callback?code=good-code&state=wrong", "", ""); mismatch.Code != http.StatusBadRequest {
		t.Fatalf("state mismatch = %d, want 400", mismatch.Code)
	}
	if denied := requestJSON(t, h, http.MethodGet, "/auth/callback?error=access_denied", "", ""); denied.Code != http.StatusUnauthorized {
		t.Fatalf("provider denial = %d, want 401", denied.Code)
	}
}

func TestPostgresStudentSessionCannotSatisfyParentGuard(t *testing.T) {
	pool := integrationPool(t)
	alice, _ := seedIntegration(t, pool)
	s := NewWithAuth(pool, "test", AuthConfig{SessionSecret: []byte("student-as-parent secret"), IssuerSecret: []byte("student-as-parent issuer")})
	h := s.Routes()
	pairRec := requestJSON(t, h, http.MethodPost, "/students/"+alice+"/pairing", "parent-a", "")
	code, _ := pairingResponse(t, pairRec)
	claim := requestJSON(t, h, http.MethodPost, "/student/pair", "", `{"code":"`+code+`"}`)
	if claim.Code != http.StatusOK {
		t.Fatalf("browser pair = %d: %s", claim.Code, claim.Body.String())
	}
	studentHandle := ""
	for _, c := range claim.Result().Cookies() {
		if c.Name == "tasks_student" {
			studentHandle = c.Value
		}
	}
	if studentHandle == "" {
		t.Fatal("student pairing did not issue a BFF cookie")
	}
	var kind string
	if err := pool.QueryRow(context.Background(), `SELECT session_kind FROM bff_sessions WHERE handle_hash=$1`, hash(studentHandle)).Scan(&kind); err != nil {
		t.Fatal(err)
	}
	if kind != "student" {
		t.Fatalf("redeemed session_kind = %q, want student", kind)
	}
	for _, tc := range []struct {
		method, path, body string
	}{
		{http.MethodGet, "/auth/session", ""},
		{http.MethodGet, "/students", ""},
		{http.MethodGet, "/students/" + alice, ""},
		{http.MethodPost, "/students", `{"displayName":"Should Not Exist"}`},
		{http.MethodPatch, "/students/" + alice, `{"displayName":"Hijacked"}`},
		{http.MethodPost, "/students/" + alice + "/pairing", ""},
		{http.MethodDelete, "/students/" + alice, ""},
	} {
		rec := requestJSON(t, h, tc.method, tc.path, studentHandle, tc.body)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s with student handle as tasks_parent = %d, want 401", tc.method, tc.path, rec.Code)
		}
	}
	var mutated int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM students WHERE display_name IN ('Should Not Exist','Hijacked')`).Scan(&mutated); err != nil {
		t.Fatal(err)
	}
	if mutated != 0 {
		t.Fatalf("student-as-parent mutated %d student rows", mutated)
	}
}

func TestPostgresPairingGuessThrottleAndEntropy(t *testing.T) {
	pool := integrationPool(t)
	alice, _ := seedIntegration(t, pool)
	s := NewWithAuth(pool, "test", AuthConfig{SessionSecret: []byte("guess-secret"), IssuerSecret: []byte("guess-issuer")})
	h := s.Routes()
	pairRec := requestJSON(t, h, http.MethodPost, "/students/"+alice+"/pairing", "parent-a", "")
	code, _ := pairingResponse(t, pairRec)
	if len(code) != 32 {
		t.Fatalf("pairing entropy length = %d", len(code))
	}
	guess := func(remote, body string) int {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/student/pair", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Forwarded-For", "198.51.100.1")
		req.RemoteAddr = remote
		h.ServeHTTP(rec, req)
		return rec.Code
	}
	for i := 0; i < pairingGuessLimit-1; i++ {
		if status := guess("203.0.113.10:54321", `{"code":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}`); status != http.StatusGone {
			t.Fatalf("failed guess %d = %d, want 410", i, status)
		}
	}
	if status := guess("203.0.113.10:54321", `{"code":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}`); status != http.StatusTooManyRequests {
		t.Fatalf("fifth failed guess = %d, want 429", status)
	}
	if status := guess("203.0.113.10:54321", `{"code":"`+code+`"}`); status != http.StatusTooManyRequests {
		t.Fatalf("spoofed XFF still throttled = %d, want 429", status)
	}
	var n int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM pairing_guess_attempts WHERE client_key='203.0.113.10'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != pairingGuessLimit {
		t.Fatalf("recorded guesses = %d, want %d", n, pairingGuessLimit)
	}
	if status := guess("198.51.100.20:12345", `{"code":"`+code+`"}`); status != http.StatusOK {
		t.Fatalf("unrelated client claim = %d", status)
	}
}

func TestPostgresPairingGuessParallelWindow(t *testing.T) {
	pool := integrationPool(t)
	alice, _ := seedIntegration(t, pool)
	s := NewWithAuth(pool, "test", AuthConfig{SessionSecret: []byte("parallel-guess"), IssuerSecret: []byte("parallel-guess")})
	h := s.Routes()
	pairRec := requestJSON(t, h, http.MethodPost, "/students/"+alice+"/pairing", "parent-a", "")
	code, _ := pairingResponse(t, pairRec)
	var wg sync.WaitGroup
	results := make(chan int, 12)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/student/pair", strings.NewReader(`{"code":"BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB"}`))
			req.Header.Set("Content-Type", "application/json")
			req.RemoteAddr = "192.0.2.55:9"
			h.ServeHTTP(rec, req)
			results <- rec.Code
		}()
	}
	wg.Wait()
	close(results)
	var gone, throttled, other int
	for status := range results {
		switch status {
		case http.StatusGone:
			gone++
		case http.StatusTooManyRequests:
			throttled++
		default:
			other++
		}
	}
	if other != 0 || gone+throttled != 12 || gone > pairingGuessLimit {
		t.Fatalf("parallel guesses gone=%d throttled=%d other=%d", gone, throttled, other)
	}
	var n int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM pairing_guess_attempts WHERE client_key='192.0.2.55'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != pairingGuessLimit {
		t.Fatalf("parallel recorded guesses = %d, want %d", n, pairingGuessLimit)
	}
	claim := httptest.NewRecorder()
	claimReq := httptest.NewRequest(http.MethodPost, "/student/pair", strings.NewReader(`{"code":"`+code+`"}`))
	claimReq.Header.Set("Content-Type", "application/json")
	claimReq.RemoteAddr = "192.0.2.55:9"
	h.ServeHTTP(claim, claimReq)
	if claim.Code != http.StatusTooManyRequests {
		t.Fatalf("claim after parallel window = %d, want 429", claim.Code)
	}
}
