package api

import "encoding/json"

type routeContract struct{ Path, Method string }

var routeContracts = []routeContract{{"/health", "get"}, {"/auth/login", "get"}, {"/auth/callback", "get"}, {"/auth/session", "get"}, {"/auth/logout", "post"}, {"/students", "get"}, {"/students", "post"}, {"/students/{id}", "get"}, {"/students/{id}", "patch"}, {"/students/{id}", "delete"}, {"/students/{id}/pairing", "post"}, {"/student/pair", "post"}, {"/student/profile", "get"}, {"/student/checklist", "get"}, {"/device/pair", "post"}, {"/device/profile", "get"}}

func ref(name string) map[string]string {
	return map[string]string{"$ref": "#/components/schemas/" + name}
}
func content(schema any) map[string]any {
	return map[string]any{"content": map[string]any{"application/json": map[string]any{"schema": schema}}}
}
func operation(path, method string) map[string]any {
	response := map[string]any{"description": "Successful response"}
	switch {
	case path == "/students" && method == "get":
		response = map[string]any{"description": "student page"}
		for k, v := range content(ref("StudentPage")) {
			response[k] = v
		}
	case path == "/students" && method == "post":
		response = map[string]any{"description": "student"}
		for k, v := range content(ref("Student")) {
			response[k] = v
		}
	case path == "/students/{id}" && (method == "get" || method == "patch"):
		response = map[string]any{"description": "student"}
		for k, v := range content(ref("Student")) {
			response[k] = v
		}
	case path == "/students/{id}/pairing":
		response = map[string]any{"description": "pairing"}
		for k, v := range content(ref("Pairing")) {
			response[k] = v
		}
	case path == "/student/profile":
		response = map[string]any{"description": "profile"}
		for k, v := range content(ref("Student")) {
			response[k] = v
		}
	case path == "/student/checklist":
		response = map[string]any{"description": "checklist"}
		for k, v := range content(ref("Checklist")) {
			response[k] = v
		}
	case path == "/student/pair":
		response = map[string]any{"description": "paired"}
		for k, v := range content(map[string]any{"type": "object", "properties": map[string]any{"studentId": map[string]string{"type": "string"}}}) {
			response[k] = v
		}
	case path == "/device/pair":
		response = map[string]any{"description": "device paired"}
		for k, v := range content(map[string]any{"type": "object", "properties": map[string]any{"token": map[string]string{"type": "string"}, "studentId": map[string]string{"type": "string"}}}) {
			response[k] = v
		}
	}
	op := map[string]any{"responses": map[string]any{"200": response, "401": map[string]any{"description": "Unauthorized"}, "403": map[string]any{"description": "Denied"}}}
	if path == "/students" && method == "post" {
		op["requestBody"] = map[string]any{"required": true}
		for k, v := range content(ref("CreateStudent")) {
			op["requestBody"].(map[string]any)[k] = v
		}
	}
	if path == "/students/{id}" && method == "patch" {
		op["requestBody"] = content(ref("UpdateStudent"))
	}
	if path == "/student/pair" || path == "/device/pair" {
		op["requestBody"] = content(ref("Pair"))
	}
	if path == "/students" && method == "get" {
		op["parameters"] = []any{map[string]any{"name": "q", "in": "query", "schema": map[string]string{"type": "string"}}, map[string]any{"name": "limit", "in": "query", "schema": map[string]string{"type": "integer"}}, map[string]any{"name": "offset", "in": "query", "schema": map[string]string{"type": "integer"}}}
	}
	if path == "/students/{id}" || path == "/students/{id}/pairing" {
		op["parameters"] = []any{map[string]any{"name": "id", "in": "path", "required": true, "schema": map[string]string{"type": "string"}}}
	}
	return op
}
func generatedOpenAPI() string {
	paths := map[string]map[string]any{}
	for _, r := range routeContracts {
		if paths[r.Path] == nil {
			paths[r.Path] = map[string]any{}
		}
		paths[r.Path][r.Method] = operation(r.Path, r.Method)
	}
	schemas := map[string]any{"Student": map[string]any{"type": "object", "required": []string{"id", "displayName", "createdAt"}, "properties": map[string]any{"id": map[string]string{"type": "string"}, "displayName": map[string]string{"type": "string"}, "createdAt": map[string]string{"type": "string", "format": "date-time"}, "archivedAt": map[string]any{"type": "string", "format": "date-time", "nullable": true}}}, "StudentPage": map[string]any{"type": "object", "required": []string{"items", "totalCount", "limit", "offset"}, "properties": map[string]any{"items": map[string]any{"type": "array", "items": ref("Student")}, "totalCount": map[string]string{"type": "integer"}, "limit": map[string]string{"type": "integer"}, "offset": map[string]string{"type": "integer"}}}, "CreateStudent": map[string]any{"type": "object", "required": []string{"displayName"}, "properties": map[string]any{"displayName": map[string]string{"type": "string"}}}, "UpdateStudent": map[string]any{"type": "object", "required": []string{"displayName"}, "properties": map[string]any{"displayName": map[string]string{"type": "string"}}}, "Pair": map[string]any{"type": "object", "required": []string{"code"}, "properties": map[string]any{"code": map[string]string{"type": "string"}}}, "Pairing": map[string]any{"type": "object", "required": []string{"pairingId", "code", "expiresAt", "qrPayload"}, "properties": map[string]any{"pairingId": map[string]string{"type": "string"}, "code": map[string]string{"type": "string"}, "expiresAt": map[string]string{"type": "string", "format": "date-time"}, "qrPayload": map[string]string{"type": "string"}}}, "ChecklistItem": map[string]any{"type": "object", "properties": map[string]any{"id": map[string]string{"type": "string"}, "title": map[string]string{"type": "string"}, "description": map[string]string{"type": "string"}, "status": map[string]string{"type": "string"}}}, "Checklist": map[string]any{"type": "object", "required": []string{"items"}, "properties": map[string]any{"items": map[string]any{"type": "array", "items": ref("ChecklistItem")}}}}
	doc := map[string]any{"openapi": "3.0.3", "info": map[string]any{"title": "Primer Tasks", "version": "1.0.0"}, "paths": paths, "components": map[string]any{"schemas": schemas}}
	b, _ := json.Marshal(doc)
	return string(b)
}
