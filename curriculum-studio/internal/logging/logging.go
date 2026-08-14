// Package logging provides structured JSON slog with secret/DSN redaction for
// the Curriculum Studio process boundary.
package logging

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"regexp"
	"strings"
)

// NewJSONLogger builds a slog logger that writes JSON to w (or stdout when nil)
// with redaction of secrets and DSN passwords.
func NewJSONLogger(w io.Writer, level string) *slog.Logger {
	if w == nil {
		w = os.Stdout
	}
	opts := &slog.HandlerOptions{Level: parseLevel(level)}
	base := slog.NewJSONHandler(w, opts)
	return slog.New(NewRedactingHandler(base))
}

func parseLevel(level string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// RedactingHandler wraps a slog.Handler and scrubs secret-like attributes.
type RedactingHandler struct {
	next slog.Handler
}

// NewRedactingHandler returns a handler that redacts secrets before logging.
func NewRedactingHandler(next slog.Handler) *RedactingHandler {
	return &RedactingHandler{next: next}
}

func (h *RedactingHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h *RedactingHandler) Handle(ctx context.Context, r slog.Record) error {
	out := slog.NewRecord(r.Time, r.Level, r.Message, r.PC)
	r.Attrs(func(a slog.Attr) bool {
		out.AddAttrs(redactAttr(a))
		return true
	})
	return h.next.Handle(ctx, out)
}

func (h *RedactingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	cleaned := make([]slog.Attr, len(attrs))
	for i, a := range attrs {
		cleaned[i] = redactAttr(a)
	}
	return &RedactingHandler{next: h.next.WithAttrs(cleaned)}
}

func (h *RedactingHandler) WithGroup(name string) slog.Handler {
	return &RedactingHandler{next: h.next.WithGroup(name)}
}

func redactAttr(a slog.Attr) slog.Attr {
	key := strings.ToLower(a.Key)
	if isSecretKey(key) {
		return slog.String(a.Key, "[REDACTED]")
	}
	switch a.Value.Kind() {
	case slog.KindString:
		return slog.String(a.Key, redactSecretText(a.Value.String()))
	case slog.KindGroup:
		group := a.Value.Group()
		cleaned := make([]slog.Attr, len(group))
		for i, g := range group {
			cleaned[i] = redactAttr(g)
		}
		return slog.Group(a.Key, attrsToAny(cleaned)...)
	case slog.KindAny:
		return slog.String(a.Key, redactAnyValue(a.Value.Any()))
	default:
		return a
	}
}

func redactAnyValue(v any) string {
	if v == nil {
		return ""
	}
	switch x := v.(type) {
	case error:
		return redactSecretText(x.Error())
	case fmt.Stringer:
		return redactSecretText(x.String())
	case string:
		return redactSecretText(x)
	case []byte:
		return redactSecretText(string(x))
	default:
		return redactSecretText(fmt.Sprint(x))
	}
}

func attrsToAny(attrs []slog.Attr) []any {
	out := make([]any, 0, len(attrs)*2)
	for _, a := range attrs {
		out = append(out, a.Key, a.Value.Any())
	}
	return out
}

func isSecretKey(key string) bool {
	switch {
	case key == "password", key == "passwd", key == "secret",
		key == "client_secret", key == "token", key == "access_token",
		key == "refresh_token", key == "authorization", key == "api_key",
		key == "apikey", strings.Contains(key, "password"),
		strings.Contains(key, "secret"), strings.HasSuffix(key, "_token"):
		return true
	default:
		return false
	}
}

// postgresURIInText matches postgres/postgresql URIs that may be embedded in
// surrounding diagnostic text (error strings from pgx, migrate wrappers, etc.).
var postgresURIInText = regexp.MustCompile(`(?i)postgres(?:ql)?://[^\s"'<>]+`)

func looksLikeDSN(s string) bool {
	ls := strings.ToLower(strings.TrimSpace(s))
	return strings.HasPrefix(ls, "postgres://") ||
		strings.HasPrefix(ls, "postgresql://") ||
		(strings.Contains(ls, "://") && strings.Contains(ls, "@"))
}

// redactSecretText scrubs DSN passwords from free-form text while preserving
// useful diagnostic class (e.g. "connection refused", "migrate:").
func redactSecretText(s string) string {
	if s == "" {
		return s
	}
	if looksLikeDSN(s) && !strings.ContainsAny(s, " \t\n") {
		return redactDSN(s)
	}
	if postgresURIInText.MatchString(s) {
		return postgresURIInText.ReplaceAllStringFunc(s, redactDSN)
	}
	return s
}

func redactDSN(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" {
		if i := strings.Index(raw, "://"); i >= 0 {
			rest := raw[i+3:]
			if at := strings.Index(rest, "@"); at >= 0 {
				userinfo := rest[:at]
				hostpart := rest[at+1:]
				if colon := strings.Index(userinfo, ":"); colon >= 0 {
					return raw[:i+3] + userinfo[:colon] + ":[REDACTED]@" + hostpart
				}
				return raw[:i+3] + "[REDACTED]@" + hostpart
			}
		}
		return "[REDACTED_DSN]"
	}
	if u.User != nil {
		name := u.User.Username()
		if _, has := u.User.Password(); has {
			u.User = url.UserPassword(name, "[REDACTED]")
		}
	}
	return u.String()
}
