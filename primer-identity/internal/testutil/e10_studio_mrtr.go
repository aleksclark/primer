package testutil

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/aleksclark/primer/identity/internal/domain"
	"github.com/aleksclark/primer/identity/internal/token"
)

const (
	// E10JSONRPCIDContext is the exact IB0 JSON-RPC ID hash prefix.
	E10JSONRPCIDContext = "primer.mcp.jsonrpc-id"
	// E10BindingContext is the Identity-local confirmation binding prefix.
	E10BindingContext = "primer.identity.e10.mrtr-binding"
	e10ConfirmTool    = "studio.publish.confirm"
	e10MaxIDBytes     = 128
	e10MaxJWKSBytes   = 64 * 1024
	e10StudioClientID = "studio-bff"
)

// E10ConfirmRequest is one Studio-like confirm attempt used as IB2-E10
// conformance evidence for later S19/IB8. It is not a Studio runtime.
type E10ConfirmRequest struct {
	AccessToken    string
	PublicClientID string
	HumanSubject   string
	Workspace      string
	DraftDigest    [32]byte
	Tool           string
	RequestState   string
	JSONRPCID      string
}

// E10Audit is the sanitized public confirmation audit record.
type E10Audit struct {
	PublicClientID string `json:"public_client_id,omitempty"`
	Outcome        string `json:"outcome"`
}

func (E10Audit) String() string { return "e10-audit{redacted}" }

// E10StudioMRTR is a credential-free Identity-local consumer harness.
type E10StudioMRTR struct {
	verifier *token.Verifier
	mu       sync.Mutex
	used     map[[32]byte]struct{}
	consumed map[string]struct{}
	audits   []E10Audit
}

// NewE10StudioMRTR fetches public JWKS over HTTP and builds a token.Verifier.
func NewE10StudioMRTR(ctx context.Context, client *http.Client, jwksURL, issuer, audience string, clock token.Clock) (*E10StudioMRTR, error) {
	if client == nil || strings.TrimSpace(jwksURL) == "" {
		return nil, errors.New("e10: jwks fetch required")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, jwksURL, nil)
	if err != nil {
		return nil, errors.New("e10: jwks fetch required")
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, errors.New("e10: jwks fetch required")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("e10: jwks fetch required")
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, e10MaxJWKSBytes+1))
	if err != nil || len(raw) == 0 || len(raw) > e10MaxJWKSBytes {
		return nil, errors.New("e10: jwks fetch required")
	}
	pubs, err := token.ParseJWKS(raw)
	if err != nil {
		return nil, errors.New("e10: jwks fetch required")
	}
	keyset, err := token.NewKeySet(func(context.Context) ([]domain.PublicJWK, error) { return pubs, nil })
	if err != nil {
		return nil, errors.New("e10: jwks fetch required")
	}
	if err := keyset.Refresh(ctx); err != nil {
		return nil, errors.New("e10: jwks fetch required")
	}
	verifier, err := token.NewVerifier(keyset, issuer, audience, clock, func(_ context.Context, clientID string) (token.ClientRegistration, error) {
		if clientID != e10StudioClientID {
			return token.ClientRegistration{}, token.ErrInvalid
		}
		return token.ClientRegistration{
			ClientID: clientID, Audience: audience, SubjectClass: token.KindHuman,
			Scope: "openid studio:read",
		}, nil
	})
	if err != nil {
		return nil, errors.New("e10: verifier unavailable")
	}
	return &E10StudioMRTR{
		verifier: verifier,
		used:     map[[32]byte]struct{}{},
		consumed: map[string]struct{}{},
	}, nil
}

// Confirm validates a signed human token and persists a one-use binding.
func (h *E10StudioMRTR) Confirm(ctx context.Context, req E10ConfirmRequest) (E10Audit, error) {
	if h == nil || h.verifier == nil {
		return E10Audit{Outcome: "denied"}, errors.New("e10: denied")
	}
	if err := validatePublicClientID(req.PublicClientID); err != nil {
		return E10Audit{Outcome: "denied"}, err
	}
	if req.Tool != e10ConfirmTool {
		return sanitizedDeny(req.PublicClientID), errors.New("e10: denied")
	}
	if err := boundedNoControl("workspace", req.Workspace, e10MaxIDBytes); err != nil {
		return sanitizedDeny(req.PublicClientID), err
	}
	if err := boundedNoControl("requestState", req.RequestState, e10MaxIDBytes); err != nil {
		return sanitizedDeny(req.PublicClientID), err
	}
	if _, err := E10JSONRPCIDHash(req.JSONRPCID); err != nil {
		return sanitizedDeny(req.PublicClientID), err
	}
	principal, err := h.verifier.Verify(ctx, req.AccessToken)
	if err != nil {
		return sanitizedDeny(req.PublicClientID), errors.New("e10: denied")
	}
	if principal.Kind != token.KindHuman || principal.Subject != req.HumanSubject || principal.ClientID != req.PublicClientID {
		return sanitizedDeny(req.PublicClientID), errors.New("e10: denied")
	}
	digest := h.BindingDigest(req)
	slot := principal.Subject + "\x00" + principal.ClientID
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, seen := h.used[digest]; seen {
		audit := E10Audit{PublicClientID: req.PublicClientID, Outcome: "denied"}
		h.audits = append(h.audits, audit)
		return audit, errors.New("e10: denied")
	}
	if _, seen := h.consumed[slot]; seen {
		audit := E10Audit{PublicClientID: req.PublicClientID, Outcome: "denied"}
		h.audits = append(h.audits, audit)
		return audit, errors.New("e10: denied")
	}
	h.used[digest] = struct{}{}
	h.consumed[slot] = struct{}{}
	audit := E10Audit{PublicClientID: req.PublicClientID, Outcome: "success"}
	h.audits = append(h.audits, audit)
	return audit, nil
}

// BindingDigest HMAC-binds human, public client_id, workspace, draft, tool,
// and requestState. A new JSON-RPC ID does not change the digest.
func (h *E10StudioMRTR) BindingDigest(req E10ConfirmRequest) [32]byte {
	mac := hmac.New(sha256.New, []byte(E10BindingContext))
	writeLP(mac, []byte(req.HumanSubject))
	writeLP(mac, []byte(req.PublicClientID))
	writeLP(mac, []byte(req.Workspace))
	writeLP(mac, req.DraftDigest[:])
	writeLP(mac, []byte(req.Tool))
	writeLP(mac, []byte(req.RequestState))
	var out [32]byte
	copy(out[:], mac.Sum(nil))
	return out
}

// Audits returns sanitized confirmation records only.
func (h *E10StudioMRTR) Audits() []E10Audit {
	if h == nil {
		return nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]E10Audit, len(h.audits))
	copy(out, h.audits)
	return out
}

// E10JSONRPCIDHash is SHA-256("primer.mcp.jsonrpc-id" || 0x01 || uint32be(len) || UTF8(id)).
func E10JSONRPCIDHash(id string) ([32]byte, error) {
	var out [32]byte
	if err := boundedNoControl("jsonrpc-id", id, e10MaxIDBytes); err != nil {
		return out, err
	}
	h := sha256.New()
	_, _ = h.Write([]byte(E10JSONRPCIDContext))
	_, _ = h.Write([]byte{0x01})
	var lenBuf [4]byte
	binary.BigEndian.PutUint32(lenBuf[:], uint32(len(id)))
	_, _ = h.Write(lenBuf[:])
	_, _ = h.Write([]byte(id))
	copy(out[:], h.Sum(nil))
	return out, nil
}

func sanitizedDeny(clientID string) E10Audit {
	if validatePublicClientID(clientID) != nil {
		return E10Audit{Outcome: "denied"}
	}
	return E10Audit{PublicClientID: clientID, Outcome: "denied"}
}

func validatePublicClientID(clientID string) error {
	if err := domain.ValidatePublicClientID(clientID); err != nil {
		return errors.New("e10: denied")
	}
	if _, err := uuid.Parse(clientID); err == nil {
		return errors.New("e10: denied")
	}
	return nil
}

func boundedNoControl(name, value string, max int) error {
	if value == "" || len(value) > max || !utf8.ValidString(value) {
		return fmt.Errorf("e10: denied")
	}
	if strings.IndexFunc(value, func(r rune) bool { return r < 0x20 || r == 0x7f }) >= 0 {
		return fmt.Errorf("e10: denied")
	}
	_ = name
	return nil
}

func writeLP(w io.Writer, value []byte) {
	var lenBuf [4]byte
	binary.BigEndian.PutUint32(lenBuf[:], uint32(len(value)))
	_, _ = w.Write(lenBuf[:])
	_, _ = w.Write(value)
}
