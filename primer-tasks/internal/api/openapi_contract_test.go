package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestOpenAPIDerivesExactProductionRouteRegistration(t *testing.T) {
	actual := map[string]struct{}{}
	if err := chi.Walk(New(nil, "test").router(), func(method, path string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		actual[strings.ToLower(method)+" "+path] = struct{}{}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	first, second := OpenAPI(), OpenAPI()
	if first != second {
		t.Fatal("offline OpenAPI emission is not deterministic")
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(first), &doc); err != nil {
		t.Fatal(err)
	}
	documented := map[string]struct{}{}
	for path, operations := range doc["paths"].(map[string]any) {
		for method := range operations.(map[string]any) {
			documented[method+" "+path] = struct{}{}
		}
	}
	if !reflect.DeepEqual(documented, actual) {
		t.Fatalf("OpenAPI routes differ from production registration:\nOpenAPI: %#v\nrouter: %#v", documented, actual)
	}

	r := New(nil, "test").Routes()
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/openapi.yaml", nil))
	if rec.Code != http.StatusOK || rec.Body.String() != first {
		t.Fatalf("served contract = (%d, %q), want (200, generated document)", rec.Code, rec.Body.String())
	}

	students := doc["paths"].(map[string]any)["/students"].(map[string]any)
	if _, ok := students["post"].(map[string]any)["responses"].(map[string]any)["201"]; !ok {
		t.Fatal("create student contract must declare the production 201 response")
	}
	student := doc["paths"].(map[string]any)["/students/{id}"].(map[string]any)
	if _, ok := student["delete"].(map[string]any)["responses"].(map[string]any)["204"]; !ok {
		t.Fatal("archive student contract must declare the production 204 response")
	}
}
