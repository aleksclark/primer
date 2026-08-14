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

	"github.com/aleksclark/primer/curriculum-studio/internal/logging"
)

func TestNewJSONLoggerEmitsStructuredFields(t *testing.T) {
	var buf bytes.Buffer
	logger := logging.NewJSONLogger(&buf, "info")
	logger.Info("hello", "path", "/studio/v1/health", "request_id", "rid-1")

	var entry map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &entry))
	assert.Equal(t, "INFO", entry["level"])
	assert.Equal(t, "hello", entry["msg"])
	assert.Equal(t, "/studio/v1/health", entry["path"])
	assert.Equal(t, "rid-1", entry["request_id"])
}

func TestRedactingHandlerScrubsDSNPasswords(t *testing.T) {
	var buf bytes.Buffer
	logger := logging.NewJSONLogger(&buf, "info")
	const sentinelPass = "super-secret-pass-RED-LOCK-42"
	secretDSN := "postgres://studio:" + sentinelPass + "@localhost:5432/curriculum_studio?sslmode=disable"
	logger.Info("boot", "database_url", secretDSN, "note", "ok")

	out := buf.String()
	assert.NotContains(t, out, sentinelPass)
	assert.NotContains(t, out, "super-secret-pass")
	assert.Contains(t, out, "database_url")
	var entry map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &entry))
	assert.Equal(t, "INFO", entry["level"])
	dsn, _ := entry["database_url"].(string)
	assert.NotContains(t, dsn, sentinelPass)
	assert.True(t,
		strings.Contains(dsn, "REDACTED") || strings.Contains(dsn, "%5BREDACTED%5D"),
		"expected redacted marker in DSN, got %q", dsn,
	)
}

func TestRedactingHandlerScrubsPasswordLikeKeys(t *testing.T) {
	var buf bytes.Buffer
	base := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	logger := slog.New(logging.NewRedactingHandler(base))
	logger.Info("auth", "password", "hunter2", "client_secret", "cs-xyz", "token", "tok-abc")

	out := buf.String()
	assert.NotContains(t, out, "hunter2")
	assert.NotContains(t, out, "cs-xyz")
	assert.NotContains(t, out, "tok-abc")
	assert.True(t, strings.Contains(out, "[REDACTED]") || strings.Contains(out, "REDACTED"))
}

type errorWithDSN struct{}

func (errorWithDSN) Error() string {
	return `connect failed: postgres://studio:err-attr-secret-QQ7@localhost:5432/curriculum_studio?sslmode=disable dial tcp: connection refused`
}

type stringerWithDSN struct{}

func (stringerWithDSN) String() string {
	return `backup dsn postgresql://ops:stringer-secret-WW3@db:5432/curriculum_studio`
}

func TestRedactingHandlerScrubsErrorAndAnyAttrs(t *testing.T) {
	var buf bytes.Buffer
	logger := logging.NewJSONLogger(&buf, "info")

	const (
		errSecret      = "err-attr-secret-QQ7"
		stringerSecret = "stringer-secret-WW3"
		wrappedSecret  = "wrapped-secret-YY5"
	)

	logger.Error("fatal", "error", errorWithDSN{})
	logger.Info("any", "err", any(errorWithDSN{}), "note", stringerWithDSN{})
	logger.Error("migrate", "error", fmt.Errorf("migrate: %w", fmt.Errorf(
		"open postgres://id:"+wrappedSecret+"@localhost/curriculum_studio: boom",
	)))

	out := buf.String()
	assert.NotContains(t, out, errSecret)
	assert.NotContains(t, out, stringerSecret)
	assert.NotContains(t, out, wrappedSecret)
	assert.NotContains(t, out, "err-attr-secret")
	assert.NotContains(t, out, "stringer-secret")
	assert.NotContains(t, out, "wrapped-secret")
	assert.Contains(t, out, "REDACTED")
	assert.True(t,
		strings.Contains(out, "connect failed") ||
			strings.Contains(out, "connection refused") ||
			strings.Contains(out, "migrate") ||
			strings.Contains(out, "postgres"),
		"expected useful diagnostic class retained, got %s", out,
	)

	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line == "" {
			continue
		}
		var entry map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &entry))
		for _, key := range []string{"error", "err", "note"} {
			if v, ok := entry[key]; ok {
				s := fmt.Sprint(v)
				assert.NotContains(t, s, errSecret)
				assert.NotContains(t, s, stringerSecret)
				assert.NotContains(t, s, wrappedSecret)
			}
		}
	}
}

func TestMainFailureLogPathRedactsErrorAttr(t *testing.T) {
	var buf bytes.Buffer
	logger := logging.NewJSONLogger(&buf, "info")
	prev := slog.Default()
	slog.SetDefault(logger)
	t.Cleanup(func() { slog.SetDefault(prev) })

	const sentinel = "main-fail-secret-MM4"
	err := fmt.Errorf("listen: failed using postgres://studio:%s@localhost:5432/curriculum_studio", sentinel)

	slog.Error("fatal", "error", err)
	slog.Error("config", "error", err)
	slog.Error("migrate", "direction", "up", "error", err)

	out := buf.String()
	require.NotEmpty(t, out)
	assert.NotContains(t, out, sentinel)
	assert.NotContains(t, out, "main-fail-secret")
	assert.Contains(t, out, "REDACTED")
	assert.Contains(t, out, "fatal")
}

func TestParseLevelAndWithAttrsGroup(t *testing.T) {
	var buf bytes.Buffer
	logger := logging.NewJSONLogger(&buf, "debug")
	logger = logger.With("password", "should-hide", "env", "test")
	logger = logger.WithGroup("nested")
	logger.Debug("dbg", "api_key", "k-123", "secret", "s-9", "ok", "yes")

	out := buf.String()
	assert.NotContains(t, out, "should-hide")
	assert.NotContains(t, out, "k-123")
	assert.NotContains(t, out, "s-9")
	assert.Contains(t, out, "DEBUG")

	for _, level := range []string{"warn", "warning", "error", "INFO", ""} {
		buf.Reset()
		l := logging.NewJSONLogger(&buf, level)
		l.Error("lvl")
		assert.Contains(t, buf.String(), "lvl")
	}

	buf.Reset()
	base := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	h := logging.NewRedactingHandler(base)
	rec := slog.NewRecord(time.Now(), slog.LevelInfo, "grp", 0)
	rec.AddAttrs(slog.Group("g", "password", "p", "x", "y"))
	require.NoError(t, h.Handle(context.Background(), rec))
	assert.NotContains(t, buf.String(), `"p"`)
	assert.Contains(t, buf.String(), "REDACTED")
}
