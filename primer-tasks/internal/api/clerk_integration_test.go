package api

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"git.clark.team/aleksclark/authstack/auth"
	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	tasksdb "primer-tasks/internal/db"
	"primer-tasks/internal/parentauth"
)

func TestClerkReleaseParentAndUnchangedStudentBoundary(t *testing.T) {
	pool := integrationPool(t)
	alice, bob := seedIntegration(t, pool)
	ctx := context.Background()
	const issuer = "https://clerk.example"
	const origin = "https://api.primerlms.com"
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	jwks := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &key.PublicKey, KeyID: "release-test", Algorithm: "RS256", Use: "sig"}}})
	}))
	defer jwks.Close()
	verifier, err := parentauth.New(ctx, issuer, jwks.URL)
	if err != nil {
		t.Fatal(err)
	}
	sign := func(changes map[string]any) string {
		t.Helper()
		signer, e := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: jose.JSONWebKey{Key: key, KeyID: "release-test"}}, nil)
		if e != nil {
			t.Fatal(e)
		}
		claims := map[string]any{"iss": issuer, "sub": "clerk-parent-a", "sid": "session-a", "azp": origin, "aud": "tasks", "exp": time.Now().Add(time.Minute).Unix(), "nbf": time.Now().Add(-time.Minute).Unix()}
		for k, v := range changes {
			claims[k] = v
		}
		tok, e := jwt.Signed(signer).Claims(claims).Serialize()
		if e != nil {
			t.Fatal(e)
		}
		return tok
	}
	for _, link := range []tasksdb.ParentLink{{Issuer: issuer, Subject: "clerk-parent-a", TenantID: tenantA, ActorRef: "parent-a"}, {Issuer: issuer, Subject: "clerk-parent-b", TenantID: tenantB, ActorRef: "parent-b"}} {
		if err := tasksdb.BootstrapParent(ctx, pool, link); err != nil {
			t.Fatal(err)
		}
		if err := tasksdb.BootstrapParent(ctx, pool, link); err != nil {
			t.Fatal("idempotent bootstrap", err)
		}
	}
	if err := tasksdb.BootstrapParent(ctx, pool, tasksdb.ParentLink{Issuer: issuer, Subject: "clerk-parent-a", TenantID: tenantB, ActorRef: "parent-b"}); err == nil {
		t.Fatal("bootstrap rebound identity")
	}
	s := NewWithAuth(pool, "production", AuthConfig{Mode: "clerk", PublicOrigin: origin})
	s.ParentAuthenticator = verifier
	s.ParentPolicy = auth.AuthenticationPolicy{AcceptedCredentials: []auth.CredentialKind{auth.CredentialSession}, AuthorizedParties: []string{origin}, Audiences: []string{"tasks"}}
	s.BasePath = "/tasks"
	h := Mount(s.Routes(), "/tasks", t.TempDir())
	call := func(method, path, token, body string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, "/tasks/api"+path, strings.NewReader(body))
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		if body != "" {
			r.Header.Set("Content-Type", "application/json")
		}
		for _, c := range cookies {
			r.AddCookie(c)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	a, b := sign(nil), sign(map[string]any{"sub": "clerk-parent-b", "sid": "session-b"})
	for name, token := range map[string]string{"missing": "", "malformed": "invalid", "wrong issuer": sign(map[string]any{"iss": "https://foreign.example"}), "wrong party": sign(map[string]any{"azp": "https://other.example"}), "wrong audience": sign(map[string]any{"aud": "elsewhere"}), "expired": sign(map[string]any{"exp": time.Now().Add(-time.Minute).Unix()}), "future": sign(map[string]any{"nbf": time.Now().Add(time.Minute).Unix()}), "machine": sign(map[string]any{"machine_id": "machine-a"}), "no sid": sign(map[string]any{"sid": ""})} {
		t.Run(name, func(t *testing.T) {
			rec := call("GET", "/students", token, "")
			if rec.Code != 401 || rec.Header().Get("WWW-Authenticate") != "Bearer" {
				t.Fatalf("status=%d", rec.Code)
			}
		})
	}
	if rec := call("GET", "/students", "", "", &http.Cookie{Name: "tasks_parent", Value: "parent-a"}); rec.Code != 401 {
		t.Fatal("legacy cookie accepted")
	}
	if rec := call("GET", "/students", sign(map[string]any{"sub": "not-provisioned"}), ""); rec.Code != 403 {
		t.Fatal("unprovisioned parent accepted")
	}
	if rec := call("GET", "/auth/session", a, ""); rec.Code != 200 || !strings.Contains(rec.Body.String(), `"subjectRef":"parent-a"`) {
		t.Fatal("local actor mapping lost")
	}
	for _, method := range []string{"GET", "PATCH", "DELETE"} {
		body := ""
		if method == "PATCH" {
			body = `{"displayName":"stolen"}`
		}
		if rec := call(method, "/students/"+alice, b, body); rec.Code != 404 {
			t.Fatalf("cross-household %s=%d", method, rec.Code)
		}
	}
	if rec := call("GET", "/students", a, ""); strings.Contains(rec.Body.String(), bob) {
		t.Fatal("household list leak")
	}

	// Existing task -> schedule -> paired browser submission -> parent approval.
	rec := call("POST", "/tasks", a, `{"title":"Release manual approval","instructions":"Observe","requirements":[{"id":"parent-approval","kind":"parent_approval","configVersion":1,"config":{},"interaction":"parent_action","executor":"human"}]}`)
	if rec.Code != 201 {
		t.Fatalf("task=%d %s", rec.Code, rec.Body.String())
	}
	var task struct {
		ID         string `json:"id"`
		TemplateID string `json:"templateId"`
	}
	if json.Unmarshal(rec.Body.Bytes(), &task) != nil {
		t.Fatal("task response")
	}
	if rec = call("POST", "/tasks/"+task.ID+"/publish", a, ""); rec.Code != 200 {
		t.Fatal("publish")
	}
	rec = call("POST", "/schedules", a, `{"studentId":"`+alice+`","templateId":"`+task.TemplateID+`","revisionId":"`+task.ID+`","kind":"one_off","timezone":"UTC","startAt":"`+time.Now().UTC().Add(-time.Minute).Format(time.RFC3339)+`","dueOffsetMinutes":0}`)
	if rec.Code != 201 {
		t.Fatalf("schedule=%d %s", rec.Code, rec.Body.String())
	}
	var page OccurrencePage2
	rec = call("GET", "/occurrences", a, "")
	if json.Unmarshal(rec.Body.Bytes(), &page) != nil || len(page.Items) != 1 {
		t.Fatalf("occurrences=%s", rec.Body.String())
	}
	occ := page.Items[0].ID
	rec = call("POST", "/students/"+alice+"/pairing", a, "")
	code, _ := pairingResponse(t, rec)
	if !strings.Contains(rec.Body.String(), `/tasks/api`) {
		t.Fatal("QR API prefix missing")
	}
	rec = call("POST", "/student/pair", "", `{"code":"`+code+`"}`)
	if rec.Code != 200 {
		t.Fatal("browser pairing")
	}
	// Fresh pairing has exactly one opaque authentication cookie and one
	// independent, non-authorizing CSRF cookie. Never depend on header order.
	cookies := map[string]*http.Cookie{}
	for _, candidate := range rec.Result().Cookies() {
		if candidate.Name != "tasks_student" && candidate.Name != "tasks_csrf" {
			t.Fatal("unexpected fresh-pairing cookie")
		}
		if cookies[candidate.Name] != nil {
			t.Fatal("duplicate fresh-pairing cookie")
		}
		cookies[candidate.Name] = candidate
	}
	cookie, csrf := cookies["tasks_student"], cookies["tasks_csrf"]
	if cookie == nil || csrf == nil || len(cookies) != 2 {
		t.Fatal("fresh pairing cookie set is incomplete")
	}
	if cookie.Name != "tasks_student" || cookie.Path != "/" || !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteLaxMode {
		t.Fatal("student cookie contract changed")
	}
	if cookie.Domain != "" || cookie.Value == "" || cookie.MaxAge != 7776000 {
		t.Fatal("student host-only credential or lifetime changed")
	}
	if csrf.Domain != "" || csrf.Value == "" || csrf.Value == cookie.Value || csrf.Path != "/" || csrf.HttpOnly || !csrf.Secure || csrf.SameSite != http.SameSiteStrictMode || csrf.MaxAge != 28800 {
		t.Fatal("CSRF cookie separation, scope, protection or lifetime is invalid")
	}
	if rec := call("GET", "/student/profile", "", "", csrf); rec.Code != 401 {
		t.Fatal("CSRF cookie authenticated as student")
	}
	if rec := call("GET", "/students", "", "", csrf); rec.Code != 401 {
		t.Fatal("CSRF cookie authenticated as parent")
	}
	rec = call("POST", "/students/"+alice+"/pairing", a, "")
	code, _ = pairingResponse(t, rec)
	rec = call("POST", "/device/pair", "", `{"code":"`+code+`"}`)
	var device DevicePair
	_ = json.Unmarshal(rec.Body.Bytes(), &device)
	if rec = call("GET", "/students", device.Token, ""); rec.Code != 401 {
		t.Fatal("device credential accepted as parent")
	}
	if rec = call("GET", "/device/profile", a, ""); rec.Code != 401 {
		t.Fatal("parent JWT accepted as device")
	}
	for _, action := range []string{"start", "submit"} {
		if rec = call("POST", "/student/occurrences/"+occ+"/"+action, "", "", cookie); rec.Code != 200 {
			t.Fatalf("student %s=%d", action, rec.Code)
		}
	}
	if rec = call("POST", "/occurrences/"+occ+"/decision", b, `{"accepted":true,"reason":"wrong parent"}`); rec.Code != 404 {
		t.Fatal("foreign approval accepted")
	}
	if rec = call("POST", "/occurrences/"+occ+"/decision", a, `{"accepted":true,"reason":"observed"}`); rec.Code != 200 {
		t.Fatal("approval")
	}
	if rec = call("GET", "/occurrences/"+occ, a, ""); !strings.Contains(rec.Body.String(), `"status":"completed"`) {
		t.Fatal("not completed")
	}
	if rec = call("DELETE", "/students/"+alice, a, ""); rec.Code != 204 {
		t.Fatal("archive")
	}
	if rec = call("GET", "/student/profile", "", "", cookie); rec.Code != 401 {
		t.Fatal("student revocation failed")
	}
	if rec = call("GET", "/device/profile", device.Token, ""); rec.Code != 401 {
		t.Fatal("device revocation failed")
	}
	if _, err = pool.Exec(ctx, `UPDATE parent_memberships SET revoked_at=now() WHERE tenant_id=$1`, tenantA); err != nil {
		t.Fatal(err)
	}
	if rec = call("GET", "/students", a, ""); rec.Code != 403 {
		t.Fatal("membership revocation not live")
	}
	if _, err = pool.Exec(ctx, `UPDATE parent_memberships SET revoked_at=NULL WHERE tenant_id=$1`, tenantA); err != nil {
		t.Fatal(err)
	}
	if rec = call("POST", "/auth/logout", a, ""); rec.Code != 200 {
		t.Fatal("logout")
	}
	if rec = call("GET", "/students", a, ""); rec.Code != 403 {
		t.Fatal("logged out sid accepted")
	}
	if rec = call("GET", "/students", sign(map[string]any{"sid": "new-session"}), ""); rec.Code != 200 {
		t.Fatal("new session blocked")
	}
	if _, err = pool.Exec(ctx, `UPDATE parent_identities SET revoked_at=now() WHERE issuer=$1 AND subject='clerk-parent-a'`, issuer); err != nil {
		t.Fatal(err)
	}
	if rec = call("GET", "/students", sign(map[string]any{"sid": "new-session"}), ""); rec.Code != 403 {
		t.Fatal("identity revocation not live")
	}
	if rec = call("GET", "/auth/login?return_to=https://attacker.example", "", ""); rec.Code != 302 || rec.Header().Get("Location") != "/tasks/parent/students" {
		t.Fatal("Clerk login must stay at the Tasks sign-in surface")
	}
	if rec = call("GET", "/auth/callback", "", ""); rec.Code != 410 {
		t.Fatal("legacy callback still active in Clerk mode")
	}
	pool.Close()
	if rec = call("GET", "/students", b, ""); rec.Code != 503 {
		t.Fatal("authorization database outage must fail closed")
	}
	if rec = call("POST", "/auth/logout", b, ""); rec.Code != 503 {
		t.Fatal("logout must not claim revocation when its database is unavailable")
	}
}

func TestTasksMountSingleStripAndSPAFallback(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("release index"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "asset.js"), []byte("release asset"), 0600); err != nil {
		t.Fatal(err)
	}
	h := Mount(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(r.URL.Path)) }), "/tasks", dir)
	for _, tc := range []struct {
		path string
		code int
		body string
	}{{"/tasks", 308, ""}, {"/tasks/", 200, "release index"}, {"/tasks/parent/students", 200, "release index"}, {"/tasks/student", 200, "release index"}, {"/tasks/asset.js", 200, "release asset"}, {"/tasks/missing.js", 404, ""}, {"/tasks/api/tasks", 200, "/tasks"}, {"/tasks/api/student/pair", 200, "/student/pair"}, {"/health", 200, "/health"}, {"/api/v1/students", 404, ""}} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", tc.path, nil))
		if rec.Code != tc.code || (tc.body != "" && rec.Body.String() != tc.body) {
			t.Fatalf("%s=%d %q", tc.path, rec.Code, rec.Body.String())
		}
		if tc.path == "/tasks" && rec.Header().Get("Location") != "/tasks/" {
			t.Fatal("mount redirect")
		}
	}
}
