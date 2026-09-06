package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"
)

// The same variant requirements constrain outbound socket values and drive
// offline schema/client emission. Field names/types are reflected from the
// actual socket boundary structs, never copied into a second DTO definition.
var socketEventFields = map[string][]string{
	"hello": {}, "user_message": {"runId", "text", "clientMessageId"},
	"text_start": {"runId"}, "text_delta": {"runId", "text"}, "text_end": {"runId"},
	"thinking_start": {"runId"}, "thinking_end": {"runId"},
	"tool_progress": {"runId", "label", "phase"},
	"retry":         {"runId", "retry", "retryAfterMs"}, "terminal": {"runId", "status"},
	"error": {"code"}, "replay_gap": {"runId"},
}
var socketCommandFields = map[string][]string{
	"hello": {}, "subscribe": {"conversationId"}, "unsubscribe": {},
	"user_message": {"conversationId", "clientMessageId", "text"},
	"cancel":       {"runId"}, "confirm": {"runId", "confirmationId"}, "ack": {"cursor"},
}

func socketFields(t reflect.Type) map[string]reflect.StructField {
	out := map[string]reflect.StructField{}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		name := strings.Split(f.Tag.Get("json"), ",")[0]
		if name != "" && name != "-" {
			out[name] = f
		}
	}
	return out
}
func validateSocketEvent(e wireAgentEvent) error {
	required, ok := socketEventFields[e.Type]
	if !ok {
		return fmt.Errorf("unsupported event variant")
	}
	if e.ProtocolVersion != agentProtocolVersion || e.Time.IsZero() || e.Cursor < 0 || e.Sequence < 0 {
		return fmt.Errorf("invalid event envelope")
	}
	fields := socketFields(reflect.TypeOf(e))
	value := reflect.ValueOf(e)
	for _, name := range required {
		f := fields[name]
		v := value.FieldByIndex(f.Index)
		if v.Kind() == reflect.String && v.String() == "" {
			return fmt.Errorf("missing event field %s", name)
		}
	}
	return nil
}
func socketVariants(t reflect.Type, kinds map[string][]string, event bool) []any {
	fields := socketFields(t)
	names := make([]string, 0, len(kinds))
	for kind := range kinds {
		names = append(names, kind)
	}
	sort.Strings(names)
	variants := []any{}
	for _, kind := range names {
		props := map[string]any{}
		for name, f := range fields {
			typ := "string"
			switch f.Type.Kind() {
			case reflect.Int, reflect.Int64:
				typ = "integer"
			case reflect.Bool:
				typ = "boolean"
			}
			prop := map[string]any{"type": typ}
			if f.Type == reflect.TypeOf(time.Time{}) {
				prop["format"] = "date-time"
			}
			props[name] = prop
		}
		props["kind"] = map[string]any{"const": kind}
		props["protocol"] = map[string]any{"const": agentProtocolVersion}
		required := []string{"protocol", "kind"}
		if event {
			required = append(required, "time", "sequence", "cursor")
		}
		required = append(required, kinds[kind]...)
		variants = append(variants, map[string]any{"type": "object", "properties": props, "required": required, "additionalProperties": false})
	}
	return variants
}

// AgentSocketSchema is offline and imports no provider, DB, or server instance.
func AgentSocketSchema() ([]byte, error) {
	return json.MarshalIndent(map[string]any{"$schema": "https://json-schema.org/draft/2020-12/schema", "$defs": map[string]any{"command": map[string]any{"oneOf": socketVariants(reflect.TypeOf(agentCommand{}), socketCommandFields, false)}, "event": map[string]any{"oneOf": socketVariants(reflect.TypeOf(wireAgentEvent{}), socketEventFields, true)}}}, "", "  ")
}
func AgentSocketTypeScript() string {
	var b bytes.Buffer
	fmt.Fprintf(&b, "// Generated from the Go socket boundary. Do not edit or commit.\nexport const AGENT_PROTOCOL_VERSION = %d as const;\n", agentProtocolVersion)
	emit := func(name string, t reflect.Type, kinds map[string][]string, event bool) {
		fields := socketFields(t)
		keys := make([]string, 0, len(fields))
		for k := range fields {
			if k != "kind" && k != "protocol" {
				keys = append(keys, k)
			}
		}
		sort.Strings(keys)
		names := make([]string, 0, len(kinds))
		for k := range kinds {
			names = append(names, k)
		}
		sort.Strings(names)
		fmt.Fprintf(&b, "export type %s =\n", name)
		for _, kind := range names {
			required := map[string]bool{}
			for _, k := range kinds[kind] {
				required[k] = true
			}
			if event {
				required["time"], required["sequence"], required["cursor"] = true, true, true
			}
			fmt.Fprintf(&b, "  | { protocol: typeof AGENT_PROTOCOL_VERSION; kind: %q;", kind)
			for _, key := range keys {
				f := fields[key]
				typ := "string"
				switch f.Type.Kind() {
				case reflect.Int, reflect.Int64:
					typ = "number"
				case reflect.Bool:
					typ = "boolean"
				}
				optional := "?"
				if required[key] {
					optional = ""
				}
				fmt.Fprintf(&b, " %s%s: %s;", key, optional, typ)
			}
			b.WriteString(" }\n")
		}
		b.WriteString(";\n")
	}
	emit("AgentCommand", reflect.TypeOf(agentCommand{}), socketCommandFields, false)
	emit("AgentEvent", reflect.TypeOf(wireAgentEvent{}), socketEventFields, true)
	b.WriteString("export type AgentEventKind = AgentEvent['kind'];\n")
	for _, name := range []string{"Hello", "Subscribe", "Unsubscribe", "Message", "Cancel", "Confirm"} {
		kind := strings.ToLower(name)
		if name == "Message" {
			kind = "user_message"
		}
		fmt.Fprintf(&b, "export type Agent%sCommand = Extract<AgentCommand, { kind: %q }>;\n", name, kind)
	}
	return b.String()
}
