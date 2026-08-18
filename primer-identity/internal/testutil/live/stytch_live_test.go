//go:build live_stytch

package live_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/identity/internal/config"
	"github.com/aleksclark/primer/identity/internal/stytch"
)

// Live provider qualification against the approved Stytch *test* project.
// Not IB8-E10 (browser + webhook). Explicit build tag + env gate only.

func TestLiveStytchProviderQualification(t *testing.T) {
	cfg := loadLiveStytchConfig(t)
	client, err := stytch.New(cfg)
	require.NoError(t, err)
	require.NotNil(t, client, "enabled adapter must construct")

	adapter, ok := client.(*stytch.Adapter)
	require.True(t, ok, "production New must return *stytch.Adapter")

	t.Run("empty_token_rejected_before_network", func(t *testing.T) {
		_, err := client.AuthenticateSession(context.Background(), "")
		require.ErrorIs(t, err, stytch.ErrEmptyToken)
		require.False(t, errors.Is(err, stytch.ErrDefinitive))
	})

	t.Run("oversized_token_rejected_before_network", func(t *testing.T) {
		huge := strings.Repeat("a", 4097)
		_, err := client.AuthenticateSession(context.Background(), huge)
		require.ErrorIs(t, err, stytch.ErrSessionTokenTooLarge)
	})

	t.Run("garbage_session_token_is_definitive_or_unavailable", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		token := "session-live-qual-" + randomHex(t, 16)
		_, err := client.AuthenticateSession(ctx, token)
		require.Error(t, err)
		// Official provider 4xx definitive kinds unwrap to ErrDefinitive;
		// transport/5xx map to non-oracular failure strings. Either is
		// acceptable; success is not.
		require.NotContains(t, err.Error(), cfg.Secret)
		require.NotContains(t, fmt.Sprintf("%v", err), cfg.Secret)
		require.NotContains(t, fmt.Sprintf("%#v", err), cfg.Secret)
		// Prefer definitive when the API classifies the token; allow
		// unavailable-style wrap if the provider shape differs.
		if !errors.Is(err, stytch.ErrDefinitive) {
			require.Contains(t, err.Error(), "stytch session authenticate failed")
		}
		t.Logf("garbage_token_class=%s", errorClass(err))
	})

	t.Run("revalidate_unknown_tuple_is_definitive_or_unavailable", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		_, err := adapter.RevalidateMemberSession(ctx,
			cfg.ProjectID,
			"organization-test-live-qual-missing",
			"member-test-live-qual-missing",
			"member-session-test-live-qual-missing",
		)
		require.Error(t, err)
		require.True(t,
			errors.Is(err, stytch.ErrDefinitive) || errors.Is(err, stytch.ErrProviderUnavailable),
			"want definitive or unavailable, got %v", err,
		)
		require.NotContains(t, fmt.Sprintf("%v", err), cfg.Secret)
		t.Logf("unknown_tuple_class=%s", errorClass(err))
	})

	t.Run("revalidate_project_mismatch_is_unavailable_without_network_claim", func(t *testing.T) {
		_, err := adapter.RevalidateMemberSession(context.Background(),
			"project-test-other-not-configured",
			"organization-test-x",
			"member-test-x",
			"member-session-test-x",
		)
		require.ErrorIs(t, err, stytch.ErrProviderUnavailable)
	})

	t.Run("adapter_string_redacts_secret", func(t *testing.T) {
		s := adapter.String()
		require.Contains(t, s, "stytch-adapter")
		require.NotContains(t, s, cfg.Secret)
		require.NotContains(t, fmt.Sprintf("%v", adapter), cfg.Secret)
		require.NotContains(t, fmt.Sprintf("%#v", adapter), cfg.Secret)
		require.NotContains(t, fmt.Sprintf("%s", cfg), cfg.Secret)
	})

	if tok := strings.TrimSpace(os.Getenv("IDENTITY_LIVE_STYTCH_SESSION_TOKEN")); tok != "" {
		t.Run("optional_live_session_authenticate", func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			snap, err := client.AuthenticateSession(ctx, tok)
			require.NoError(t, err)
			require.Equal(t, cfg.ProjectID, snap.ProjectID)
			require.NotEmpty(t, snap.OrganizationID)
			require.NotEmpty(t, snap.MemberID)
			require.NotEmpty(t, snap.ProviderMemberSessionID)
			require.True(t, snap.ExpiresAt.After(time.Now().UTC().Add(-time.Minute)))
			// No email/JWT/token fields exist on the snapshot type; assert
			// IDs stay control-free bounded text.
			for _, id := range []string{snap.OrganizationID, snap.MemberID, snap.ProviderMemberSessionID} {
				require.True(t, utf8.ValidString(id))
				require.NotContains(t, id, "\n")
				for _, r := range id {
					require.False(t, unicode.IsControl(r))
				}
			}
			// Revalidate the exact tuple without presenting the token again.
			again, err := adapter.RevalidateMemberSession(ctx,
				snap.ProjectID, snap.OrganizationID, snap.MemberID, snap.ProviderMemberSessionID)
			require.NoError(t, err)
			require.Equal(t, snap.OrganizationID, again.OrganizationID)
			require.Equal(t, snap.MemberID, again.MemberID)
			require.Equal(t, snap.ProviderMemberSessionID, again.ProviderMemberSessionID)
			require.True(t, again.Active && again.Eligible)
			t.Log("optional_live_session_authenticate=ok")
		})
	} else {
		t.Log("optional_live_session_authenticate=skipped (set IDENTITY_LIVE_STYTCH_SESSION_TOKEN to exercise happy path)")
	}
}

func loadLiveStytchConfig(t *testing.T) config.StytchConfig {
	t.Helper()
	if os.Getenv("IDENTITY_LIVE_STYTCH") != "1" {
		t.Fatal("IDENTITY_LIVE_STYTCH=1 is required (refusing ambient credential use)")
	}

	if path := strings.TrimSpace(os.Getenv("IDENTITY_LIVE_STYTCH_ENV_FILE")); path != "" {
		loadEnvFile(t, path)
	}

	projectID := strings.TrimSpace(os.Getenv("IDENTITY_STYTCH_PROJECT_ID"))
	secret := strings.TrimSpace(os.Getenv("IDENTITY_STYTCH_SECRET"))
	envName := strings.ToLower(strings.TrimSpace(os.Getenv("IDENTITY_STYTCH_ENV")))
	if envName == "" {
		envName = "test"
	}
	if projectID == "" || secret == "" {
		t.Fatal("IDENTITY_STYTCH_PROJECT_ID and IDENTITY_STYTCH_SECRET are required")
	}
	// Hard gate: this harness only talks to the Stytch test environment.
	// Production/live project credentials are refused here even if present.
	if envName != "test" {
		t.Fatalf("IDENTITY_STYTCH_ENV must be test for this harness, got %q", envName)
	}
	if !strings.HasPrefix(projectID, "project-test-") {
		t.Fatal("IDENTITY_STYTCH_PROJECT_ID must use project-test- prefix")
	}

	cfg := config.StytchConfig{
		Enabled:               true,
		ProjectID:             projectID,
		Secret:                secret,
		Env:                   envName,
		RequestTimeout:        3 * time.Second,
		PositiveCacheTTL:      15 * time.Second,
		NegativeCacheTTL:      5 * time.Second,
		PositiveCacheCapacity: 10000,
		NegativeCacheCapacity: 2000,
	}
	require.NoError(t, cfg.Validate())
	t.Logf("live_stytch_project_prefix=%s env=%s secret_len=%d", projectID[:min(20, len(projectID))], envName, len(secret))
	return cfg
}

func loadEnvFile(t *testing.T, path string) {
	t.Helper()
	abs, err := filepath.Abs(path)
	require.NoError(t, err)
	data, err := os.ReadFile(abs)
	require.NoError(t, err)
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		val = strings.Trim(val, `"'`)
		if key == "" {
			continue
		}
		// Do not override an already-exported value.
		if os.Getenv(key) == "" {
			require.NoError(t, os.Setenv(key, val))
		}
	}
}

func randomHex(t *testing.T, n int) string {
	t.Helper()
	b := make([]byte, n)
	_, err := rand.Read(b)
	require.NoError(t, err)
	return hex.EncodeToString(b)
}

func errorClass(err error) string {
	switch {
	case errors.Is(err, stytch.ErrDefinitive):
		return "definitive"
	case errors.Is(err, stytch.ErrProviderUnavailable):
		return "unavailable"
	case errors.Is(err, stytch.ErrEmptyToken):
		return "empty_token"
	case errors.Is(err, stytch.ErrSessionTokenTooLarge):
		return "token_too_large"
	default:
		return "other"
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
