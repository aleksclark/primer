// Package app boots the Primer Identity HTTP process: config, migrate, listen,
// and graceful shutdown. cmd/identity-server is a thin main over Run.
package app

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/aleksclark/primer/identity/internal/api"
	"github.com/aleksclark/primer/identity/internal/broker"
	"github.com/aleksclark/primer/identity/internal/brokerprovider"
	"github.com/aleksclark/primer/identity/internal/config"
	"github.com/aleksclark/primer/identity/internal/db"
	"github.com/aleksclark/primer/identity/internal/logging"
	"github.com/aleksclark/primer/identity/internal/stytch"
	"github.com/aleksclark/primer/identity/internal/stytchcache"
)

// Options customizes process bootstrap for tests.
type Options struct {
	// Config, when set, skips config.Load().
	Config *config.Config
	// Stdout is the log destination (defaults to os.Stdout).
	Stdout io.Writer
	// Listener allows tests to inject a bound listener.
	// When nil, the server listens on cfg.Addr().
	Listener net.Listener
	// SkipMigrate skips goose up (tests that manage schema themselves).
	SkipMigrate bool
	// ShutdownSignal, when set, is used instead of OS signals.
	ShutdownSignal <-chan struct{}

	// Provider injects a broker provider. Tests only; rejected in production.
	Provider brokerprovider.Provider
	// EnableBrokerForTest composes broker routes in development/test without
	// enabling the official Stytch provider. Rejected in production.
	EnableBrokerForTest bool
	// InsecureBrokerCookieForTest disables the Secure cookie flag. Rejected
	// in production. The production binary has no environment test-provider mode.
	InsecureBrokerCookieForTest bool
	// ProofKeySource overrides the CSPRNG used to mint the official proof-cache
	// HMAC key. Tests only; production always uses crypto/rand.
	ProofKeySource func([]byte) (int, error)
}

// Result is returned after a successful Run that has shut down.
type Result struct {
	Addr string
}

var (
	errProductionTestSeam = errors.New("identity app: production rejects test broker seams")
	errBrokerUnavailable  = errors.New("identity app: broker composition is unavailable")
)

// Run loads config (unless provided), composes broker dependencies when
// required, migrates, serves HTTP, and shuts down gracefully on
// SIGINT/SIGTERM or Options.ShutdownSignal.
func Run(ctx context.Context, opts Options) error {
	out := opts.Stdout
	if out == nil {
		out = os.Stdout
	}

	cfg := opts.Config
	if cfg == nil {
		var err error
		cfg, err = config.Load()
		if err != nil {
			return err
		}
	} else if err := cfg.Validate(); err != nil {
		return err
	}

	if cfg.Env == "production" && (opts.Provider != nil || opts.EnableBrokerForTest || opts.InsecureBrokerCookieForTest || opts.ProofKeySource != nil) {
		return errProductionTestSeam
	}

	composeBroker := cfg.BrokerEnabled() || opts.EnableBrokerForTest || opts.Provider != nil
	var (
		secrets  config.BrokerSecretSet
		provider brokerprovider.Provider
	)
	if composeBroker {
		if err := cfg.RequireBrokerHTTP(); err != nil {
			return err
		}
		var err error
		secrets, err = cfg.BrokerSecrets()
		if err != nil {
			return err
		}
		provider = opts.Provider
		if provider == nil {
			proofs, err := newOfficialProofCache(cfg, opts.ProofKeySource)
			if err != nil {
				return errBrokerUnavailable
			}
			official, err := stytch.NewBroker(stytch.BrokerConfig{
				Stytch:               cfg.Stytch,
				DiscoveryRedirectURL: cfg.BrokerDiscoveryRedirectURL,
				LoginRedirectURL:     cfg.BrokerLoginRedirectURL,
				SignupRedirectURL:    cfg.BrokerSignupRedirectURL,
				PublicToken:          cfg.StytchPublicToken,
				Proofs:               proofs,
			})
			if err != nil {
				return errBrokerUnavailable
			}
			provider = official
		}
	}

	logger := logging.NewJSONLogger(out, cfg.LogLevel)
	slog.SetDefault(logger)

	if !opts.SkipMigrate {
		if err := db.Migrate(ctx, cfg.DatabaseURL); err != nil {
			return fmt.Errorf("migrate: %w", err)
		}
	}

	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	apiOpts := api.Options{
		RequireBroker: composeBroker,
		BrokerHTTP: api.BrokerHTTPOptions{
			AllowedOrigin:         cfg.BrokerAllowedOrigin,
			InsecureTestCookie:    opts.InsecureBrokerCookieForTest || cfg.InsecureBrokerCookie,
			Production:            cfg.Env == "production",
			MaxRequestTargetBytes: cfg.AuthorizeTargetMaxBytes,
			PublicToken:           cfg.StytchPublicToken,
			PublicHost:            officialPublicHost(cfg),
		},
	}
	if composeBroker {
		svc, err := broker.NewService(broker.ServiceConfig{
			Pool: pool, Secrets: secrets, Provider: provider, Issuer: cfg.Issuer,
		})
		if err != nil {
			return errBrokerUnavailable
		}
		apiOpts.Broker = svc
	}

	_, handler := api.New(pool, apiOpts)

	srv := &http.Server{
		Addr:              cfg.Addr(),
		Handler:           handler,
		ReadHeaderTimeout: cfg.HTTPReadHeaderTimeout,
	}

	ln := opts.Listener
	if ln == nil {
		ln, err = net.Listen("tcp", cfg.Addr())
		if err != nil {
			return fmt.Errorf("listen: %w", err)
		}
	}
	addr := ln.Addr().String()
	logger.Info("listening", "addr", addr, "env", cfg.Env)

	errCh := make(chan error, 1)
	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	sigCtx := ctx
	stop := func() {}
	if opts.ShutdownSignal == nil {
		sigCtx, stop = signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	}
	defer stop()

	select {
	case err := <-errCh:
		return err
	case <-sigCtx.Done():
	case <-opts.ShutdownSignal:
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	logger.Info("shutdown complete", "addr", addr)
	return nil
}

func newOfficialProofCache(cfg *config.Config, source func([]byte) (int, error)) (*stytchcache.ProofCache, error) {
	if cfg == nil {
		return nil, errBrokerUnavailable
	}
	ttl := cfg.ProviderProofCacheTTL
	if ttl == 0 {
		ttl = 15 * time.Second
	}
	capacity := cfg.ProviderProofCacheCapacity
	if capacity == 0 {
		capacity = stytchcache.DefaultProofCacheCapacity
	}
	if source == nil {
		source = rand.Read
	}
	key := make([]byte, 32)
	n, err := source(key)
	if err != nil || n != len(key) {
		clear(key)
		return nil, errBrokerUnavailable
	}
	cache, err := stytchcache.NewProofCache(stytchcache.ProofConfig{
		HMACKey:  key,
		TTL:      ttl,
		Capacity: capacity,
	})
	clear(key)
	if err != nil {
		return nil, errBrokerUnavailable
	}
	return cache, nil
}

func officialPublicHost(cfg *config.Config) string {
	if cfg != nil && strings.EqualFold(cfg.Stytch.Env, "live") {
		return "api.stytch.com"
	}
	return "test.stytch.com"
}

// WaitReady polls addr until /readyz returns 200 or timeout.
func WaitReady(ctx context.Context, baseURL string) error {
	client := &http.Client{Timeout: 2 * time.Second}
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/readyz", nil)
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
			return fmt.Errorf("wait ready: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}
