package contracts

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"gopkg.in/yaml.v3"
)

// DecodeError is a document parse failure with a stable path for authors/CI.
type DecodeError struct {
	Path    string
	Field   string
	Message string
	Err     error
}

func (e *DecodeError) Error() string {
	loc := e.Path
	if e.Field != "" {
		if loc != "" {
			loc = loc + ": "
		}
		loc = loc + e.Field
	}
	if loc == "" {
		if e.Err != nil {
			return e.Message + ": " + e.Err.Error()
		}
		return e.Message
	}
	if e.Err != nil {
		return fmt.Sprintf("%s: %s: %v", loc, e.Message, e.Err)
	}
	return fmt.Sprintf("%s: %s", loc, e.Message)
}

func (e *DecodeError) Unwrap() error { return e.Err }

// DecodeActivityJSON strictly decodes an activity document from JSON.
func DecodeActivityJSON(data []byte, pathHint string) (*ActivityDocument, error) {
	if err := rejectDuplicateJSONKeys(data); err != nil {
		return nil, &DecodeError{Path: pathHint, Message: "duplicate json key", Err: err}
	}
	var doc ActivityDocument
	if err := decodeJSONStrict(data, &doc); err != nil {
		return nil, &DecodeError{Path: pathHint, Message: "parse activity json", Err: err}
	}
	if err := dispatchActivitySchemaVersion(doc.SchemaVersion, pathHint); err != nil {
		return nil, err
	}
	return &doc, nil
}

// DecodeActivityYAML strictly decodes an activity document from YAML.
func DecodeActivityYAML(data []byte, pathHint string) (*ActivityDocument, error) {
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, &DecodeError{Path: pathHint, Message: "parse activity yaml", Err: err}
	}
	if err := rejectDuplicateYAMLKeys(&root, ""); err != nil {
		return nil, &DecodeError{Path: pathHint, Message: "duplicate yaml key", Err: err}
	}
	var doc ActivityDocument
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&doc); err != nil {
		return nil, &DecodeError{Path: pathHint, Message: "parse activity yaml", Err: err}
	}
	// Ensure single document.
	var extra any
	if err := dec.Decode(&extra); err != io.EOF && err != nil {
		return nil, &DecodeError{Path: pathHint, Message: "parse activity yaml", Err: fmt.Errorf("multiple documents not allowed")}
	}
	if err := dispatchActivitySchemaVersion(doc.SchemaVersion, pathHint); err != nil {
		return nil, err
	}
	return &doc, nil
}

// DecodeCourseJSON strictly decodes a course manifest from JSON.
func DecodeCourseJSON(data []byte, pathHint string) (*CourseDocument, error) {
	if err := rejectDuplicateJSONKeys(data); err != nil {
		return nil, &DecodeError{Path: pathHint, Message: "duplicate json key", Err: err}
	}
	var doc CourseDocument
	if err := decodeJSONStrict(data, &doc); err != nil {
		return nil, &DecodeError{Path: pathHint, Message: "parse course json", Err: err}
	}
	if err := dispatchCourseSchemaVersion(doc.SchemaVersion, pathHint); err != nil {
		return nil, err
	}
	return &doc, nil
}

func decodeJSONStrict(data []byte, dest any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	dec.UseNumber()
	if err := dec.Decode(dest); err != nil {
		return err
	}
	// Reject trailing junk.
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return fmt.Errorf("trailing data after json value")
		}
		return err
	}
	return nil
}

func dispatchActivitySchemaVersion(version, pathHint string) error {
	switch version {
	case SchemaVersion:
		return nil
	case "":
		return &DecodeError{Path: pathHint, Field: "schemaVersion", Message: "schema version is required"}
	default:
		return &DecodeError{
			Path:    pathHint,
			Field:   "schemaVersion",
			Message: fmt.Sprintf("unsupported schema version %q (want %q)", version, SchemaVersion),
		}
	}
}

func dispatchCourseSchemaVersion(version, pathHint string) error {
	switch version {
	case CourseSchemaVersion:
		return nil
	case "":
		return &DecodeError{Path: pathHint, Field: "schemaVersion", Message: "schema version is required"}
	default:
		return &DecodeError{
			Path:    pathHint,
			Field:   "schemaVersion",
			Message: fmt.Sprintf("unsupported course schema version %q (want %q)", version, CourseSchemaVersion),
		}
	}
}

func rejectDuplicateJSONKeys(data []byte) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	return walkJSONForDupKeys(dec, "$")
}

func walkJSONForDupKeys(dec *json.Decoder, path string) error {
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	delim, ok := tok.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := map[string]struct{}{}
		for dec.More() {
			keyTok, err := dec.Token()
			if err != nil {
				return err
			}
			key, ok := keyTok.(string)
			if !ok {
				return fmt.Errorf("%s: expected object key", path)
			}
			if _, exists := seen[key]; exists {
				return fmt.Errorf("%s.%s", path, key)
			}
			seen[key] = struct{}{}
			child := path + "." + key
			if path == "$" {
				child = "$." + key
			}
			if err := walkJSONForDupKeys(dec, child); err != nil {
				return err
			}
		}
		// consume closing }
		if _, err := dec.Token(); err != nil {
			return err
		}
	case '[':
		i := 0
		for dec.More() {
			if err := walkJSONForDupKeys(dec, fmt.Sprintf("%s[%d]", path, i)); err != nil {
				return err
			}
			i++
		}
		if _, err := dec.Token(); err != nil {
			return err
		}
	}
	return nil
}

func rejectDuplicateYAMLKeys(n *yaml.Node, path string) error {
	if n == nil {
		return nil
	}
	switch n.Kind {
	case yaml.DocumentNode, yaml.SequenceNode:
		for i := range n.Content {
			childPath := path
			if n.Kind == yaml.SequenceNode {
				childPath = fmt.Sprintf("%s[%d]", path, i)
			}
			if err := rejectDuplicateYAMLKeys(n.Content[i], childPath); err != nil {
				return err
			}
		}
	case yaml.MappingNode:
		seen := map[string]struct{}{}
		for i := 0; i < len(n.Content); i += 2 {
			keyNode := n.Content[i]
			valNode := n.Content[i+1]
			key := keyNode.Value
			if _, exists := seen[key]; exists {
				if path == "" {
					return fmt.Errorf("%s", key)
				}
				return fmt.Errorf("%s.%s", path, key)
			}
			seen[key] = struct{}{}
			childPath := key
			if path != "" {
				childPath = path + "." + key
			}
			if err := rejectDuplicateYAMLKeys(valNode, childPath); err != nil {
				return err
			}
		}
	}
	return nil
}

// FormatJSONPath turns a Decode/Validate field path into a stable author-facing string.
func FormatJSONPath(parts ...string) string {
	var b strings.Builder
	for i, p := range parts {
		if p == "" {
			continue
		}
		if i > 0 && !strings.HasPrefix(p, "[") {
			b.WriteByte('.')
		}
		b.WriteString(p)
	}
	return b.String()
}
