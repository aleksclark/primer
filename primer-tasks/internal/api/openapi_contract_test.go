package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/danielgtaylor/huma/v2"
)

func TestTypedBridgeErrorPaths(t *testing.T) {
	problem := &Problem{Detail: "detail"}
	if problem.Error() != "detail" || problem.ContentType("application/problem+json") != "application/json" {
		t.Fatal("problem error contract is not stable")
	}
	if OpenAPIJSON() == "" {
		t.Fatal("JSON OpenAPI emission is empty")
	}
	if _, err := legacyResponse(context.Background(), http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), nil); err == nil {
		t.Fatal("legacy response without a Huma context did not fail")
	}
	fallback := capturedError(&capturedResponse{header: make(http.Header), status: http.StatusBadGateway})
	if fallback.(*Problem).Code != "internal" || fallback.(*Problem).GetStatus() != http.StatusBadGateway {
		t.Fatalf("fallback problem = %#v", fallback)
	}

	a := New(nil, "openapi").humaAPI()
	huma.Register(a, huma.Operation{OperationID: "test-invalid-output", Method: http.MethodGet, Path: "/test-invalid-output"}, func(ctx context.Context, _ *struct{}) (*HealthOutput, error) {
		body, headers, err := legacyJSON[Health](ctx, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("not-json")) }), nil)
		return &HealthOutput{ResponseHeaders: headers, Body: body}, err
	})
	huma.Register(a, huma.Operation{OperationID: "test-unencodable-input", Method: http.MethodGet, Path: "/test-unencodable-input"}, func(ctx context.Context, _ *struct{}) (*HealthOutput, error) {
		_, err := legacyResponse(ctx, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), func() {})
		return &HealthOutput{}, err
	})
	for _, path := range []string{"/test-invalid-output", "/test-unencodable-input"} {
		rec := httptest.NewRecorder()
		a.Adapter().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("%s = %d, want 500", path, rec.Code)
		}
	}

	response := &capturedResponse{header: make(http.Header)}
	_, _ = response.Write([]byte("ok"))
	response.WriteHeader(http.StatusBadRequest)
	if response.status != http.StatusOK || response.body.String() != "ok" {
		t.Fatalf("captured response = %#v", response)
	}
}

func TestTypedBoundarySchemasAndStatuses(t *testing.T) {
	paths := New(nil, "openapi").humaAPI().OpenAPI().Paths

	create := paths["/students"].Post
	if create == nil || create.RequestBody == nil || create.RequestBody.Content["application/json"] == nil {
		t.Fatal("create student boundary has no JSON request body")
	}
	if !create.RequestBody.Required || create.RequestBody.Content["application/json"].Schema.Ref != "#/components/schemas/CreateStudent" {
		t.Fatalf("create student request schema = %#v, want required CreateStudent", create.RequestBody)
	}
	if create.Responses["201"].Content["application/json"].Schema.Ref != "#/components/schemas/Student" {
		t.Fatal("create student response is not derived from Student")
	}
	if paths["/students/{id}"].Delete.Responses["204"] == nil {
		t.Fatal("archive student boundary must declare 204")
	}
	if paths["/student/pair"].Post.RequestBody.Content["application/json"].Schema.Ref != "#/components/schemas/PairCode" {
		t.Fatal("pairing boundary is not derived from PairCode")
	}
}

func TestOpenAPIDerivesExactProductionRegistration(t *testing.T) {
	first, second := OpenAPI(), OpenAPI()
	if first != second {
		t.Fatal("offline OpenAPI emission is not deterministic")
	}

	registered := New(nil, "test").humaAPI().OpenAPI().Paths
	if len(registered) == 0 {
		t.Fatal("Huma registered no operations")
	}
	operationCount := 0
	for _, item := range registered {
		if item.Get != nil {
			operationCount++
		}
		if item.Post != nil {
			operationCount++
		}
		if item.Patch != nil {
			operationCount++
		}
		if item.Delete != nil {
			operationCount++
		}
	}
	if operationCount != 44 {
		t.Fatalf("registered %d operations, want 44", operationCount)
	}
	dialogueInspect := registered["/occurrences/{id}/inspect"]
	if dialogueInspect == nil || dialogueInspect.Get == nil || dialogueInspect.Get.OperationID != "occurrence-dialogue-inspect" {
		t.Fatal("dialogue inspect operation is missing or has the wrong operation ID")
	}
	dialogueOverride := registered["/occurrences/{id}/override"]
	if dialogueOverride == nil || dialogueOverride.Post == nil || dialogueOverride.Post.OperationID != "occurrence-dialogue-override" {
		t.Fatal("dialogue override operation is missing or has the wrong operation ID")
	}
	studentDialogue := registered["/student/occurrences/{id}/dialogue"]
	if studentDialogue == nil || studentDialogue.Post == nil || studentDialogue.Post.OperationID != "student-occurrence-dialogue" {
		t.Fatal("student dialogue operation is missing or has the wrong operation ID")
	}
	for path, item := range registered {
		if item.Get == nil && item.Post == nil && item.Patch == nil && item.Delete == nil {
			t.Fatalf("registered path %s has no operation", path)
		}
	}

	r := New(nil, "test").Routes()
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/openapi.yaml", nil))
	if rec.Code != http.StatusOK || rec.Body.String() != first {
		t.Fatalf("served contract = (%d, %q), want (200, generated document)", rec.Code, rec.Body.String())
	}

	students := registered["/students"]
	if students.Post == nil || students.Post.Responses["201"] == nil {
		t.Fatal("create student contract must declare the production 201 response")
	}
	student := registered["/students/{id}"]
	if student == nil || student.Delete == nil || student.Delete.Responses["204"] == nil {
		t.Fatal("archive student contract must declare the production 204 response")
	}
	studentChecklist := registered["/student/checklist"]
	if studentChecklist == nil || studentChecklist.Get == nil || studentChecklist.Get.OperationID != "student-checklist" {
		t.Fatal("browser checklist operation is missing or has the wrong operation ID")
	}
	deviceProfile := registered["/device/profile"]
	if deviceProfile == nil || deviceProfile.Get == nil || deviceProfile.Get.OperationID != "device-profile" {
		t.Fatal("device profile operation is missing or has the wrong operation ID")
	}
	deviceChecklist := registered["/device/checklist"]
	if deviceChecklist == nil || deviceChecklist.Get == nil || deviceChecklist.Get.OperationID != "device-checklist" {
		t.Fatal("device checklist operation is missing or has the wrong operation ID")
	}
}
