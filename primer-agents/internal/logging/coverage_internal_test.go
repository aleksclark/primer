package logging

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"log/slog"
)

type coverageStringer struct{}

func (coverageStringer) String() string { return "stringer-secret" }

func TestCoverageLevelsAndDSNForms(t *testing.T) {
	require.Equal(t, slog.LevelDebug, parseLevel("debug"))
	require.Equal(t, slog.LevelWarn, parseLevel("warning"))
	require.Equal(t, slog.LevelError, parseLevel("error"))
	require.Equal(t, slog.LevelInfo, parseLevel("unknown"))
	require.Contains(t, redactDSN("postgres://user@host/db"), "user@host")
	require.Contains(t, redactDSN("postgres://user:pass@host/db"), "REDACTED")
	require.Equal(t, "[REDACTED_DSN]", redactDSN("not-a-dsn"))
	require.Contains(t, redactDSN("postgres://user:%zz@host/db"), "REDACTED")
}

func TestCoverageRedactionHelpers(t *testing.T) {
	require.Equal(t, "", redactAnyValue(nil))
	require.Contains(t, redactAnyValue(errors.New("postgres://u:secret@host/db")), "REDACTED")
	require.Contains(t, redactAnyValue(coverageStringer{}), "stringer-secret")
	require.Contains(t, redactAnyValue([]byte("bytes")), "bytes")
	require.Contains(t, redactAnyValue(fmt.Sprintf("%d", 42)), "42")
	attrs := attrsToAny([]slog.Attr{slog.String("a", "b")})
	require.Equal(t, []any{"a", "b"}, attrs)
}
