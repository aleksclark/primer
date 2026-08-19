package logging_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

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

func TestRedactingHandlerWithAttrsAndWithGroup(t *testing.T) {
	// Exercises WithAttrs and WithGroup code paths.
	var buf bytes.Buffer
	base := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	h := logging.NewRedactingHandler(base)

	// WithAttrs — must redact secret keys.
	h2 := h.WithAttrs([]slog.Attr{
		slog.String("password", "should-hide"),
		slog.String("normal", "visible"),
	})
	rec := slog.NewRecord(time.Now(), slog.LevelInfo, "test", 0)
	_ = h2.Handle(context.Background(), rec)
	assert.NotContains(t, buf.String(), "should-hide")

	// WithGroup wraps the handler.
	buf.Reset()
	h3 := h.WithGroup("mygroup")
	rec2 := slog.NewRecord(time.Now(), slog.LevelInfo, "grp", 0)
	rec2.AddAttrs(slog.String("secret", "hide-me"))
	_ = h3.Handle(context.Background(), rec2)
	assert.NotContains(t, buf.String(), "hide-me")
}
