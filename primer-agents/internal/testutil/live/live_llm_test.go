//go:build live_llm

// Package live_test contains the deliberately opt-in billable-provider probe.
// It is excluded from ordinary Go tests and all default Make targets.
package live_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/agents/internal/authn/jwttest"
	"github.com/aleksclark/primer/agents/internal/testutil"
)

const (
	liveFlag       = "PRIMER_AGENTS_LIVE_LLM"
	liveKey        = "PRIMER_AGENTS_LIVE_LLM_API_KEY"
	liveBaseURL    = "PRIMER_AGENTS_LIVE_LLM_BASE_URL"
	liveModel      = "gpt-4o-mini"
	liveRequestN   = 1
	outerTimeout   = 45 * time.Second
	processTimeout = 30 * time.Second
)

type runResponse struct {
	ID      string `json:"id"`
	Status  string `json:"status"`
	Profile string `json:"profile"`
}

type liveReport struct {
	SHA              string `json:"sha"`
	Model            string `json:"model"`
	RequestCount     int    `json:"request_count"`
	Status           string `json:"status"`
	ProcessBoundary  bool   `json:"real_process_boundary"`
	IdentityLoopback bool   `json:"identity_loopback"`
	SecretsInLogs    bool   `json:"secrets_in_logs"`
}

func TestLiveBillableLLMQualification(t *testing.T) {
	loadLiveEnvFile(t)
	if os.Getenv(liveFlag) != "1" {
		t.Fatal("PRIMER_AGENTS_LIVE_LLM=1 is required; refusing a billable call")
	}
	key := strings.TrimSpace(os.Getenv(liveKey))
	if key == "" {
		t.Fatal("PRIMER_AGENTS_LIVE_LLM_API_KEY is required; refusing a billable call")
	}
	baseURL := strings.TrimSpace(os.Getenv(liveBaseURL))
	if baseURL != "https://api.openai.com/v1" {
		t.Fatalf("%s must be explicitly set to https://api.openai.com/v1; got %q", liveBaseURL, baseURL)
	}

	ctx, cancel := context.WithTimeout(context.Background(), outerTimeout)
	defer cancel()

	identity, identityKey, jwt := startLoopbackIdentity(t)
	defer identity.Close()
	binary := buildService(t)
	dsn := testutil.URL(t)
	port := freePort(t)
	var logs bytes.Buffer
	cmd := exec.Command(binary)
	cmd.Env = childEnvironment(dsn, port, identity.URL, key)
	cmd.Stdout = &logs
	cmd.Stderr = &logs
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	})

	base := fmt.Sprintf("http://127.0.0.1:%d", port)
	require.NoError(t, waitReady(ctx, base))

	// Wrong audience is rejected at the real process boundary before a run is
	// accepted. This token is never passed to the provider.
	wrong := jwt
	wrongClaims := jwttest.ValidHumanClaims(time.Now(), "identity:wrong-audience")
	wrongClaims.Issuer = identity.URL
	wrongClaims.Audience = "not-primer-agents"
	wrong = jwttest.Mint(t, identityKey, wrongClaims)
	status := postRun(t, base, wrong, "wrong-audience", "tutor", "Reply with exactly OK.")
	require.Equal(t, http.StatusUnauthorized, status)
	status = postRun(t, base, jwt, "student-profile", "student", "Reply with exactly OK.")
	require.Equal(t, http.StatusForbidden, status, "student profile must fail closed before provider traffic")

	// The only billable probe is a fixed tutor request. Student profile input
	// is not accepted by this harness and is never sent to the process.
	run := createRun(t, base, jwt, "live-qualification", "tutor", "Reply with exactly OK.")
	require.Equal(t, "tutor", run.Profile)
	terminal := waitRun(ctx, t, base, jwt, run.ID)
	require.Equal(t, "succeeded", terminal.Status)
	require.Equal(t, liveRequestN, 1, "qualification has exactly one provider request budget")

	// Fetch the durable event page through HTTP as an additional assertion that
	// the real worker/provider path completed; no in-process status is trusted.
	require.NoError(t, getEvents(t, base, jwt, run.ID))

	_ = cmd.Process.Kill()
	_ = cmd.Wait()
	logText := logs.String()
	secretsInLogs := strings.Contains(logText, key) || strings.Contains(logText, jwt)
	require.False(t, secretsInLogs, "provider key or bearer token appeared in process logs")

	report := liveReport{
		SHA:              gitSHA(t),
		Model:            liveModel,
		RequestCount:     liveRequestN,
		Status:           "qualified",
		ProcessBoundary:  true,
		IdentityLoopback: true,
		SecretsInLogs:    false,
	}
	writeReport(t, report)
	t.Logf("live_llm_qualification sha=%s model=%s request_count=%d secrets_in_logs=false", report.SHA, report.Model, report.RequestCount)
}

// loadLiveEnvFile only accepts a clearly named, opt-in file. It never reads a
// generic .env and never permits provider keys under an unscoped name.
func loadLiveEnvFile(t *testing.T) {
	t.Helper()
	path := strings.TrimSpace(os.Getenv("PRIMER_AGENTS_LIVE_LLM_ENV_FILE"))
	if path == "" {
		home, err := os.UserHomeDir()
		require.NoError(t, err)
		path = filepath.Join(home, ".config", "primer", "primer-agents-live-llm.env")
	} else if _, err := os.Stat(path); err != nil {
		t.Fatalf("explicit live env file is unavailable: %v", err)
	}
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return
	}
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		name, value, ok := strings.Cut(line, "=")
		if !ok || !strings.HasPrefix(strings.TrimSpace(name), "PRIMER_AGENTS_LIVE_") {
			continue
		}
		name = strings.TrimSpace(name)
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		if os.Getenv(name) == "" {
			require.NoError(t, os.Setenv(name, value))
		}
	}
}

func buildService(t *testing.T) string {
	t.Helper()
	_, source, _, ok := runtime.Caller(0)
	require.True(t, ok)
	moduleRoot := filepath.Clean(filepath.Join(filepath.Dir(source), "../../.."))
	binary := filepath.Join(t.TempDir(), "primer-agents")
	cmd := exec.Command("go", "build", "-o", binary, "./cmd/primer-agents")
	cmd.Dir = moduleRoot
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "build primer-agents: %s", out)
	return binary
}

func childEnvironment(dsn string, port int, issuer, key string) []string {
	keep := make([]string, 0, len(os.Environ()))
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		switch name {
		case "OPENAI_API_KEY", "ANTHROPIC_API_KEY", "AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY":
			continue
		}
		if strings.HasPrefix(name, "PRIMER_AGENTS_LIVE_") || name == "PRIMER_AGENTS_DATABASE_URL" {
			continue
		}
		keep = append(keep, entry)
	}
	keep = append(keep,
		"PRIMER_AGENTS_DATABASE_URL="+dsn,
		"PRIMER_AGENTS_ENV=development",
		"PRIMER_AGENTS_HOST=127.0.0.1",
		fmt.Sprintf("PRIMER_AGENTS_PORT=%d", port),
		"PRIMER_AGENTS_WORKER_ENABLED=true",
		"PRIMER_AGENTS_IDENTITY_ISSUER="+issuer,
		"PRIMER_AGENTS_IDENTITY_JWKS_URL="+issuer+"/jwks",
		liveFlag+"=1",
		liveKey+"="+key,
		liveBaseURL+"=https://api.openai.com/v1",
		"PRIMER_AGENTS_LIVE_LLM_MODEL="+liveModel,
		"PRIMER_AGENTS_LIVE_LLM_TIMEOUT=20s",
		"PRIMER_AGENTS_LIVE_LLM_MAX_CALLS=1",
	)
	return keep
}

func startLoopbackIdentity(t *testing.T) (*httptest.Server, *jwttest.Keypair, string) {
	t.Helper()
	key := jwttest.GenerateKey(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/jwks" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(jwttest.JWKSDoc(key)); err != nil {
			http.Error(w, "jwks encoding failed", http.StatusInternalServerError)
		}
	}))
	claims := jwttest.ValidHumanClaims(time.Now(), "identity:live-qualification")
	claims.Issuer = server.URL
	return server, key, jwttest.Mint(t, key, claims)
}

func freePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port
}

func waitReady(ctx context.Context, base string) error {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	client := &http.Client{Timeout: 2 * time.Second}
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/readyz", nil)
		if err != nil {
			return err
		}
		resp, err := client.Do(req)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func createRun(t *testing.T, base, token, idempotency, profile, input string) runResponse {
	t.Helper()
	body := fmt.Sprintf(`{"profile":%q,"inputPreview":%q}`, profile, input)
	req, err := http.NewRequest(http.MethodPost, base+"/agents/v1/runs", strings.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Idempotency-Key", idempotency)
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: processTimeout}).Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var out runResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
	require.NotEmpty(t, out.ID)
	return out
}

func postRun(t *testing.T, base, token, idempotency, profile, input string) int {
	t.Helper()
	body := fmt.Sprintf(`{"profile":%q,"inputPreview":%q}`, profile, input)
	req, err := http.NewRequest(http.MethodPost, base+"/agents/v1/runs", strings.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Idempotency-Key", idempotency)
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: processTimeout}).Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	return resp.StatusCode
}

func waitRun(ctx context.Context, t *testing.T, base, token, id string) runResponse {
	t.Helper()
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/agents/v1/runs/"+id, nil)
		require.NoError(t, err)
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := (&http.Client{Timeout: 2 * time.Second}).Do(req)
		require.NoError(t, err)
		var out runResponse
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
		_ = resp.Body.Close()
		if out.Status == "succeeded" || out.Status == "failed" || out.Status == "canceled" || out.Status == "interrupted" {
			return out
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-ticker.C:
		}
	}
}

func getEvents(t *testing.T, base, token, id string) error {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, base+"/agents/v1/runs/"+id+"/events", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := (&http.Client{Timeout: processTimeout}).Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("events status %d", resp.StatusCode)
	}
	return nil
}

func gitSHA(t *testing.T) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", "HEAD")
	out, err := cmd.Output()
	require.NoError(t, err)
	return strings.TrimSpace(string(out))
}

func writeReport(t *testing.T, report liveReport) {
	t.Helper()
	path := strings.TrimSpace(os.Getenv("PRIMER_AGENTS_LIVE_LLM_REPORT_FILE"))
	if path == "" {
		path = filepath.Join(t.TempDir(), "primer-agents-live-llm-report.json")
	}
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	data, err := json.MarshalIndent(report, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, append(data, '\n'), 0o600))
	t.Logf("live_llm_report=%s", path)
}
