package api_test

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/aleksclark/primer/identity/internal/api"
	"github.com/aleksclark/primer/identity/internal/broker"
	"github.com/aleksclark/primer/identity/internal/brokerprovider"
	"github.com/aleksclark/primer/identity/internal/testutil"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// Inspect only the new diagnostic record's fields, never whole access/service
// logs. An unexpected field or arbitrary string fails without printing it.
type safeDiagnosticLog struct {
	mu      sync.Mutex
	invalid bool
	records int
}

func (h *safeDiagnosticLog) Enabled(context.Context, slog.Level) bool { return true }
func (h *safeDiagnosticLog) WithAttrs([]slog.Attr) slog.Handler       { return h }
func (h *safeDiagnosticLog) WithGroup(string) slog.Handler            { return h }
func (h *safeDiagnosticLog) Handle(_ context.Context, r slog.Record) error {
	if r.Message != "broker_callback_failure" {
		return nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records++
	r.Attrs(func(a slog.Attr) bool {
		switch a.Key {
		case "stage":
			if a.Value.Kind() != slog.KindString || !closedDiagnosticValue(a.Value.String(), "input|cookie_hash|binding|advance|provider|release|tuple_mapping|mapping_lookup|code_generation|code_hash|lifetime|issuance|state_recovery|redirect|unknown") {
				h.invalid = true
			}
		case "class":
			if a.Value.Kind() != slog.KindString || !closedDiagnosticValue(a.Value.String(), "unknown|serialization_conflict|deadlock|not_found|conflict|invalid|canceled|deadline|capacity|query_canceled|unavailable|denied|incomplete") {
				h.invalid = true
			}
		case "attempts", "limit", "mapping_calls":
			if a.Value.Kind() != slog.KindInt64 || a.Value.Int64() < 0 || a.Value.Int64() > 15 {
				h.invalid = true
			}
		case "exhausted":
			if a.Value.Kind() != slog.KindBool {
				h.invalid = true
			}
		default:
			h.invalid = true
		}
		return true
	})
	return nil
}

func TestCallbackAttributionPrivateAndRequestScoped(t *testing.T) {
	old := slog.Default()
	logs := &safeDiagnosticLog{}
	slog.SetDefault(slog.New(logs))
	t.Cleanup(func() { slog.SetDefault(old) })
	svc := newBrokerService(t, scripted(t, successFixture(uniqueLabel("unused"), brokerprovider.MethodEmailMagicLink)))
	handler, _ := newBrokerAPI(t, svc)
	server := httptest.NewServer(handler)
	defer server.Close()
	address := strings.TrimPrefix(server.URL, "http://")
	type result struct {
		status                   int
		got                      broker.CallbackFailure
		captured, leaked, failed bool
		wantStage                string
	}
	results := make(chan result, 16)
	for i := 0; i < 16; i++ {
		go func(i int) {
			rid := uuid.NewString()
			capture, release := api.CaptureBrokerCallbackForTest(address, rid)
			defer release()
			marker := "DO_NOT_LOG_" + uuid.NewString()
			path := "/broker/stytch/callback"
			stage := "input"
			if i%2 != 0 {
				path += "?token=" + url.QueryEscape(marker) + "&type=discovery_magic_link"
				stage = "binding"
			}
			req, e := http.NewRequest("GET", server.URL+path, nil)
			if e != nil {
				results <- result{failed: true}
				return
			}
			req.Header.Set("X-Request-ID", rid)
			if i%2 != 0 {
				req.Header.Set("Cookie", broker.BrokerCookieName+"="+marker)
			}
			response, e := http.DefaultClient.Do(req)
			if e != nil {
				results <- result{failed: true}
				return
			}
			raw, e := io.ReadAll(response.Body)
			response.Body.Close()
			d, ok := capture.Snapshot()
			projection, _ := json.Marshal(d)
			leaked := strings.Contains(d.String(), marker) || strings.Contains(d.String(), rid) || strings.Contains(string(projection), marker) || strings.Contains(string(raw), marker) || strings.Contains(string(raw), "stage")
			results <- result{response.StatusCode, d, ok, leaked, e != nil, stage}
		}(i)
	}
	for i := 0; i < 16; i++ {
		r := <-results
		require.False(t, r.failed)
		require.False(t, r.leaked, "unsafe diagnostic/response content")
		require.Equal(t, 400, r.status)
		require.True(t, r.captured)
		require.Equal(t, r.wantStage, r.got.Stage())
		if r.wantStage == "binding" {
			require.Equal(t, "not_found", r.got.Class())
		} else {
			require.Equal(t, "invalid", r.got.Class())
		}
	}
	logs.mu.Lock()
	defer logs.mu.Unlock()
	require.Equal(t, 16, logs.records)
	require.False(t, logs.invalid, "diagnostic log contained non-allowlisted data")
}

func closedDiagnosticValue(value, allowed string) bool {
	for _, candidate := range strings.Split(allowed, "|") {
		if value == candidate {
			return true
		}
	}
	return false
}

func TestCallbackAttributionPreservesPublicLocalErrorResponses(t *testing.T) {
	marker := "DO_NOT_LOG_provider_cookie_state_client_" + uuid.NewString()
	artifact := uniqueLabel("random-failure")
	var failRand atomic.Bool
	provider := scripted(t, successFixture(artifact, brokerprovider.MethodEmailMagicLink))
	svc, err := broker.NewService(broker.ServiceConfig{Pool: testutil.DB(t), Secrets: brokerSecrets(t), Issuer: testIssuer, Provider: provider, Rand: func(b []byte) error {
		if failRand.Load() {
			return errors.New(marker)
		}
		_, err := rand.Read(b)
		return err
	}})
	require.NoError(t, err)
	clientID := uniqueLabel("diagnostic")
	redirect, resource, audience := registerHTTPClient(t, clientID)
	start, err := svc.Authorize(context.Background(), broker.AuthorizeRequest{ClientID: clientID, RedirectURI: redirect, ResourceURI: resource, Audience: audience, Scopes: []string{"openid"}, State: []byte(marker), CodeChallenge: pkce, CodeMethod: "S256"})
	require.NoError(t, err)
	handler, _ := newBrokerAPI(t, svc)
	server := httptest.NewServer(handler)
	defer server.Close()
	address := strings.TrimPrefix(server.URL, "http://")
	call := func(path, cookie string) ([]byte, http.Header, broker.CallbackFailure) {
		rid := uuid.NewString()
		capture, release := api.CaptureBrokerCallbackForTest(address, rid)
		defer release()
		req, err := http.NewRequest("GET", server.URL+path, nil)
		require.NoError(t, err)
		req.Header.Set("X-Request-ID", rid)
		if cookie != "" {
			req.Header.Set("Cookie", broker.BrokerCookieName+"="+cookie)
		}
		response, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer response.Body.Close()
		require.Equal(t, 400, response.StatusCode)
		raw, err := io.ReadAll(response.Body)
		require.NoError(t, err)
		d, ok := capture.Snapshot()
		require.True(t, ok)
		for _, secret := range []string{marker, cookie, artifact, rid} {
			if secret != "" && (strings.Contains(string(raw), secret) || strings.Contains(d.String(), secret)) {
				t.Fatal("unsafe response or diagnostic")
			}
		}
		require.Equal(t, "no-store", response.Header.Get("Cache-Control"))
		require.Equal(t, "no-cache", response.Header.Get("Pragma"))
		require.Equal(t, "no-referrer", response.Header.Get("Referrer-Policy"))
		require.Equal(t, "nosniff", response.Header.Get("X-Content-Type-Options"))
		require.Empty(t, response.Header.Get("Location"))
		require.Empty(t, response.Header.Get("Set-Cookie"))
		return raw, response.Header, d
	}
	input, h1, d1 := call("/broker/stytch/callback", "")
	require.Equal(t, "input", d1.Stage())
	binding, h2, d2 := call("/broker/stytch/callback?token=unknown&type=discovery_magic_link", "unknown-cookie")
	require.Equal(t, "binding", d2.Stage())
	failRand.Store(true)
	generation, h3, d3 := call("/broker/stytch/callback?token="+url.QueryEscape(artifact)+"&type=discovery_magic_link", start.CookieValue)
	require.Equal(t, "code_generation", d3.Stage())
	require.Equal(t, "unknown", d3.Class())
	require.True(t, string(input) == string(binding) && string(input) == string(generation), "internal causes must not become a public oracle")
	require.Equal(t, h1.Get("Content-Security-Policy"), h2.Get("Content-Security-Policy"))
	require.Equal(t, h1.Get("Content-Security-Policy"), h3.Get("Content-Security-Policy"))
	// Existing success/503/trusted-denial HTTP tests remain the external contract
	// checks for those classes; this deliberately compares indistinguishable400s.
}
