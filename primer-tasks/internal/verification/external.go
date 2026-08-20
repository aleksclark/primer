package verification

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const (
	ExternalCallbackKind          = "external_callback"
	ExternalCallbackConfigVersion = 1
	ExternalCallbackSchemaVersion = "external_callback.v1"
)

var (
	ErrExternalInvalidSignature = errors.New("external callback signature is invalid")
	ErrExternalReplay           = errors.New("external callback replayed")
	ErrExternalBinding          = errors.New("external callback binding is invalid")
	ErrExternalExpired          = errors.New("external callback is expired")
)

type ExternalBinding struct{ TenantID, AttemptID, OccurrenceID, RequirementID, VerifierID, RequestID, PayloadDigest, CallbackPath, SchemaVersion, SecretVersion string }

type ExternalConfig struct {
	VerifierID string         `json:"verifierId"`
	Capability string         `json:"capability"`
	Schema     string         `json:"schemaVersion"`
	Options    map[string]any `json:"options,omitempty"`
}

var publicOptionKey = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,31}$`)

func (c ExternalConfig) Validate() error {
	if strings.TrimSpace(c.VerifierID) == "" || strings.TrimSpace(c.Capability) == "" {
		return errors.New("external verifier and capability are required")
	}
	if c.Schema == "" {
		return errors.New("external schema version is required")
	}
	return ValidatePublicOptions(c.Options)
}

// ValidatePublicOptions is the verifier protocol's public-options boundary.
// Options are deliberately limited to small scalar values with safe field
// names; endpoints, headers, credentials, and opaque nested documents belong
// to the administrator catalog, never to a parent task revision.
func ValidatePublicOptions(options map[string]any) error {
	if len(options) > 32 {
		return errors.New("external public options contain too many fields")
	}
	for key, value := range options {
		lower := strings.ToLower(key)
		if !publicOptionKey.MatchString(key) || strings.Contains(lower, "url") || strings.Contains(lower, "endpoint") || strings.Contains(lower, "header") || strings.Contains(lower, "secret") || strings.Contains(lower, "token") || strings.Contains(lower, "auth") || strings.Contains(lower, "password") || strings.Contains(lower, "credential") {
			return errors.New("external public option name is not allowed")
		}
		switch value.(type) {
		case nil, string, bool, float64, float32, int, int32, int64, uint, uint32, uint64:
		default:
			return errors.New("external public options must be scalar")
		}
	}
	return nil
}

type RequestEnvelope struct {
	Version        int             `json:"version"`
	RequestID      string          `json:"requestId"`
	AttemptRef     string          `json:"attemptRef"`
	RequirementRef string          `json:"requirementRef"`
	SchemaVersion  string          `json:"schemaVersion"`
	ExpiresAt      time.Time       `json:"expiresAt"`
	IdempotencyKey string          `json:"idempotencyKey"`
	CallbackPath   string          `json:"callbackPath"`
	PayloadDigest  string          `json:"payloadDigest"`
	Payload        json.RawMessage `json:"payload"`
	AllowedHandles []string        `json:"allowedHandles,omitempty"`
}

func (e RequestEnvelope) Validate(now time.Time) error {
	if e.Version != 1 || e.RequestID == "" || e.AttemptRef == "" || e.RequirementRef == "" || e.SchemaVersion == "" || e.IdempotencyKey == "" || e.CallbackPath == "" || len(e.Payload) == 0 {
		return errors.New("invalid external request envelope")
	}
	if e.ExpiresAt.Before(now) {
		return ErrExternalExpired
	}
	h := sha256.Sum256(e.Payload)
	if !hmac.Equal([]byte(strings.ToLower(e.PayloadDigest)), []byte(hex.EncodeToString(h[:]))) {
		return errors.New("external request payload digest mismatch")
	}
	return nil
}

type ProgressResult struct {
	Code      string `json:"code"`
	Message   string `json:"message,omitempty"`
	Percent   int    `json:"percent,omitempty"`
	Retryable bool   `json:"retryable,omitempty"`
}
type AcceptedResult struct {
	Rationale string `json:"rationale,omitempty"`
}
type RejectedResult struct {
	Rationale string `json:"rationale,omitempty"`
}
type ErrorResult struct {
	Code      string `json:"code"`
	Message   string `json:"message,omitempty"`
	Retryable bool   `json:"retryable,omitempty"`
}

type CallbackEnvelope struct {
	Version       int             `json:"version"`
	CallbackID    string          `json:"callbackId"`
	RequestID     string          `json:"requestId"`
	AttemptRef    string          `json:"attemptRef"`
	VerifierID    string          `json:"verifierId"`
	SchemaVersion string          `json:"schemaVersion"`
	Sequence      int64           `json:"sequence"`
	RequestDigest string          `json:"requestDigest"`
	Type          string          `json:"type"`
	Progress      *ProgressResult `json:"progress,omitempty"`
	Accepted      *AcceptedResult `json:"accepted,omitempty"`
	Rejected      *RejectedResult `json:"rejected,omitempty"`
	Error         *ErrorResult    `json:"error,omitempty"`
}

func (c CallbackEnvelope) Validate() error {
	if c.Version != 1 || c.CallbackID == "" || c.RequestID == "" || c.AttemptRef == "" || c.VerifierID == "" || c.SchemaVersion == "" || c.Sequence < 1 || c.RequestDigest == "" {
		return errors.New("invalid external callback envelope")
	}
	switch c.Type {
	case "progress":
		if c.Progress == nil || strings.TrimSpace(c.Progress.Code) == "" || c.Progress.Percent < 0 || c.Progress.Percent > 100 {
			return errors.New("invalid external progress result")
		}
	case "accepted":
		if c.Accepted == nil {
			return errors.New("missing accepted result")
		}
	case "rejected":
		if c.Rejected == nil {
			return errors.New("missing rejected result")
		}
	case "retryable_error":
		if c.Error == nil || !c.Error.Retryable {
			return errors.New("invalid retryable error result")
		}
	case "terminal_error":
		if c.Error == nil || c.Error.Retryable {
			return errors.New("invalid terminal error result")
		}
	default:
		return errors.New("unknown external callback result type")
	}
	return nil
}

func NewRequestEnvelope(requestID, attemptRef, requirementRef, schemaVersion, idempotencyKey, callbackPath string, payload any, now time.Time, ttl time.Duration, handles []string) (RequestEnvelope, error) {
	b, err := json.Marshal(payload)
	if err != nil {
		return RequestEnvelope{}, err
	}
	if requestID == "" {
		requestID = uuid.NewString()
	}
	e := RequestEnvelope{Version: 1, RequestID: requestID, AttemptRef: attemptRef, RequirementRef: requirementRef, SchemaVersion: schemaVersion, ExpiresAt: now.UTC().Add(ttl), IdempotencyKey: idempotencyKey, CallbackPath: callbackPath, PayloadDigest: ExternalPayloadDigest(b), Payload: b, AllowedHandles: handles}
	return e, e.Validate(now)
}
func MarshalRequestEnvelope(e RequestEnvelope) ([]byte, error) { return json.Marshal(e) }

func ExternalPayloadDigest(payload []byte) string {
	h := sha256.Sum256(payload)
	return hex.EncodeToString(h[:])
}

// CanonicalSignatureInput deliberately includes the HTTP method and path. A
// signature copied to another endpoint or method therefore cannot be replayed.
func CanonicalSignatureInput(method, path string, timestamp time.Time, bodyDigest, requestID string) string {
	return strings.ToUpper(strings.TrimSpace(method)) + "\n" + path + "\n" + timestamp.UTC().Format(time.RFC3339Nano) + "\n" + bodyDigest + "\n" + requestID
}
func Sign(method, path string, timestamp time.Time, requestID string, body, secret []byte) string {
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(CanonicalSignatureInput(method, path, timestamp, ExternalPayloadDigest(body), requestID)))
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}
func Verify(method, path string, timestamp time.Time, requestID, signature string, body, secret []byte, now time.Time, maxSkew time.Duration) error {
	if len(body) > 1<<20 || requestID == "" || maxSkew <= 0 || timestamp.Before(now.Add(-maxSkew)) || timestamp.After(now.Add(maxSkew)) {
		return ErrExternalInvalidSignature
	}
	want := Sign(method, path, timestamp, requestID, body, secret)
	if len(signature) != len(want) || !hmac.Equal([]byte(want), []byte(signature)) {
		return ErrExternalInvalidSignature
	}
	return nil
}

func SignRequest(method, path string, timestamp time.Time, requestID string, body, secret []byte) string {
	return Sign(method, path, timestamp, requestID, body, secret)
}
func VerifyRequest(method, path string, timestamp time.Time, requestID, signature string, body, secret []byte, now time.Time, maxSkew time.Duration) error {
	return Verify(method, path, timestamp, requestID, signature, body, secret, now, maxSkew)
}

func ValidateEndpoint(raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return errors.New("verifier endpoint must be an HTTPS URL without credentials, query, or fragment")
	}
	host := strings.ToLower(u.Hostname())
	if host == "localhost" || strings.HasPrefix(host, "127.") || strings.HasPrefix(host, "10.") || strings.HasPrefix(host, "192.168.") || strings.HasPrefix(host, "169.254.") || host == "::1" || strings.HasPrefix(host, "fc") || strings.HasPrefix(host, "fd") {
		return errors.New("verifier endpoint targets a private or loopback address")
	}
	return nil
}

func DecodeRequest(body []byte) (RequestEnvelope, error) {
	var e RequestEnvelope
	if len(body) == 0 || len(body) > 1<<20 {
		return e, errors.New("request envelope body is invalid")
	}
	if err := json.Unmarshal(body, &e); err != nil {
		return e, fmt.Errorf("decode request envelope: %w", err)
	}
	return e, e.Validate(time.Now().UTC())
}

// HandleExternalCallback is the only callback-to-decision bridge. Progress and
// errors are durable protocol events but can never mutate verification state.
func HandleExternalCallback(ctx context.Context, committer DecisionCommitter, binding ExternalBinding, c CallbackEnvelope) (bool, error) {
	if err := c.Validate(); err != nil {
		return false, err
	}
	if c.AttemptRef != binding.AttemptID || c.VerifierID != binding.VerifierID || c.RequestID != binding.RequestID || c.RequestDigest != binding.PayloadDigest {
		return false, ErrExternalBinding
	}
	if c.Type != "accepted" && c.Type != "rejected" {
		return false, nil
	}
	reason := "external verifier rejected result"
	if c.Type == "accepted" {
		reason = "external verifier accepted result"
	}
	return CommitDecision(ctx, committer, Decision{ID: uuid.NewString(), TenantID: binding.TenantID, AttemptID: binding.AttemptID, OccurrenceID: binding.OccurrenceID, Accepted: c.Type == "accepted", Reason: reason, DecidedBy: "external_verifier"})
}

func DecodeCallback(body []byte) (CallbackEnvelope, error) {
	var c CallbackEnvelope
	if len(body) == 0 || len(body) > 1<<20 {
		return c, errors.New("callback body is invalid")
	}
	if err := json.Unmarshal(body, &c); err != nil {
		return c, fmt.Errorf("decode callback: %w", err)
	}
	return c, c.Validate()
}
