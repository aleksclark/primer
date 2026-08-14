package domain

import (
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

// Field length bounds enforced at the store/domain boundary (claim-size defense).
const (
	MaxProviderSubjectLen = 255
	MaxEmailLen           = 320
	MaxDisplayNameLen     = 200
)

// Account status values persisted on accounts.status.
const (
	AccountStatusActive = "active"
	AccountStatusLocked = "locked"
)

// Known external-identity providers. Extend carefully in later waves.
const (
	ProviderGoogle     = "google"
	ProviderPassword   = "password"
	ProviderBreakglass = "breakglass"
)

// Account is the Identity principal. ID is the stable JWT `sub` (UUID).
// Students are intentionally not Identity principals in v1.
type Account struct {
	ID                      uuid.UUID  `json:"id" db:"id"`
	Status                  string     `json:"status" db:"status"`
	DisplayName             string     `json:"display_name" db:"display_name"`
	PrimaryEmail            *string    `json:"primary_email,omitempty" db:"primary_email"`
	PrimaryEmailVerifiedAt  *time.Time `json:"primary_email_verified_at,omitempty" db:"primary_email_verified_at"`
	CreatedAt               time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt               time.Time  `json:"updated_at" db:"updated_at"`
}

// ExternalIdentity links one (provider, provider_subject) pair to an account.
// Uniqueness is solely on (provider, provider_subject) — never email.
type ExternalIdentity struct {
	ID              uuid.UUID `json:"id" db:"id"`
	AccountID       uuid.UUID `json:"account_id" db:"account_id"`
	Provider        string    `json:"provider" db:"provider"`
	ProviderSubject string    `json:"provider_subject" db:"provider_subject"`
	CreatedAt       time.Time `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time `json:"updated_at" db:"updated_at"`
}

// PasswordCredential holds the KDF hash for an account (migration/break-glass).
// Plaintext is never stored.
type PasswordCredential struct {
	AccountID    uuid.UUID  `json:"account_id" db:"account_id"`
	PasswordHash string     `json:"-" db:"password_hash"`
	Algorithm    string     `json:"algorithm" db:"algorithm"`
	RotatedAt    time.Time  `json:"rotated_at" db:"rotated_at"`
	DisabledAt   *time.Time `json:"disabled_at,omitempty" db:"disabled_at"`
}

// CreateAccountInput is validated at the store boundary before insert.
type CreateAccountInput struct {
	DisplayName  string
	PrimaryEmail *string
	Status       string // empty → active
}

// ValidateCreateAccount checks display name, optional email, and status.
func ValidateCreateAccount(in CreateAccountInput) error {
	if err := ValidateDisplayName(in.DisplayName); err != nil {
		return err
	}
	if in.PrimaryEmail != nil {
		if err := ValidateEmail(*in.PrimaryEmail); err != nil {
			return err
		}
	}
	status := in.Status
	if status == "" {
		status = AccountStatusActive
	}
	if status != AccountStatusActive && status != AccountStatusLocked {
		return invalidf("status", "unsupported account status %q", status)
	}
	return nil
}

// ValidateProviderSubject enforces non-empty, length, UTF-8, and no controls.
func ValidateProviderSubject(s string) error {
	if s == "" {
		return invalidf("provider_subject", "must not be empty")
	}
	if err := validateTextField("provider_subject", s, MaxProviderSubjectLen); err != nil {
		return err
	}
	return nil
}

// ValidateEmail enforces non-empty, length, UTF-8, and no controls.
// Email is a display/recovery field only — not a login key and not unique.
func ValidateEmail(s string) error {
	if s == "" {
		return invalidf("email", "must not be empty")
	}
	if err := validateTextField("email", s, MaxEmailLen); err != nil {
		return err
	}
	return nil
}

// ValidateDisplayName allows empty (optional) but rejects oversize/controls/bad UTF-8.
func ValidateDisplayName(s string) error {
	if s == "" {
		return nil
	}
	return validateTextField("display_name", s, MaxDisplayNameLen)
}

// ValidateProvider checks known provider tokens.
func ValidateProvider(p string) error {
	switch p {
	case ProviderGoogle, ProviderPassword, ProviderBreakglass:
		return nil
	case "":
		return invalidf("provider", "must not be empty")
	default:
		return invalidf("provider", "unsupported provider %q", p)
	}
}

func validateTextField(field, s string, max int) error {
	if !utf8.ValidString(s) {
		return invalidf(field, "must be valid UTF-8")
	}
	if utf8.RuneCountInString(s) > max {
		return invalidf(field, "exceeds max length %d", max)
	}
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return invalidf(field, "must not contain control characters")
		}
	}
	return nil
}
