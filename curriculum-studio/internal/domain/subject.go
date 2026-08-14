package domain

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
)

// Subject ref conventions (DB stores opaque text; validation is application-layer).
//
// Canonical forms (persisted and used for lookup):
//   - human:   identity:<uuid>   where <uuid> is lowercase RFC-4122 hex
//   - service: identity:svc:<id> where <id> is a validated token (case-preserving)
//
// Prefix matching is case-insensitive on input; CanonicalSubjectRef.String() always
// emits the lowercase prefixes above. Human UUID bodies are always lowercased.
// Service IDs preserve caller case (opaque tokens may be case-sensitive upstream)
// but the prefix is always identity:svc:.
const (
	// HumanSubjectPrefix is the canonical prefix for human Identity subjects.
	HumanSubjectPrefix = "identity:"
	// ServiceSubjectPrefix is the canonical prefix for service principals.
	ServiceSubjectPrefix = "identity:svc:"
	// MaxSubjectRefLen caps subject_ref size (DB is TEXT; app enforces bound).
	MaxSubjectRefLen = 256
	// MaxServiceSubjectIDLen caps the service id portion after identity:svc:.
	MaxServiceSubjectIDLen = 128
)

// ErrInvalidSubject is returned when a subject_ref fails canonical validation.
var ErrInvalidSubject = errors.New("studio domain: invalid subject_ref")

// CanonicalSubjectRef is the parsed, normalized membership subject identity.
// String() always returns the deterministic form suitable for persistence/lookup.
type CanonicalSubjectRef struct {
	// Kind is SubjectKindHuman or SubjectKindService.
	Kind string
	// ID is the opaque id portion: lowercase uuid string (human) or service token.
	ID string
}

// String returns the canonical wire/storage form.
func (c CanonicalSubjectRef) String() string {
	switch c.Kind {
	case SubjectKindHuman:
		return HumanSubjectPrefix + c.ID
	case SubjectKindService:
		return ServiceSubjectPrefix + c.ID
	default:
		return ""
	}
}

// HumanSubjectRef builds identity:<uuid> for a human Identity account sub.
// The UUID is always lowercased (uuid.UUID.String()).
func HumanSubjectRef(id uuid.UUID) string {
	return HumanSubjectPrefix + id.String()
}

// ServiceSubjectRef builds identity:svc:<id> for a service principal.
// id must already be a non-empty safe token (validated by Parse/Validate).
// The service id token is case-preserving; only the prefix is fixed.
func ServiceSubjectRef(id string) (string, error) {
	id = strings.TrimSpace(id)
	if err := validateServiceID(id); err != nil {
		return "", err
	}
	return ServiceSubjectPrefix + id, nil
}

// CanonicalizeSubjectRef parses and normalizes a subject_ref for persistence/lookup.
// Human UUID case variants and identity:/IDENTITY: prefix case variants collapse
// to one form so unique indexes cannot be bypassed by case.
// (Named Canonicalize* because CanonicalSubjectRef is the result type.)
func CanonicalizeSubjectRef(ref string) (CanonicalSubjectRef, error) {
	id, kind, err := ParseSubjectRef(ref)
	if err != nil {
		return CanonicalSubjectRef{}, err
	}
	return CanonicalSubjectRef{Kind: kind, ID: id}, nil
}

// ValidateSubjectRef checks canonical human or service subject_ref form.
// kind, when non-empty, must match the inferred kind (human|service).
func ValidateSubjectRef(ref string, kind string) error {
	canon, err := CanonicalizeSubjectRef(ref)
	if err != nil {
		return err
	}
	if kind != "" && kind != canon.Kind {
		return fmt.Errorf("%w: kind %q does not match ref kind %q", ErrInvalidSubject, kind, canon.Kind)
	}
	return nil
}

// ParseSubjectRef validates and returns the opaque id portion and subject kind.
// Human → lowercase uuid string + "human"; service → service id + "service".
// Accepts case-insensitive prefixes (IDENTITY: / IDENTITY:SVC:).
func ParseSubjectRef(ref string) (id string, kind string, err error) {
	if ref == "" {
		return "", "", fmt.Errorf("%w: empty", ErrInvalidSubject)
	}
	if strings.TrimSpace(ref) != ref {
		return "", "", fmt.Errorf("%w: leading/trailing whitespace", ErrInvalidSubject)
	}
	if !utf8.ValidString(ref) {
		return "", "", fmt.Errorf("%w: invalid utf-8", ErrInvalidSubject)
	}
	if len(ref) > MaxSubjectRefLen {
		return "", "", fmt.Errorf("%w: exceeds max length %d", ErrInvalidSubject, MaxSubjectRefLen)
	}
	if containsControl(ref) {
		return "", "", fmt.Errorf("%w: control characters", ErrInvalidSubject)
	}

	lower := strings.ToLower(ref)
	switch {
	case strings.HasPrefix(lower, ServiceSubjectPrefix):
		// Preserve original id casing after the prefix bytes of equal length.
		// Prefix length is ASCII-stable (identity:svc:).
		svcID := ref[len(ServiceSubjectPrefix):]
		if err := validateServiceID(svcID); err != nil {
			return "", "", err
		}
		return svcID, SubjectKindService, nil
	case strings.HasPrefix(lower, HumanSubjectPrefix):
		// Reject identity:svc: leaking into human branch is already handled by
		// checking service prefix first. Remaining must be a bare UUID.
		raw := ref[len(HumanSubjectPrefix):]
		if raw == "" {
			return "", "", fmt.Errorf("%w: missing human uuid", ErrInvalidSubject)
		}
		// Disallow nested prefixes like identity:identity:...
		if strings.Contains(raw, ":") {
			return "", "", fmt.Errorf("%w: human ref must be identity:<uuid>", ErrInvalidSubject)
		}
		u, perr := uuid.Parse(raw)
		if perr != nil {
			return "", "", fmt.Errorf("%w: human uuid: %v", ErrInvalidSubject, perr)
		}
		if u == uuid.Nil {
			return "", "", fmt.Errorf("%w: nil uuid", ErrInvalidSubject)
		}
		// uuid.UUID.String() is always lowercase canonical hex.
		return u.String(), SubjectKindHuman, nil
	default:
		return "", "", fmt.Errorf("%w: must start with %q or %q", ErrInvalidSubject, HumanSubjectPrefix, ServiceSubjectPrefix)
	}
}

func validateServiceID(id string) error {
	if id == "" {
		return fmt.Errorf("%w: empty service id", ErrInvalidSubject)
	}
	if strings.TrimSpace(id) != id {
		return fmt.Errorf("%w: service id whitespace", ErrInvalidSubject)
	}
	if len(id) > MaxServiceSubjectIDLen {
		return fmt.Errorf("%w: service id exceeds max length %d", ErrInvalidSubject, MaxServiceSubjectIDLen)
	}
	if containsControl(id) {
		return fmt.Errorf("%w: service id control characters", ErrInvalidSubject)
	}
	// Allow common opaque service tokens: alnum, dash, underscore, dot.
	for _, r := range id {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' || r == '.' {
			continue
		}
		return fmt.Errorf("%w: service id has disallowed character %q", ErrInvalidSubject, r)
	}
	return nil
}

func containsControl(s string) bool {
	for _, r := range s {
		if unicode.IsControl(r) {
			return true
		}
	}
	return false
}
