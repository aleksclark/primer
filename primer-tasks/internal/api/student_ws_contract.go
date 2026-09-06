package api

// One runtime-used contract: actual socket structs carry variant membership and
// scalar constraints. Reflection drives BOTH strict validation and offline
// schema/TypeScript emission. There is no client-owned wire declaration.
import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"primer-tasks/internal/domain"
)

type studentWireVariant struct {
	Name, Kind string
	Scoped     bool
}

var studentCommandVariants = []studentWireVariant{{Name: "subscribe", Kind: "subscribe"}, {Name: "user_message", Kind: "user_message"}, {Name: "retry", Kind: "retry"}, {Name: "ack", Kind: "ack"}, {Name: "unsubscribe", Kind: "unsubscribe"}}
var studentEventVariants = []studentWireVariant{{Name: "hello", Kind: "hello"}, {Name: "state", Kind: "state", Scoped: true}, {Name: "question", Kind: "question", Scoped: true}, {Name: "message_ack", Kind: "message_ack", Scoped: true}, {Name: "progress", Kind: "progress", Scoped: true}, {Name: "answer_evaluation", Kind: "answer_evaluation", Scoped: true}, {Name: "complete", Kind: "complete", Scoped: true}, {Name: "override", Kind: "override", Scoped: true}, {Name: "error", Kind: "error"}, {Name: "terminal_error", Kind: "error", Scoped: true}}

var studentCommandContract = studentWireContract(reflect.TypeOf(studentCommand{}), studentCommandVariants)
var studentEventContract = studentWireContract(reflect.TypeOf(wireStudentEvent{}), studentEventVariants)
var wireUUID = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
var wireSHA256 = regexp.MustCompile(`^[0-9a-f]{64}$`)

func variantMatches(label string, v studentWireVariant) bool {
	return label == "*" || label == v.Name || (label == "scope" && v.Scoped)
}
func wireMembership(f reflect.StructField, v studentWireVariant) (allowed, required bool) {
	for _, item := range strings.Split(f.Tag.Get("wire"), ",") {
		if variantMatches(strings.TrimSuffix(item, "!"), v) {
			allowed = true
			required = required || strings.HasSuffix(item, "!")
		}
	}
	return
}
func wireProperty(f reflect.StructField, v studentWireVariant) map[string]any {
	p := map[string]any{}
	switch f.Type.Kind() {
	case reflect.String:
		p["type"] = "string"
	case reflect.Int, reflect.Int64:
		p["type"] = "integer"
		p["maximum"] = int64(9007199254740991)
	case reflect.Bool:
		p["type"] = "boolean"
	default:
		if f.Type == reflect.TypeOf(time.Time{}) {
			p["type"] = "string"
		} else {
			panic("unsupported student wire field " + f.Name)
		}
	}
	for _, tag := range []struct{ Go, Schema string }{{"wireMin", "minimum"}, {"wireMax", "maximum"}, {"wireMinLength", "minLength"}, {"wireMaxLength", "maxLength"}, {"wireMaxBytes", "x-maxBytes"}} {
		if raw := f.Tag.Get(tag.Go); raw != "" {
			number, err := strconv.ParseInt(raw, 10, 64)
			if err != nil {
				panic(err)
			}
			p[tag.Schema] = number
		}
	}
	if f.Tag.Get("wireNonBlank") == "true" {
		p["x-nonBlank"] = true
	}
	if format := f.Tag.Get("wireFormat"); format != "" {
		p["format"] = format
	}
	for _, item := range strings.Split(f.Tag.Get("wireEnum"), ";") {
		label, values, ok := strings.Cut(item, "=")
		if !ok || !variantMatches(label, v) {
			continue
		}
		var enums []any
		for _, value := range strings.Split(values, "|") {
			if p["type"] == "integer" {
				n, err := strconv.ParseInt(value, 10, 64)
				if err != nil {
					panic(err)
				}
				enums = append(enums, n)
			} else {
				enums = append(enums, value)
			}
		}
		if len(enums) == 1 {
			p["const"] = enums[0]
		} else {
			p["enum"] = enums
		}
	}
	if (f.Name == "Sequence" || f.Name == "Cursor") && v.Scoped && v.Name != "state" {
		p["minimum"] = int64(1)
	}
	if f.Name == "Text" && (v.Name == "question" || v.Name == "state") {
		p["maxLength"] = int64(500)
	}
	return p
}
func studentWireContract(t reflect.Type, variants []studentWireVariant) map[string]any {
	var alternatives []any
	fieldName := func(name string) string {
		field, ok := t.FieldByName(name)
		if !ok {
			panic("missing student wire field " + name)
		}
		return strings.Split(field.Tag.Get("json"), ",")[0]
	}
	for _, variant := range variants {
		properties := map[string]any{}
		var required []string
		for i := 0; i < t.NumField(); i++ {
			field := t.Field(i)
			name := strings.Split(field.Tag.Get("json"), ",")[0]
			if name == "" || name == "-" {
				continue
			}
			allowed, need := wireMembership(field, variant)
			if !allowed {
				continue
			}
			properties[name] = wireProperty(field, variant)
			if need {
				required = append(required, name)
			}
		}
		properties[fieldName("Protocol")] = map[string]any{"const": studentProtocolVersion, "type": "integer"}
		properties[fieldName("Kind")] = map[string]any{"const": variant.Kind, "type": "string"}
		sort.Strings(required)
		schema := map[string]any{"type": "object", "title": variant.Name, "properties": properties, "required": required, "additionalProperties": false}
		if t == reflect.TypeOf(wireStudentEvent{}) {
			schema["x-equalFields"] = []string{fieldName("Sequence"), fieldName("Cursor")}
			if variant.Name == "state" {
				schema["dependentRequired"] = map[string]any{fieldName("QuestionID"): []string{fieldName("Text")}, fieldName("Text"): []string{fieldName("QuestionID")}, fieldName("Code"): []string{fieldName("Phase")}, fieldName("Retryable"): []string{fieldName("Phase"), fieldName("Code")}}
				schema["allOf"] = []any{
					map[string]any{"if": map[string]any{"required": []string{fieldName("Phase")}, "properties": map[string]any{fieldName("Phase"): map[string]any{"const": "failed"}}}, "then": map[string]any{"required": []string{fieldName("Code")}}},
					map[string]any{"if": map[string]any{"required": []string{fieldName("Code")}}, "then": map[string]any{"properties": map[string]any{fieldName("Phase"): map[string]any{"const": "failed"}}}},
					map[string]any{"if": map[string]any{"required": []string{fieldName("Status")}, "properties": map[string]any{fieldName("Status"): map[string]any{"const": "accepted"}}}, "then": map[string]any{"required": []string{fieldName("DecisionSource")}}},
				}
			}
			if variant.Name == "complete" || variant.Name == "state" {
				condition := map[string]any{"properties": map[string]any{fieldName("DecisionSource"): map[string]any{"const": "verification_engine"}, fieldName("Status"): map[string]any{"const": "accepted"}}, "required": []string{fieldName("DecisionSource"), fieldName("Status")}}
				schema["if"] = condition
				schema["then"] = map[string]any{"properties": map[string]any{fieldName("AcceptedCount"): map[string]any{"const": 3}}, "required": []string{fieldName("AcceptedCount")}}
			}
		}
		alternatives = append(alternatives, schema)
	}
	return map[string]any{"oneOf": alternatives}
}

func strictWireJSON(data []byte) (map[string]any, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	start, err := decoder.Token()
	if err != nil || start != json.Delim('{') {
		return nil, errors.New("wire object required")
	}
	out := map[string]any{}
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		name, ok := key.(string)
		if !ok {
			return nil, errors.New("wire property required")
		}
		if _, duplicate := out[name]; duplicate {
			return nil, errors.New("duplicate wire property")
		}
		var value any
		if err = decoder.Decode(&value); err != nil {
			return nil, err
		}
		out[name] = value
	}
	end, err := decoder.Token()
	if err != nil || end != json.Delim('}') || decoder.Decode(new(any)) != io.EOF {
		return nil, errors.New("one wire object required")
	}
	return out, nil
}
func validateStudentContractJSON(schema map[string]any, data []byte) error {
	object, err := strictWireJSON(data)
	if err != nil {
		return err
	}
	if !matchesStudentSchema(schema, object) {
		return errors.New("student wire contract rejected")
	}
	return nil
}
func wireNumber(value any) (float64, bool) {
	switch v := value.(type) {
	case json.Number:
		n, e := v.Float64()
		return n, e == nil
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case float64:
		return v, true
	}
	return 0, false
}
func wireEqual(a, b any) bool {
	if x, ok := wireNumber(a); ok {
		y, valid := wireNumber(b)
		return valid && x == y
	}
	return reflect.DeepEqual(a, b)
}
func matchesStudentSchema(schema map[string]any, value any) bool {
	if all, ok := schema["allOf"].([]any); ok {
		for _, part := range all {
			if !matchesStudentSchema(part.(map[string]any), value) {
				return false
			}
		}
	}
	if alternatives, ok := schema["oneOf"].([]any); ok {
		matches := 0
		for _, alternative := range alternatives {
			if matchesStudentSchema(alternative.(map[string]any), value) {
				matches++
			}
		}
		if matches != 1 {
			return false
		}
	}
	if constant, ok := schema["const"]; ok && !wireEqual(value, constant) {
		return false
	}
	if values, ok := schema["enum"].([]any); ok {
		matched := false
		for _, v := range values {
			matched = matched || wireEqual(value, v)
		}
		if !matched {
			return false
		}
	}
	if condition, ok := schema["if"].(map[string]any); ok && matchesStudentSchema(condition, value) {
		if !matchesStudentSchema(schema["then"].(map[string]any), value) {
			return false
		}
	}
	switch schema["type"] {
	case "object":
		if _, ok := value.(map[string]any); !ok {
			return false
		}
	case "integer":
		n, ok := wireNumber(value)
		if !ok || math.IsNaN(n) || math.IsInf(n, 0) || math.Trunc(n) != n {
			return false
		}
		for _, bound := range []string{"minimum", "maximum"} {
			if v, exists := schema[bound]; exists {
				limit, _ := wireNumber(v)
				if bound == "minimum" && n < limit || bound == "maximum" && n > limit {
					return false
				}
			}
		}
	case "boolean":
		if _, ok := value.(bool); !ok {
			return false
		}
	case "string":
		s, ok := value.(string)
		if !ok || !utf8.ValidString(s) {
			return false
		}
		for _, bound := range []string{"minLength", "maxLength", "x-maxBytes"} {
			if v, exists := schema[bound]; exists {
				limit, _ := wireNumber(v)
				n := len([]rune(s))
				if bound == "x-maxBytes" {
					n = len(s)
				}
				if bound == "minLength" && n < int(limit) || bound != "minLength" && n > int(limit) {
					return false
				}
			}
		}
		if schema["x-nonBlank"] == true && strings.TrimSpace(s) == "" {
			return false
		}
		switch schema["format"] {
		case "uuid":
			if !wireUUID.MatchString(s) {
				return false
			}
		case "sha256":
			if !wireSHA256.MatchString(s) {
				return false
			}
		case "date-time":
			at, err := time.Parse(time.RFC3339Nano, s)
			if err != nil || at.IsZero() {
				return false
			}
		}
	}
	if object, ok := value.(map[string]any); ok {
		if names, ok := schema["required"].([]string); ok {
			for _, name := range names {
				if _, present := object[name]; !present {
					return false
				}
			}
		}
		properties, _ := schema["properties"].(map[string]any)
		for key, v := range object {
			property, known := properties[key]
			if !known {
				if schema["additionalProperties"] == false {
					return false
				}
				continue
			}
			if !matchesStudentSchema(property.(map[string]any), v) {
				return false
			}
		}
		if dependencies, ok := schema["dependentRequired"].(map[string]any); ok {
			for key, names := range dependencies {
				if _, present := object[key]; present {
					for _, name := range names.([]string) {
						if _, exists := object[name]; !exists {
							return false
						}
					}
				}
			}
		}
		if fields, ok := schema["x-equalFields"].([]string); ok && len(fields) == 2 {
			if !wireEqual(object[fields[0]], object[fields[1]]) {
				return false
			}
		}
	}
	return true
}

func StudentSocketSchema() ([]byte, error) {
	return json.MarshalIndent(map[string]any{"$schema": "https://json-schema.org/draft/2020-12/schema", "$defs": map[string]any{"command": studentCommandContract, "event": studentEventContract}, "x-transport": map[string]any{"protocol": studentSocketProtocol, "path": studentSocketPath, "authentication": "host-only-student-cookie", "readLimitBytes": studentReadLimit, "replayPageSize": studentReplayPageSize, "acknowledgmentWindow": studentAckWindow, "writeTimeoutMs": studentWriteTimeout.Milliseconds()}}, "", "  ")
}
func tsWireType(schema map[string]any) string {
	if value, ok := schema["const"]; ok {
		encoded, _ := json.Marshal(value)
		return string(encoded)
	}
	if values, ok := schema["enum"].([]any); ok {
		parts := []string{}
		for _, v := range values {
			encoded, _ := json.Marshal(v)
			parts = append(parts, string(encoded))
		}
		return strings.Join(parts, " | ")
	}
	switch schema["type"] {
	case "integer":
		return "number"
	case "boolean":
		return "boolean"
	case "string":
		return "string"
	}
	panic("unsupported generated student scalar")
}
func DialogueConfigSchema() ([]byte, error) {
	return json.MarshalIndent(domain.DialogueConfigSchema(), "", "  ")
}
func DialogueConfigTypeScript() string {
	schema := domain.DialogueConfigSchema()
	properties := schema["properties"].(map[string]any)
	required := map[string]bool{}
	for _, name := range schema["required"].([]string) {
		required[name] = true
	}
	keys := []string{}
	for name := range properties {
		keys = append(keys, name)
	}
	sort.Strings(keys)
	var out strings.Builder
	out.WriteString("// Generated from Go parent dialogue configuration. Do not edit or commit.\nexport interface DialogueConfig {\n")
	for _, name := range keys {
		property := properties[name].(map[string]any)
		typ := ""
		if property["type"] == "array" {
			typ = tsWireType(property["items"].(map[string]any)) + "[]"
		} else {
			typ = tsWireType(property)
		}
		optional := "?"
		if required[name] {
			optional = ""
		}
		fmt.Fprintf(&out, "  %s%s: %s;\n", name, optional, typ)
	}
	out.WriteString("}\n")
	data, err := DialogueConfigSchema()
	if err != nil {
		panic(err)
	}
	fmt.Fprintf(&out, "export const DIALOGUE_CONFIG_SCHEMA = %s as const;\n", data)
	return out.String()
}

func StudentSocketTypeScript() string {
	var out strings.Builder
	fmt.Fprintf(&out, "// Generated from actual Go student socket boundaries and runtime rules. Do not edit or commit.\nexport const STUDENT_DIALOGUE_PROTOCOL_VERSION = %d as const;\n", studentProtocolVersion)
	for _, pair := range []struct {
		Name   string
		Schema map[string]any
	}{{"StudentDialogueCommand", studentCommandContract}, {"StudentDialogueEvent", studentEventContract}} {
		fmt.Fprintf(&out, "export type %s =\n", pair.Name)
		for _, raw := range pair.Schema["oneOf"].([]any) {
			variant := raw.(map[string]any)
			properties := variant["properties"].(map[string]any)
			required := map[string]bool{}
			for _, field := range variant["required"].([]string) {
				required[field] = true
			}
			keys := []string{}
			for key := range properties {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			out.WriteString("  | {")
			for _, key := range keys {
				optional := "?"
				if required[key] {
					optional = ""
				}
				fmt.Fprintf(&out, " %s%s: %s;", key, optional, tsWireType(properties[key].(map[string]any)))
			}
			out.WriteString(" }\n")
		}
		out.WriteString(";\n")
	}
	data, err := StudentSocketSchema()
	if err != nil {
		panic(err)
	}
	fmt.Fprintf(&out, "export const STUDENT_DIALOGUE_SCHEMA = %s as const;\n", data)
	out.WriteString("export type StudentDialogueEventKind = StudentDialogueEvent['kind'];\n")
	return out.String()
}
