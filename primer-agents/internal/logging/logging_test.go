package logging_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/agents/internal/logging"
)

func TestNewJSONLoggerEmitsStructuredFields(t *testing.T) {
	var buf bytes.Buffer
	logger := logging.NewJSONLogger(&buf, "info")
	logger.Info("hello", "path", "/healthz", "request_id", "rid-1")

	var entry map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &entry))
	assert.Equal(t, "INFO", entry["level"])
	assert.Equal(t, "hello", entry["msg"])
	assert.Equal(t, "/healthz", entry["path"])
	assert.Equal(t, "rid-1", entry["request_id"])
}

func TestRedactingHandlerScrubsDSNPassword(t *testing.T) {
	var buf bytes.Buffer
	logger := logging.NewJSONLogger(&buf, "info")
	const sentinel = "super-secret-pass-RED-LOCK-42"
	secretDSN := "postgres://agents:" + sentinel + "@localhost:5432/primer_agents?sslmode=disable"
	logger.Info("boot", "database_url", secretDSN)

	out := buf.String()
	assert.NotContains(t, out, sentinel)
	assert.Contains(t, out, "database_url")
	var entry map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &entry))
	dsn, _ := entry["database_url"].(string)
	assert.True(t,
		strings.Contains(dsn, "REDACTED") || strings.Contains(dsn, "%5BREDACTED%5D"),
		"expected redacted marker in DSN, got %q", dsn)
}

func TestRedactingHandlerScrubsSecretKeys(t *testing.T) {
	var buf bytes.Buffer
	base := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	logger := slog.New(logging.NewRedactingHandler(base))
	logger.Info("auth", "password", "hunter2", "client_secret", "cs-xyz", "token", "tok-abc")

	out := buf.String()
	assert.NotContains(t, out, "hunter2")
	assert.NotContains(t, out, "cs-xyz")
	assert.NotContains(t, out, "tok-abc")
	assert.Contains(t, out, "[REDACTED]")
}

func TestRedactingHandlerScrubsErrorAttr(t *testing.T) {
	var buf bytes.Buffer
	logger := logging.NewJSONLogger(&buf, "info")
	const sentinel = "err-secret-ZZ9"
	err := fmt.Errorf("connect: postgres://agents:%s@localhost:5432/primer_agents: refused", sentinel)
	logger.Error("fatal", "error", err)

	out := buf.String()
	assert.NotContains(t, out, sentinel)
	assert.Contains(t, out, "REDACTED")
}

func TestRedactingHandlerScrubsDSNInWrappedError(t *testing.T) {
	var buf bytes.Buffer
	logger := logging.NewJSONLogger(&buf, "info")
	const sentinel = "wrapped-secret-MM4"
	inner := fmt.Errorf("open postgres://agents:%s@localhost/primer_agents: boom", sentinel)
	logger.Error("migrate", "error", fmt.Errorf("migrate: %w", inner))

	out := buf.String()
	assert.NotContains(t, out, sentinel)
	assert.Contains(t, out, "REDACTED")
}
