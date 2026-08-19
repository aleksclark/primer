package api

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"

	"github.com/go-chi/chi/v5"
)

type routeContract struct{ Path, Method string }

// productionRouteContracts walks the same chi router used by the server. There
// is deliberately no second route inventory for the offline contract to drift
// from production registration.
func productionRouteContracts() []routeContract {
	routes := []routeContract{}
	if err := chi.Walk(New(nil, "openapi").router(), func(method, path string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		routes = append(routes, routeContract{Path: path, Method: strings.ToLower(method)})
		return nil
	}); err != nil {
		panic(err)
	}
	sort.Slice(routes, func(i, j int) bool {
		if routes[i].Path == routes[j].Path {
			return routes[i].Method < routes[j].Method
		}
		return routes[i].Path < routes[j].Path
	})
	return routes
}

func ref(name string) map[string]string {
	return map[string]string{"$ref": "#/components/schemas/" + name}
}

func content(schema any) map[string]any {
	return map[string]any{"content": map[string]any{"application/json": map[string]any{"schema": schema}}}
}

func response(description string, schema any) map[string]any {
	out := map[string]any{"description": description}
	if schema != nil {
		for k, v := range content(schema) {
			out[k] = v
		}
	}
	return out
}

func responses(values map[string]map[string]any) map[string]any {
	out := make(map[string]any, len(values))
	for status, value := range values {
		out[status] = value
	}
	return out
}

func operation(path, method string) map[string]any {
	op := map[string]any{}
	switch {
	case path == "/health":
		op["responses"] = responses(map[string]map[string]any{"200": response("healthy", map[string]any{"type": "object", "properties": map[string]any{"status": map[string]string{"type": "string"}}})})
	case path == "/openapi.yaml":
		op["responses"] = responses(map[string]map[string]any{"200": response("OpenAPI document", nil)})
	case path == "/auth/login":
		op["responses"] = responses(map[string]map[string]any{"302": response("Redirect to identity provider", nil), "500": response("Unable to create authorization state", nil), "503": response("Identity provider is unconfigured", nil)})
	case path == "/auth/callback":
		op["responses"] = responses(map[string]map[string]any{"302": response("Parent session established", nil), "400": response("Invalid authorization state or request", nil), "401": response("Identity provider denied authorization", nil), "403": response("Principal is not provisioned", nil), "500": response("Unable to establish session", nil)})
	case path == "/auth/session":
		op["responses"] = responses(map[string]map[string]any{"200": response("parent session", ref("Session")), "401": response("Parent session required", nil)})
	case path == "/auth/logout":
		op["responses"] = responses(map[string]map[string]any{"200": response("signed out", ref("Status"))})
	case path == "/students" && method == "get":
		op["parameters"] = []any{map[string]any{"name": "q", "in": "query", "schema": map[string]string{"type": "string"}}, map[string]any{"name": "limit", "in": "query", "schema": map[string]string{"type": "integer"}}, map[string]any{"name": "offset", "in": "query", "schema": map[string]string{"type": "integer"}}}
		op["responses"] = responses(map[string]map[string]any{"200": response("student page", ref("StudentPage")), "401": response("Parent session required", nil), "500": response("Unable to list students", nil)})
	case path == "/students" && method == "post":
		op["requestBody"] = map[string]any{"required": true}
		for k, v := range content(ref("CreateStudent")) {
			op["requestBody"].(map[string]any)[k] = v
		}
		op["responses"] = responses(map[string]map[string]any{"201": response("student", ref("Student")), "400": response("Invalid request", nil), "401": response("Parent session required", nil), "409": response("Student conflicts with an existing record", nil), "500": response("Unable to create student", nil)})
	case path == "/students/{id}" && method == "get":
		op["parameters"] = pathParameter()
		op["responses"] = responses(map[string]map[string]any{"200": response("student", ref("Student")), "401": response("Parent session required", nil), "404": response("Student not found", nil), "500": response("Unable to load student", nil)})
	case path == "/students/{id}" && method == "patch":
		op["parameters"] = pathParameter()
		op["requestBody"] = content(ref("UpdateStudent"))
		op["responses"] = responses(map[string]map[string]any{"200": response("student", ref("Student")), "400": response("Invalid request", nil), "401": response("Parent session required", nil), "404": response("Student not found", nil), "409": response("Student cannot be updated", nil)})
	case path == "/students/{id}" && method == "delete":
		op["parameters"] = pathParameter()
		op["responses"] = responses(map[string]map[string]any{"204": response("student archived", nil), "401": response("Parent session required", nil), "404": response("Student not found", nil), "500": response("Unable to archive student", nil)})
	case path == "/students/{id}/pairing":
		op["parameters"] = pathParameter()
		op["responses"] = responses(map[string]map[string]any{"200": response("pairing", ref("Pairing")), "401": response("Parent session required", nil), "404": response("Student not found", nil), "500": response("Unable to issue pairing", nil)})
	case path == "/student/pair":
		op["requestBody"] = content(ref("Pair"))
		op["responses"] = responses(map[string]map[string]any{"200": response("paired", ref("StudentPair")), "400": response("Invalid request", nil), "410": response("Pairing code is unavailable", nil)})
	case path == "/student/profile" || path == "/device/profile":
		op["responses"] = responses(map[string]map[string]any{"200": response("student profile", ref("Student")), "401": response("Student credential required", nil)})
	case path == "/student/checklist":
		op["responses"] = responses(map[string]map[string]any{"200": response("checklist", ref("Checklist")), "401": response("Student session required", nil)})
	case path == "/device/pair":
		op["requestBody"] = content(ref("Pair"))
		op["responses"] = responses(map[string]map[string]any{"200": response("device paired", ref("DevicePair")), "400": response("Invalid request", nil), "410": response("Pairing code is unavailable", nil)})
	default:
		panic("missing OpenAPI operation for registered route " + method + " " + path)
	}
	return op
}

func pathParameter() []any {
	return []any{map[string]any{"name": "id", "in": "path", "required": true, "schema": map[string]string{"type": "string"}}}
}

func generatedOpenAPI() string {
	paths := map[string]map[string]any{}
	for _, route := range productionRouteContracts() {
		if paths[route.Path] == nil {
			paths[route.Path] = map[string]any{}
		}
		paths[route.Path][route.Method] = operation(route.Path, route.Method)
	}
	schemas := map[string]any{
		"Student":       map[string]any{"type": "object", "required": []string{"id", "displayName", "createdAt"}, "properties": map[string]any{"id": map[string]string{"type": "string"}, "displayName": map[string]string{"type": "string"}, "createdAt": map[string]string{"type": "string", "format": "date-time"}, "archivedAt": map[string]any{"type": "string", "format": "date-time", "nullable": true}}},
		"StudentPage":   map[string]any{"type": "object", "required": []string{"items", "totalCount", "limit", "offset"}, "properties": map[string]any{"items": map[string]any{"type": "array", "items": ref("Student")}, "totalCount": map[string]string{"type": "integer"}, "limit": map[string]string{"type": "integer"}, "offset": map[string]string{"type": "integer"}}},
		"CreateStudent": map[string]any{"type": "object", "required": []string{"displayName"}, "properties": map[string]any{"displayName": map[string]string{"type": "string"}}},
		"UpdateStudent": map[string]any{"type": "object", "required": []string{"displayName"}, "properties": map[string]any{"displayName": map[string]string{"type": "string"}}},
		"Pair":          map[string]any{"type": "object", "required": []string{"code"}, "properties": map[string]any{"code": map[string]string{"type": "string"}}},
		"StudentPair":   map[string]any{"type": "object", "required": []string{"studentId"}, "properties": map[string]any{"studentId": map[string]string{"type": "string"}}},
		"DevicePair":    map[string]any{"type": "object", "required": []string{"token", "studentId"}, "properties": map[string]any{"token": map[string]string{"type": "string"}, "studentId": map[string]string{"type": "string"}}},
		"Pairing":       map[string]any{"type": "object", "required": []string{"pairingId", "code", "expiresAt", "qrPayload"}, "properties": map[string]any{"pairingId": map[string]string{"type": "string"}, "code": map[string]string{"type": "string"}, "expiresAt": map[string]string{"type": "string", "format": "date-time"}, "qrPayload": map[string]string{"type": "string"}}},
		"ChecklistItem": map[string]any{"type": "object", "properties": map[string]any{"id": map[string]string{"type": "string"}, "title": map[string]string{"type": "string"}, "description": map[string]string{"type": "string"}, "status": map[string]string{"type": "string"}}},
		"Checklist":     map[string]any{"type": "object", "required": []string{"items"}, "properties": map[string]any{"items": map[string]any{"type": "array", "items": ref("ChecklistItem")}}},
		"Session":       map[string]any{"type": "object", "required": []string{"subjectRef", "tenantId"}, "properties": map[string]any{"subjectRef": map[string]string{"type": "string"}, "tenantId": map[string]string{"type": "string"}}},
		"Status":        map[string]any{"type": "object", "required": []string{"status"}, "properties": map[string]any{"status": map[string]string{"type": "string"}}},
	}
	doc := map[string]any{"openapi": "3.0.3", "info": map[string]any{"title": "Primer Tasks", "version": "1.0.0"}, "paths": paths, "components": map[string]any{"schemas": schemas}}
	b, err := json.Marshal(doc)
	if err != nil {
		panic(err)
	}
	return string(b)
}
