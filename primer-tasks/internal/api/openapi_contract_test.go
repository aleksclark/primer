package api

import (
	"bytes"
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
	revision := paths["/tasks/{id}/revisions"].Post
	if revision == nil || len(revision.Parameters) != 1 || revision.Parameters[0].Name != "id" || revision.Parameters[0].In != "path" {
		t.Fatalf("task revision boundary has no generated id path parameter: %#v", revision)
	}
}

func TestArtifactBoundariesAreStrictAndGenerated(t *testing.T) {
	doc := New(nil, "openapi").humaAPI().OpenAPI()
	checks := []struct {
		path     string
		method   string
		request  string
		response string
		status   string
	}{
		{"/student/occurrences/{occurrence}/artifacts/reserve", http.MethodPost, "ArtifactReservationInput", "ArtifactReservationOutput", "201"},
		{"/student/occurrences/{occurrence}/artifacts", http.MethodGet, "", "ArtifactStateResponse", "200"},
		{"/student/occurrences/{occurrence}/artifacts/finalize", http.MethodPost, "ArtifactFinalizeInput", "ArtifactOutput", "200"},
		{"/student/occurrences/{occurrence}/artifacts/retry", http.MethodPost, "", "ArtifactStateResponse", "200"},
		{"/occurrences/{occurrence}/artifacts/inspect", http.MethodGet, "", "ArtifactStateResponse", "200"},
	}
	for _, check := range checks {
		item := doc.Paths[check.path]
		if item == nil {
			t.Fatalf("artifact path %s is missing", check.path)
		}
		var operation *huma.Operation
		switch check.method {
		case http.MethodGet:
			operation = item.Get
		case http.MethodPost:
			operation = item.Post
		}
		if operation == nil || operation.Responses[check.status] == nil {
			t.Fatalf("artifact %s %s has no %s response", check.method, check.path, check.status)
		}
		if check.request != "" {
			if operation.RequestBody == nil || operation.RequestBody.Content["application/json"].Schema.Ref != "#/components/schemas/"+check.request {
				t.Fatalf("artifact %s request schema is not %s", check.path, check.request)
			}
		}
		if got := operation.Responses[check.status].Content["application/json"].Schema.Ref; got != "#/components/schemas/"+check.response {
			t.Fatalf("artifact %s response schema = %q, want %s", check.path, got, check.response)
		}
	}
	for _, name := range []string{"ArtifactReservationInput", "ArtifactFinalizeInput", "ArtifactReservationOutput", "ArtifactOutput", "ArtifactStateResponse"} {
		schema := doc.Components.Schemas.Map()[name]
		if schema == nil || schema.AdditionalProperties != false {
			t.Fatalf("artifact schema %s is not closed: %#v", name, schema)
		}
	}
}

func TestArtifactJSONBoundaryRejectsUnknownFields(t *testing.T) {
	handler := New(nil, "openapi").Routes()
	req := httptest.NewRequest(http.MethodPost, "/student/occurrences/00000000-0000-0000-0000-000000000001/artifacts/reserve", bytes.NewBufferString(`{"kind":"image","contentType":"image/png","size":4,"idempotencyKey":"idem","unexpected":true}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown artifact field status = %d, body=%s; want 400", rec.Code, rec.Body.String())
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
	if operationCount != 59 {
		t.Fatalf("registered %d operations, want 59", operationCount)
	}
	externalSubmit := registered["/student/occurrences/{id}/external/submit"]
	if externalSubmit == nil || externalSubmit.Post == nil || externalSubmit.Post.OperationID != "student-external-submit" {
		t.Fatal("external student submit operation is missing or has the wrong operation ID")
	}
	externalInspect := registered["/occurrences/{id}/external/inspect"]
	if externalInspect == nil || externalInspect.Get == nil || externalInspect.Get.OperationID != "parent-external-inspect" {
		t.Fatal("external parent inspect operation is missing or has the wrong operation ID")
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
	studentDialogueState := registered["/student/occurrences/{id}/dialogue"]
	if studentDialogueState == nil || studentDialogueState.Get == nil || studentDialogueState.Get.OperationID != "student-occurrence-dialogue-state" {
		t.Fatal("student dialogue state operation is missing or has the wrong operation ID")
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
