// Package standards contains deterministic standards-catalog policy and
// translation helpers. Persistence remains owned by internal/repo; HTTP DTOs
// remain owned by internal/api.
package standards

import (
	"strings"
	"unicode"
)

// ValidSource reports whether source is one of the closed catalog source values.
func ValidSource(source string) bool {
	switch source {
	case "tennessee", "common_core", "custom":
		return true
	default:
		return false
	}
}

// ImportCode returns the stable workspace-framework key for an import. Reusing
// source and title therefore makes repeated imports idempotent.
func ImportCode(source, title string) string {
	base := Slug(title)
	if base == "" {
		base = "catalog"
	}
	return strings.ToLower(source) + "-" + base
}

// SourceFromCode maps the import key back to the public source enum.
func SourceFromCode(code string) string {
	code = strings.ToLower(code)
	switch {
	case strings.HasPrefix(code, "tennessee-"):
		return "tennessee"
	case strings.HasPrefix(code, "common_core-"):
		return "common_core"
	case strings.HasPrefix(code, "custom-"):
		return "custom"
	default:
		return "custom"
	}
}

// Slug produces a bounded, deterministic URL-safe fragment for catalog keys.
func Slug(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var out strings.Builder
	separator := false
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			out.WriteRune(r)
			separator = false
			continue
		}
		if out.Len() > 0 {
			separator = true
		}
		if separator && !strings.HasSuffix(out.String(), "-") {
			out.WriteByte('-')
		}
	}
	return strings.Trim(out.String(), "-")
}
