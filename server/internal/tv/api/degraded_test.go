package api

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/tv/domain"
)

// unconfiguredAPI builds a TV API with no media source and no database, which
// is how the spec generator constructs it. Requests must degrade to clean
// errors rather than panicking on the nil dependencies.
func unconfiguredAPI(t *testing.T) humatest.TestAPI {
	t.Helper()
	_, testAPI := humatest.New(t)
	RegisterRoutes(testAPI, nil, Options{})
	return testAPI
}

func TestEndpointsRequiringMediaSourceReportUnavailable(t *testing.T) {
	t.Parallel()
	h := unconfiguredAPI(t)

	// Browse and sync check the media client before touching the database.
	assert.Equal(t, http.StatusServiceUnavailable, h.Get("/jellyfin/browse").Code,
		"an unconfigured Jellyfin is reported as unavailable, not as a crash")
	assert.Equal(t, http.StatusServiceUnavailable, h.Post("/jellyfin/sync", map[string]any{}).Code)
}

func TestHealthWorksWithoutDependencies(t *testing.T) {
	t.Parallel()
	h := unconfiguredAPI(t)
	assert.Equal(t, http.StatusOK, h.Get("/health").Code,
		"health must not depend on Jellyfin or the database")
}

func TestWithFreshPairingCodeRetriesOnCollision(t *testing.T) {
	t.Parallel()
	s := &Server{clock: time.Now, pairingTTL: DefaultPairingTTL}

	var attempts int
	var codes []string
	dev, err := s.withFreshPairingCode(map[string]any{}, func(v map[string]any) (*domain.Device, error) {
		attempts++
		codes = append(codes, v["pairing_code"].(string))
		if attempts < 3 {
			// A duplicate code must be retried rather than surfaced.
			return nil, &pgconn.PgError{Code: "23505"}
		}
		return &domain.Device{PairingCode: v["pairing_code"].(string)}, nil
	})

	require.NoError(t, err)
	assert.Equal(t, 3, attempts)
	assert.NotEmpty(t, dev.PairingCode)
	assert.Len(t, codes, 3)
	assert.NotEqual(t, codes[0], codes[1], "each attempt mints a new code")
}

func TestWithFreshPairingCodeGivesUpAfterRepeatedCollisions(t *testing.T) {
	t.Parallel()
	s := &Server{clock: time.Now, pairingTTL: DefaultPairingTTL}

	var attempts int
	_, err := s.withFreshPairingCode(map[string]any{}, func(map[string]any) (*domain.Device, error) {
		attempts++
		return nil, &pgconn.PgError{Code: "23505"}
	})

	require.Error(t, err)
	assert.Equal(t, pairingCodeAttempts, attempts, "retries are bounded")

	var statusErr huma.StatusError
	require.ErrorAs(t, err, &statusErr)
	assert.Equal(t, http.StatusConflict, statusErr.GetStatus())
}

func TestWithFreshPairingCodeSurfacesOtherFailures(t *testing.T) {
	t.Parallel()
	s := &Server{clock: time.Now, pairingTTL: DefaultPairingTTL}

	var attempts int
	_, err := s.withFreshPairingCode(map[string]any{}, func(map[string]any) (*domain.Device, error) {
		attempts++
		return nil, errors.New("connection reset")
	})

	require.Error(t, err)
	assert.Equal(t, 1, attempts, "a non-collision failure is not retried")
}

func TestPairingCodeExpiryUsesTheConfiguredTTL(t *testing.T) {
	t.Parallel()
	s := &Server{clock: time.Now, pairingTTL: DefaultPairingTTL}

	var got map[string]any
	_, err := s.withFreshPairingCode(map[string]any{}, func(v map[string]any) (*domain.Device, error) {
		got = v
		return &domain.Device{}, nil
	})
	require.NoError(t, err)

	expiresAt, ok := got["pairing_expires_at"].(time.Time)
	require.True(t, ok, "the expiry is stored as a timestamp")
	assert.WithinDuration(t, time.Now().UTC().Add(DefaultPairingTTL), expiresAt, time.Minute)
}
