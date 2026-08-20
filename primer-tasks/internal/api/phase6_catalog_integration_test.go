package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"primer-tasks/internal/repo"
	"primer-tasks/internal/verification"
)

func TestPhase6ExternalCatalogRotationAndAttemptSnapshot(t *testing.T) {
	pool := integrationPool(t)
	f := seedPhase6External(t, pool)
	t.Cleanup(func() { cleanupPhase6External(t, pool, f) })
	ctx := context.Background()
	catalog := repo.NewVerifierCatalogRepository(pool)
	got, err := catalog.Get(ctx, f.verifier)
	if err != nil || got.Name != "fixture" || got.SecretVersion != "1" {
		t.Fatalf("catalog=%+v err=%v", got, err)
	}
	if ref, err := catalog.SecretRefForVersion(ctx, f.verifier, "1"); err != nil || ref != "fixture" {
		t.Fatalf("secret ref=%q err=%v", ref, err)
	}
	items, err := catalog.List(ctx, false)
	if err != nil || len(items) != 1 {
		t.Fatalf("catalog list=%d err=%v", len(items), err)
	}
	if err := catalog.RotateSecret(ctx, f.verifier, "fixture-old", "0"); err != nil {
		t.Fatal(err)
	}
	if ref, err := catalog.SecretRefForVersion(ctx, f.verifier, "0"); err != nil || ref != "fixture-old" {
		t.Fatalf("rotated secret ref=%q err=%v", ref, err)
	}
	if err := catalog.SetActive(ctx, f.verifier, false); err != nil {
		t.Fatal(err)
	}
	server := NewWithStore(pool, "test", nil)
	catalogParent := "catalog-parent-" + uuid.NewString()
	catalogCookie := "catalog-cookie-" + uuid.NewString()
	if _, err := pool.Exec(ctx, `INSERT INTO parent_memberships(tenant_id,subject_ref,role) VALUES($1,$2,'admin')`, f.tenant, catalogParent); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO bff_sessions(handle_hash,tenant_id,subject_ref,session_kind,expires_at) VALUES($1,$2,$3,'parent',now()+interval '1 hour')`, hash(catalogCookie), f.tenant, catalogParent); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM bff_sessions WHERE handle_hash=$1`, hash(catalogCookie))
		_, _ = pool.Exec(context.Background(), `DELETE FROM parent_memberships WHERE tenant_id=$1 AND subject_ref=$2`, f.tenant, catalogParent)
	})
	catalogHTTP := requestJSON(t, server.Routes(), http.MethodGet, "/admin/verifiers", catalogCookie, "")
	if catalogHTTP.Code != http.StatusOK || !strings.Contains(catalogHTTP.Body.String(), `"name":"fixture"`) || strings.Contains(catalogHTTP.Body.String(), "endpointUrl") || strings.Contains(catalogHTTP.Body.String(), "secretRef") {
		t.Fatalf("ordinary parent catalog read status=%d body=%s", catalogHTTP.Code, catalogHTTP.Body)
	}
	config := map[string]any{"verifierId": f.verifier.String(), "capability": "response", "schemaVersion": verification.ExternalCallbackSchemaVersion}
	if err := server.validateExternalConfig(ctx, config); err == nil {
		t.Fatal("disabled verifier validated")
	}
	listRec := httptest.NewRecorder()
	server.listExternalVerifiers(listRec, httptest.NewRequest(http.MethodGet, "/external/verifiers", nil), scope{Role: "product_admin"})
	if listRec.Code != http.StatusOK || !strings.Contains(listRec.Body.String(), "fixture") {
		t.Fatalf("catalog list status=%d body=%s", listRec.Code, listRec.Body.String())
	}
	createBody := `{"name":"catalog test verifier","endpointUrl":"http://external-verifier-fixture:8092/v1/verify","active":true,"schemaVersions":["external_callback.v1"],"capabilities":["response"],"secretRef":"fixture","secretVersion":"1","egressPolicy":{"testFixture":true}}`
	createRec := httptest.NewRecorder()
	server.createExternalVerifier(createRec, httptest.NewRequest(http.MethodPost, "/external/verifiers", strings.NewReader(createBody)), scope{Role: "product_admin"})
	if createRec.Code != http.StatusCreated {
		t.Fatalf("catalog create status=%d body=%s", createRec.Code, createRec.Body.String())
	}
	var created externalVerifierOutput
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM external_verifier_secret_versions WHERE verifier_id=$1`, created.ID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM external_verifier_catalog WHERE id=$1`, created.ID)
	})
	setRoute := chi.NewRouteContext()
	setRoute.URLParams.Add("id", f.verifier.String())
	setReq := httptest.NewRequest(http.MethodPost, "/external/verifiers/"+f.verifier.String(), strings.NewReader(`{"active":true}`)).WithContext(context.WithValue(ctx, chi.RouteCtxKey, setRoute))
	setRec := httptest.NewRecorder()
	server.setExternalVerifierActive(setRec, setReq, scope{Role: "product_admin"})
	if setRec.Code != http.StatusOK {
		t.Fatalf("catalog activation status=%d body=%s", setRec.Code, setRec.Body.String())
	}
	if err := server.validateExternalConfig(ctx, config); err != nil {
		t.Fatal(err)
	}
	unsupportedSchema := map[string]any{"verifierId": f.verifier.String(), "capability": "response", "schemaVersion": "unsupported"}
	if err := server.validateExternalConfig(ctx, unsupportedSchema); err == nil {
		t.Fatal("unsupported external schema accepted")
	}
	unsupportedCapability := map[string]any{"verifierId": f.verifier.String(), "capability": "private", "schemaVersion": verification.ExternalCallbackSchemaVersion}
	if err := server.validateExternalConfig(ctx, unsupportedCapability); err == nil {
		t.Fatal("unsupported external capability accepted")
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := snapshotExternalAttempt(ctx, tx, f.tenant.String(), f.attempt.String(), f.requirement.String(), verification.ExternalConfig{VerifierID: f.verifier.String(), Capability: "response", Schema: verification.ExternalCallbackSchemaVersion, Options: map[string]any{"public": "ok"}}); err != nil {
		tx.Rollback(ctx)
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	seedPhase6ExternalDelivery(t, pool, f)
	health := server.externalVerifierHealth(ctx)
	if health == nil || health.Attempts < 0 {
		t.Fatalf("external verifier health=%+v", health)
	}
	fallbackRoute := chi.NewRouteContext()
	fallbackRoute.URLParams.Add("id", f.occurrence.String())
	fallbackReq := httptest.NewRequest(http.MethodPost, "/occurrences/"+f.occurrence.String()+"/external/fallback", strings.NewReader(`{"accepted":false,"reason":"parent review"}`)).WithContext(context.WithValue(ctx, chi.RouteCtxKey, fallbackRoute))
	fallbackRec := httptest.NewRecorder()
	server.fallbackExternal(fallbackRec, fallbackReq, scope{Tenant: f.tenant.String()})
	if fallbackRec.Code != http.StatusOK {
		t.Fatalf("fallback status=%d body=%s", fallbackRec.Code, fallbackRec.Body.String())
	}
	var version string
	if err := pool.QueryRow(ctx, `SELECT secret_version FROM external_verifier_attempts WHERE tenant_id=$1 AND attempt_id=$2`, f.tenant, f.attempt).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != "0" {
		t.Fatalf("snapshot secret version=%q", version)
	}
	rotateRoute := chi.NewRouteContext()
	rotateRoute.URLParams.Add("id", f.verifier.String())
	rotateReq := httptest.NewRequest(http.MethodPost, "/external/verifiers/"+f.verifier.String(), strings.NewReader(`{"active":true,"secretRef":"fixture-next","secretVersion":"2"}`)).WithContext(context.WithValue(ctx, chi.RouteCtxKey, rotateRoute))
	rotateRec := httptest.NewRecorder()
	server.setExternalVerifierActive(rotateRec, rotateReq, scope{Role: "product_admin"})
	if rotateRec.Code != http.StatusOK {
		t.Fatalf("catalog rotation status=%d body=%s", rotateRec.Code, rotateRec.Body.String())
	}
}
