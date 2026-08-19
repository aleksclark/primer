//go:build live_stytch

package live_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/identity/internal/brokerprovider"
	"github.com/aleksclark/primer/identity/internal/stytch"
)

// Live broker qualification against the approved Stytch *test* project.
// This exercises the browser/BFF boundary: Broker construction, StartLogin
// (email magic-link, email OTP, SSO URL construction), and secret redaction.
//
// Full CompleteTypedCallback requires a real one-time callback token from a
// browser flow. If IDENTITY_LIVE_STYTCH_SESSION_TOKEN is set, the optional
// session-authenticated callback proof also runs.
//
// Not IB8-E10 (full browser + webhook end-to-end).

func TestLiveBrokerConstruction(t *testing.T) {
	cfg := loadLiveBrokerConfig(t)
	broker, err := stytch.NewBroker(cfg)
	require.NoError(t, err)
	require.NotNil(t, broker)
	t.Log("broker_construction=ok")
}

func TestLiveBrokerStartLoginEmailMagicLink(t *testing.T) {
	cfg := loadLiveBrokerConfig(t)
	broker, err := stytch.NewBroker(cfg)
	require.NoError(t, err)

	// Send a discovery magic-link email. The test project accepts the API call
	// even for unknown emails (email enumeration is not Stytch's default).
	// Note: if the magic-link product is not enabled on the test project, or
	// the redirect URL is not registered, the API returns a 4xx which our
	// adapter maps to ErrProviderUnavailable. We document that residual.
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	result, err := broker.StartLogin(ctx, brokerprovider.StartRequest{
		Method:       brokerprovider.MethodEmailMagicLink,
		EmailAddress: "live-proof-" + randomHex(t, 8) + "@primer-test.example",
	})
	if err != nil {
		// If the project doesn't have magic links enabled or the redirect URL
		// isn't registered, this is expected. Document and skip.
		if errors.Is(err, brokerprovider.ErrProviderUnavailable) {
			t.Skipf("magic-link start returned unavailable (project config residual): %v", err)
		}
		require.NoError(t, err)
	}
	require.Equal(t, brokerprovider.MethodEmailMagicLink, result.Method)
	require.NotEmpty(t, result.Handle)
	require.Empty(t, result.ContinueURL, "magic-link start must not produce a continue URL")

	// Verify no secret leakage in handle or result
	stytchCfg := cfg.Stytch
	require.NotContains(t, result.Handle, stytchCfg.Secret)
	require.NotContains(t, fmt.Sprintf("%v", result), stytchCfg.Secret)
	t.Log("start_email_magic_link=ok")
}

func TestLiveBrokerStartLoginEmailOTP(t *testing.T) {
	cfg := loadLiveBrokerConfig(t)
	broker, err := stytch.NewBroker(cfg)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	result, err := broker.StartLogin(ctx, brokerprovider.StartRequest{
		Method:       brokerprovider.MethodEmailOTP,
		EmailAddress: "live-proof-otp-" + randomHex(t, 8) + "@primer-test.example",
	})
	require.NoError(t, err)
	require.Equal(t, brokerprovider.MethodEmailOTP, result.Method)
	require.NotEmpty(t, result.Handle)
	require.Empty(t, result.ContinueURL, "OTP start must not produce a continue URL")
	require.NotContains(t, fmt.Sprintf("%v", result), cfg.Stytch.Secret)
	t.Log("start_email_otp=ok")
}

func TestLiveBrokerStartLoginSSOURLConstruction(t *testing.T) {
	cfg := loadLiveBrokerConfig(t)
	broker, err := stytch.NewBroker(cfg)
	require.NoError(t, err)

	// SSO start produces a public URL without a network call to the Stytch
	// backend. This proves the URL construction is correct with real config.
	ctx := context.Background()
	result, err := broker.StartLogin(ctx, brokerprovider.StartRequest{
		Method:       brokerprovider.MethodSSOSAML,
		ConnectionID: "saml-connection-test-" + randomHex(t, 4),
	})
	require.NoError(t, err)
	require.Equal(t, brokerprovider.MethodSSOSAML, result.Method)
	require.NotEmpty(t, result.Handle)
	require.NotEmpty(t, result.ContinueURL, "SSO start must produce a public continue URL")

	// Verify it targets the Stytch test environment
	require.True(t, strings.HasPrefix(result.ContinueURL, "https://test.stytch.com/"),
		"SSO URL must target the test environment, got: %s", result.ContinueURL)
	require.Contains(t, result.ContinueURL, "public_token="+cfg.PublicToken)
	require.Contains(t, result.ContinueURL, "connection_id=")

	// No secrets in the URL
	require.NotContains(t, result.ContinueURL, cfg.Stytch.Secret)
	require.NotContains(t, result.ContinueURL, cfg.Stytch.ProjectID)
	t.Logf("sso_start_url=%s", result.ContinueURL)
}

func TestLiveBrokerStartLoginSSOByOrganization(t *testing.T) {
	cfg := loadLiveBrokerConfig(t)
	broker, err := stytch.NewBroker(cfg)
	require.NoError(t, err)

	ctx := context.Background()
	result, err := broker.StartLogin(ctx, brokerprovider.StartRequest{
		Method:         brokerprovider.MethodSSOOIDC,
		OrganizationID: "organization-test-" + randomHex(t, 4),
	})
	require.NoError(t, err)
	require.Equal(t, brokerprovider.MethodSSOOIDC, result.Method)
	require.Contains(t, result.ContinueURL, "https://test.stytch.com/")
	require.Contains(t, result.ContinueURL, "organization_id=")
	require.NotContains(t, result.ContinueURL, cfg.Stytch.Secret)
	t.Log("sso_org_start=ok")
}

func TestLiveBrokerSecretRedaction(t *testing.T) {
	cfg := loadLiveBrokerConfig(t)
	broker, err := stytch.NewBroker(cfg)
	require.NoError(t, err)

	// The Broker's contained Adapter has Format/String/GoString methods that
	// redact secrets. Verify that fmt.Sprintf with the adapter (which is what
	// logs/errors reference) doesn't leak. The Broker struct itself doesn't
	// implement fmt.Stringer — that's fine because it's never directly printed
	// in production paths; only the adapter and config String() are used.
	//
	// The StytchConfig.String() must also redact:
	s := cfg.Stytch.String()
	require.NotContains(t, s, cfg.Stytch.Secret)
	s = fmt.Sprintf("%v", cfg.Stytch)
	require.NotContains(t, s, cfg.Stytch.Secret)
	s = fmt.Sprintf("%s", cfg.Stytch)
	require.NotContains(t, s, cfg.Stytch.Secret)

	// Verify the broker is non-nil and constructed correctly
	_ = broker
	t.Log("broker_redaction=ok")
}

func TestLiveBrokerInvalidEmailRejectedBeforeNetwork(t *testing.T) {
	cfg := loadLiveBrokerConfig(t)
	broker, err := stytch.NewBroker(cfg)
	require.NoError(t, err)

	ctx := context.Background()
	_, err = broker.StartLogin(ctx, brokerprovider.StartRequest{
		Method:       brokerprovider.MethodEmailMagicLink,
		EmailAddress: "", // empty
	})
	require.Error(t, err)
	require.ErrorIs(t, err, brokerprovider.ErrDefinitiveDenial)

	_, err = broker.StartLogin(ctx, brokerprovider.StartRequest{
		Method:       brokerprovider.MethodEmailOTP,
		EmailAddress: "no-at-sign",
	})
	require.Error(t, err)
	require.ErrorIs(t, err, brokerprovider.ErrDefinitiveDenial)
	t.Log("invalid_email_pre_network=ok")
}

func TestLiveBrokerUnsupportedMethodRejected(t *testing.T) {
	cfg := loadLiveBrokerConfig(t)
	broker, err := stytch.NewBroker(cfg)
	require.NoError(t, err)

	_, err = broker.StartLogin(context.Background(), brokerprovider.StartRequest{
		Method: "password", // not allowed
	})
	require.ErrorIs(t, err, brokerprovider.ErrUnsupportedMethod)
	t.Log("unsupported_method=ok")
}

// loadLiveBrokerConfig builds a BrokerConfig from the test env credentials.
// Hard gate: test project only, test environment only.
func loadLiveBrokerConfig(t *testing.T) stytch.BrokerConfig {
	t.Helper()

	stytchCfg := loadLiveStytchConfig(t) // reuse from stytch_live_test.go

	publicToken := strings.TrimSpace(os.Getenv("IDENTITY_STYTCH_PUBLIC_TOKEN"))
	if publicToken == "" {
		publicToken = "public-token-test-placeholder"
		t.Log("IDENTITY_STYTCH_PUBLIC_TOKEN not set, using placeholder (SSO URL tests only prove shape)")
	}

	// Use the Stytch test API as the SSO/callback target. For live proof we
	// don't actually complete callbacks, we just prove the Broker can talk to
	// the real Stytch test API for start flows.
	discoveryRedirect := "https://id.primer-test.example/broker/stytch/callback"
	loginRedirect := "https://id.primer-test.example/broker/stytch/callback"
	signupRedirect := "https://id.primer-test.example/broker/stytch/callback"

	return stytch.BrokerConfig{
		Stytch:               stytchCfg,
		DiscoveryRedirectURL: discoveryRedirect,
		LoginRedirectURL:     loginRedirect,
		SignupRedirectURL:    signupRedirect,
		PublicToken:          publicToken,
	}
}
