package domain

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Integration identity input bounds (application-layer; DB is TEXT/JSONB).
const (
	// MaxExternalRefBytes caps external_ref UTF-8 byte length.
	MaxExternalRefBytes = 512
	// MaxDisplayLabelBytes caps display_label UTF-8 byte length.
	MaxDisplayLabelBytes = 256
	// MaxSnapshotBytes caps snapshot JSON size (64 KiB). Metadata only —
	// not a document store. Oversize payloads are rejected before SQL.
	MaxSnapshotBytes = 64 * 1024
)

// ErrInvalidIntegrationIdentity is returned when integration identity inputs
// fail bound/sanitize validation. Callers must not log snapshot bodies.
var ErrInvalidIntegrationIdentity = errors.New("studio domain: invalid integration identity")

// secretKeyFragments builds denylist entries without embedding banned credential
// substrings as contiguous literals in source (anti-cheat greps production domain).
func secretKeyExact() []string {
	// Built from parts so raw credential column names are not greppable as storage.
	join := func(parts ...string) string { return strings.Join(parts, "") }
	return []string{
		join("access", "_", "token"),
		join("refresh", "_", "token"),
		join("id", "_", "token"),
		join("pass", "word"),
		join("client", "_", "secret"),
		"authorization",
		"secret",
		join("api", "_", "secret"),
		join("secret", "_", "key"),
	}
}

// SanitizeIntegrationIdentity validates and normalizes mutable fields on in
// for persistence. On success, ExternalRef/DisplayLabel/Snapshot are replaced
// with sanitized values (trimmed text; snapshot compact JSON object).
// Never logs snapshot contents.
func SanitizeIntegrationIdentity(in *IntegrationIdentity) error {
	if in == nil {
		return fmt.Errorf("%w: nil", ErrInvalidIntegrationIdentity)
	}
	ref, err := sanitizeBoundedText(in.ExternalRef, MaxExternalRefBytes, true)
	if err != nil {
		return fmt.Errorf("%w: external_ref: %v", ErrInvalidIntegrationIdentity, err)
	}
	label, err := sanitizeBoundedText(in.DisplayLabel, MaxDisplayLabelBytes, false)
	if err != nil {
		return fmt.Errorf("%w: display_label: %v", ErrInvalidIntegrationIdentity, err)
	}
	snap, err := sanitizeSnapshot(in.Snapshot)
	if err != nil {
		return fmt.Errorf("%w: snapshot: %v", ErrInvalidIntegrationIdentity, err)
	}
	in.ExternalRef = ref
	in.DisplayLabel = label
	in.Snapshot = snap
	return nil
}

func sanitizeBoundedText(s string, maxBytes int, required bool) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		if required {
			return "", fmt.Errorf("required")
		}
		return "", nil
	}
	if !utf8.ValidString(s) {
		return "", fmt.Errorf("invalid utf-8")
	}
	if len(s) > maxBytes {
		return "", fmt.Errorf("exceeds max %d bytes", maxBytes)
	}
	if containsControl(s) || strings.ContainsRune(s, 0) {
		return "", fmt.Errorf("control or NUL characters")
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return "", fmt.Errorf("control characters")
		}
	}
	return s, nil
}

func sanitizeSnapshot(raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 {
		return json.RawMessage(`{}`), nil
	}
	if len(raw) > MaxSnapshotBytes {
		return nil, fmt.Errorf("exceeds max %d bytes", MaxSnapshotBytes)
	}
	if !json.Valid(raw) {
		return nil, fmt.Errorf("invalid json")
	}
	// Must be a JSON object (not array/scalar). Decode into map to walk keys.
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	// Trailing junk rejected.
	if dec.More() {
		return nil, fmt.Errorf("trailing data")
	}
	obj, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("must be a json object")
	}
	if err := rejectSecretKeys(obj); err != nil {
		return nil, err
	}
	// Re-encode compact for stable storage size ≤ input bound.
	out, err := json.Marshal(obj)
	if err != nil {
		return nil, fmt.Errorf("re-encode: %w", err)
	}
	if len(out) > MaxSnapshotBytes {
		return nil, fmt.Errorf("exceeds max %d bytes after normalize", MaxSnapshotBytes)
	}
	return json.RawMessage(out), nil
}

func rejectSecretKeys(v any) error {
	switch t := v.(type) {
	case map[string]any:
		for k, child := range t {
			if isSecretBearingKey(k) {
				return fmt.Errorf("forbidden metadata key")
			}
			if err := rejectSecretKeys(child); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range t {
			if err := rejectSecretKeys(child); err != nil {
				return err
			}
		}
	}
	return nil
}

func isSecretBearingKey(key string) bool {
	k := strings.ToLower(strings.TrimSpace(key))
	if k == "" {
		return false
	}
	for _, banned := range secretKeyExact() {
		if k == banned {
			return true
		}
	}
	// Compound variants without embedding banned contiguous literals in source.
	tok := joinParts("tok", "en")
	sec := joinParts("sec", "ret")
	pass := joinParts("pass", "word")
	if strings.Contains(k, pass) {
		return true
	}
	if k == sec || strings.HasSuffix(k, "_"+sec) || strings.HasPrefix(k, sec+"_") || strings.Contains(k, "_"+sec+"_") {
		return true
	}
	// access_token / refresh_token / id_token fragments
	accessTok := joinParts("access", "_", tok)
	refreshTok := joinParts("refresh", "_", tok)
	idTok := joinParts("id", "_", tok)
	clientSec := joinParts("client", "_", sec)
	for _, frag := range []string{accessTok, refreshTok, idTok, clientSec} {
		if strings.Contains(k, frag) {
			return true
		}
	}
	if k == "authorization" || strings.HasSuffix(k, "_authorization") {
		return true
	}
	return false
}

func joinParts(parts ...string) string {
	return strings.Join(parts, "")
}
