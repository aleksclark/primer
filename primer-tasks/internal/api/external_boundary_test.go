package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestHumaParentRouteRequiresSessionBeforeDatabaseAccess(t *testing.T) {
	server := NewWithStore(nil, "test", nil)
	rec := httptest.NewRecorder()
	server.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin/verifiers", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated Huma parent route status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestExternalCatalogAndCallbackBoundariesFailClosedBeforeDatabaseAccess(t *testing.T) {
	server := &Server{}
	request := httptest.NewRequest(http.MethodGet, "/external/verifiers", nil)
	for _, role := range []string{"student", "", "parent", "admin", "educator"} {
		rec := httptest.NewRecorder()
		server.listExternalVerifiers(rec, request, scope{Role: role})
		if rec.Code != http.StatusForbidden {
			t.Fatalf("list role=%q status=%d", role, rec.Code)
		}
	}
	for _, role := range []string{"student", "parent", "admin", "educator"} {
		rec := httptest.NewRecorder()
		server.createExternalVerifier(rec, httptest.NewRequest(http.MethodPost, "/external/verifiers", strings.NewReader(`{}`)), scope{Role: role})
		if rec.Code != http.StatusForbidden {
			t.Fatalf("create role=%q status=%d", role, rec.Code)
		}
	}
	malformedCreate := httptest.NewRecorder()
	server.createExternalVerifier(malformedCreate, httptest.NewRequest(http.MethodPost, "/external/verifiers", strings.NewReader("not-json")), scope{Role: "product_admin"})
	if malformedCreate.Code != http.StatusBadRequest {
		t.Fatalf("malformed create status=%d", malformedCreate.Code)
	}
	rec := httptest.NewRecorder()
	server.setExternalVerifierActive(rec, httptest.NewRequest(http.MethodPost, "/external/verifiers/not-a-uuid", strings.NewReader(`{"active":true}`)), scope{Role: "product_admin"})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("invalid catalog id status=%d", rec.Code)
	}
	malformedSet := httptest.NewRecorder()
	setRoute := chi.NewRouteContext()
	setRoute.URLParams.Add("id", "00000000-0000-0000-0000-000000000001")
	setReq := httptest.NewRequest(http.MethodPost, "/external/verifiers/1", strings.NewReader("not-json")).WithContext(context.WithValue(context.Background(), chi.RouteCtxKey, setRoute))
	server.setExternalVerifierActive(malformedSet, setReq, scope{Role: "product_admin"})
	if malformedSet.Code != http.StatusBadRequest {
		t.Fatalf("malformed set status=%d", malformedSet.Code)
	}
	rec = httptest.NewRecorder()
	server.externalCallback(rec, httptest.NewRequest(http.MethodPost, "/external/verifiers/id/callback", strings.NewReader(`{}`)))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("missing callback secret status=%d", rec.Code)
	}
	rec = httptest.NewRecorder()
	server.fallbackExternal(rec, httptest.NewRequest(http.MethodPost, "/occurrences/id/external/fallback", strings.NewReader("not-json")), scope{Tenant: "tenant"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("malformed fallback status=%d", rec.Code)
	}
}
