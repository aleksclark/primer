// Package remoteagent is the LMS-side compatibility adapter for the
// primer-agents remote service.
//
// # Design invariants
//
//   - Default disabled: PrimerAgentsEnabled=false leaves all existing
//     Fantasy/LMS tutor/local-controller paths unchanged.
//   - AGENT_RUNTIME_ENABLED (process-local preview) is independent and unmodified.
//   - LMS product authorization must happen before any remote call.
//   - No-duplicate-after-acceptance: once a server-assigned run ID is returned,
//     retries must reconnect to that run and never start a duplicate.
//   - No static secrets, raw Stytch tokens, or long-lived stored bearer values.
//     The Identity access JWT is read from the token source at call time.
//   - No Studio S19, MCP, or Stytch SDK dependency.
//
// # Token source
//
// The adapter accepts a TokenSource that returns the current short-lived
// Identity access JWT (aud=primer-agents). Production callers read it from
// an environment variable named by PrimerAgentsTokenEnvVar.
//
// # Generated client
//
// All wire types come from github.com/aleksclark/primer/agents/client/go.
// No DTOs may be hand-written here; import only from that package.
package remoteagent

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	agentsclient "github.com/aleksclark/primer/agents/client/go"
)

// ErrDisabled is returned by Adapter methods when remote integration is not
// enabled. Callers should fall back to the legacy Fantasy/LMS path.
var ErrDisabled = errors.New("remoteagent: primer-agents integration is disabled")

// ErrNotConfigured is returned when enabled but the base URL or token source
// is absent. This is a configuration error; do not fall back to legacy.
var ErrNotConfigured = errors.New("remoteagent: primer-agents enabled but not fully configured")

// ErrLocalAuthorizationRequired is returned when callers attempt a remote
// operation without first performing LMS-local authorization. Never bypass.
var ErrLocalAuthorizationRequired = errors.New("remoteagent: local product authorization must precede remote call")

// TokenSource returns the current short-lived Identity access JWT.
// The returned token must NOT be cached across requests; each call should
// return the current in-flight token.
type TokenSource interface {
	Token(ctx context.Context) (string, error)
}

// EnvTokenSource reads the bearer token from an environment variable named
// envVar. Empty envVar means the token source is unconfigured.
// This is the production token source; never store the returned value.
type EnvTokenSource struct{ EnvVar string }

func (e EnvTokenSource) Token(_ context.Context) (string, error) {
	if e.EnvVar == "" {
		return "", fmt.Errorf("remoteagent: PRIMER_AGENTS_TOKEN_ENV_VAR is not set (Identity access token unavailable)")
	}
	tok := strings.TrimSpace(os.Getenv(e.EnvVar))
	if tok == "" {
		return "", fmt.Errorf("remoteagent: env var %q is empty (no Identity access token)", e.EnvVar)
	}
	return tok, nil
}

// Config carries the adapter configuration derived from the LMS Config.
type Config struct {
	Enabled     bool
	BaseURL     string
	Timeout     time.Duration
	TokenSource TokenSource
}

// Adapter is the LMS-side compatibility adapter for primer-agents.
// It is safe for concurrent use after construction.
type Adapter struct {
	cfg Config

	// acceptedMu protects acceptedRuns for concurrent access.
	// acceptedRuns maps a caller-supplied idempotency key to the run ID that
	// the remote service confirmed. Once present, retries must use this run ID
	// and must NOT start a new run (no-duplicate-after-acceptance invariant).
	acceptedMu   sync.RWMutex
	acceptedRuns map[string]string // idempotencyKey → remote run ID
}

// New constructs an Adapter. When cfg.Enabled is false, all methods return
// ErrDisabled and the caller must use the legacy Fantasy/LMS path.
func New(cfg Config) *Adapter {
	return &Adapter{
		cfg:          cfg,
		acceptedRuns: make(map[string]string),
	}
}

// Enabled reports whether remote integration is configured and active.
func (a *Adapter) Enabled() bool {
	return a != nil && a.cfg.Enabled
}

// client constructs a configured agents client using the current bearer token.
// Never caches the token; every call re-fetches it from the TokenSource.
func (a *Adapter) client(ctx context.Context) (*agentsclient.AgentsClient, error) {
	if !a.cfg.Enabled {
		return nil, ErrDisabled
	}
	if a.cfg.BaseURL == "" || a.cfg.TokenSource == nil {
		return nil, ErrNotConfigured
	}
	tok, err := a.cfg.TokenSource.Token(ctx)
	if err != nil {
		return nil, fmt.Errorf("remoteagent: acquire token: %w", err)
	}
	httpClient := &http.Client{Timeout: a.cfg.Timeout}
	return agentsclient.NewAgentsClient(a.cfg.BaseURL, tok,
		agentsclient.WithHTTPClient(httpClient))
}

// StartRun creates an idempotent queued run via the remote service, performing
// the no-duplicate check before any network call.
//
// Callers MUST have completed LMS-local product authorization before calling
// this method. The locallyAuthorized parameter is the evidence; false returns
// ErrLocalAuthorizationRequired and makes no remote call.
//
// idempotencyKey is scoped to the caller; the same key always returns the same
// run ID (no-duplicate-after-acceptance). inputPreview is a bounded diagnostic
// summary; raw prompts must never be passed here.
func (a *Adapter) StartRun(ctx context.Context, idempotencyKey, profile string,
	inputPreview *string, locallyAuthorized bool) (*agentsclient.RunResponse, error) {

	if !locallyAuthorized {
		return nil, ErrLocalAuthorizationRequired
	}
	if !a.cfg.Enabled {
		return nil, ErrDisabled
	}

	// No-duplicate-after-acceptance: if we already have a confirmed run ID for
	// this key, fetch it directly rather than creating a new one.
	a.acceptedMu.RLock()
	existingID, accepted := a.acceptedRuns[idempotencyKey]
	a.acceptedMu.RUnlock()

	c, err := a.client(ctx)
	if err != nil {
		return nil, err
	}

	if accepted {
		// Post-acceptance: reconnect to the existing run, never start a duplicate.
		return c.GetRun(ctx, existingID)
	}

	body := agentsclient.CreateRunJSONRequestBody{Profile: profile}
	if inputPreview != nil {
		s := *inputPreview
		body.InputPreview = &s
	}
	run, err := c.CreateRun(ctx, idempotencyKey, body)
	if err != nil {
		return nil, fmt.Errorf("remoteagent: create run: %w", err)
	}

	// Record acceptance so future retries reconnect to this run.
	a.acceptedMu.Lock()
	a.acceptedRuns[idempotencyKey] = run.Id.String()
	a.acceptedMu.Unlock()

	return run, nil
}

// GetRun returns the current status of a previously accepted run.
// Returns ErrDisabled when integration is off.
func (a *Adapter) GetRun(ctx context.Context, runID string) (*agentsclient.RunResponse, error) {
	if !a.cfg.Enabled {
		return nil, ErrDisabled
	}
	c, err := a.client(ctx)
	if err != nil {
		return nil, err
	}
	return c.GetRun(ctx, runID)
}

// CancelRun requests cancellation of an accepted run.
func (a *Adapter) CancelRun(ctx context.Context, runID string) (*agentsclient.RunResponse, error) {
	if !a.cfg.Enabled {
		return nil, ErrDisabled
	}
	c, err := a.client(ctx)
	if err != nil {
		return nil, err
	}
	return c.CancelRun(ctx, runID, agentsclient.CancelRunJSONRequestBody{})
}

// AcceptedRunID returns the remote run ID for a given idempotency key, if
// acceptance has already occurred. Returns ("", false) before acceptance.
func (a *Adapter) AcceptedRunID(idempotencyKey string) (string, bool) {
	if a == nil {
		return "", false
	}
	a.acceptedMu.RLock()
	defer a.acceptedMu.RUnlock()
	id, ok := a.acceptedRuns[idempotencyKey]
	return id, ok
}

// IsAccepted reports whether acceptance has occurred for the given key.
func (a *Adapter) IsAccepted(idempotencyKey string) bool {
	_, ok := a.AcceptedRunID(idempotencyKey)
	return ok
}
