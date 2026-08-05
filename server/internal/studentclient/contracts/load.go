package contracts

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// LoadDocument reads an activity document from a YAML or JSON file and validates it.
func LoadDocument(path string) (*ActivityDocument, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read activity %s: %w", path, err)
	}
	doc, err := ParseDocument(data, path)
	if err != nil {
		return nil, err
	}
	if err := ValidateDocument(doc); err != nil {
		return nil, fmt.Errorf("validate %s: %w", path, err)
	}
	return doc, nil
}

// ParseDocument unmarshals YAML or JSON based on the path extension (or content).
// Decoding is strict: unknown fields and duplicate keys are rejected.
func ParseDocument(data []byte, pathHint string) (*ActivityDocument, error) {
	ext := strings.ToLower(filepath.Ext(pathHint))
	switch ext {
	case ".json":
		return DecodeActivityJSON(data, pathHint)
	default:
		// YAML default; also used when extension is missing.
		if ext == ".yaml" || ext == ".yml" || ext == "" {
			return DecodeActivityYAML(data, pathHint)
		}
		// Non-json with unknown extension: try YAML then JSON.
		if doc, err := DecodeActivityYAML(data, pathHint); err == nil {
			return doc, nil
		}
		return DecodeActivityJSON(data, pathHint)
	}
}

// LoadCourseDocument reads and validates a course manifest JSON file.
func LoadCourseDocument(path string) (*CourseDocument, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read course %s: %w", path, err)
	}
	doc, err := DecodeCourseJSON(data, path)
	if err != nil {
		return nil, err
	}
	if err := ValidateCourseDocument(doc); err != nil {
		return nil, fmt.Errorf("validate %s: %w", path, err)
	}
	return doc, nil
}

// LoadDocumentsDir loads every activity.yaml/yml/json under immediate subdirectories
// of root (curriculum/activities layout).
func LoadDocumentsDir(root string) ([]*ActivityDocument, []error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, []error{fmt.Errorf("read activities dir %s: %w", root, err)}
	}
	var docs []*ActivityDocument
	var errs []error
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(root, e.Name())
		path, err := findActivityFile(dir)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		doc, err := LoadDocument(path)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if doc.Slug != e.Name() {
			errs = append(errs, fmt.Errorf("%s: slug %q must match directory name %q", path, doc.Slug, e.Name()))
			continue
		}
		docs = append(docs, doc)
	}
	return docs, errs
}

func findActivityFile(dir string) (string, error) {
	candidates := []string{
		filepath.Join(dir, "activity.yaml"),
		filepath.Join(dir, "activity.yml"),
		filepath.Join(dir, "activity.json"),
	}
	for _, c := range candidates {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return c, nil
		}
	}
	return "", fmt.Errorf("no activity.yaml/yml/json in %s", dir)
}

// MustJSON returns the canonical JSON encoding of v or panics (tests only).
func MustJSON(v any) []byte {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		panic(err)
	}
	b = append(b, '\n')
	return b
}
