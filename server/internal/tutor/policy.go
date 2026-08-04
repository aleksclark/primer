package tutor

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"sync"
	"time"
)

// PolicyConfig controls response shaping and safety limits.
type PolicyConfig struct {
	// MaxSentences caps reply length (default 2).
	MaxSentences int
	// MaxMessagesPerSession is the soft rate limit (default 20).
	MaxMessagesPerSession int
	// Timeout bounds provider calls (default 8s).
	Timeout time.Duration
	// StripCodeLines drops lines that look like shell/code paste when true (default true).
	StripCodeLines bool
	// Enabled gates the whole tutor; when false Coach returns a disabled reply.
	Enabled bool
}

// DefaultPolicy returns production-leaning defaults.
func DefaultPolicy() PolicyConfig {
	return PolicyConfig{
		MaxSentences:          2,
		MaxMessagesPerSession: 20,
		Timeout:               8 * time.Second,
		StripCodeLines:        true,
		Enabled:               true,
	}
}

// PolicyService wraps a Provider with length, rate, timeout, and safety filters.
// On provider failure or policy rejection it returns an activity-local fallback.
type PolicyService struct {
	inner  Provider
	cfg    PolicyConfig
	mu     sync.Mutex
	counts map[string]int // sessionID -> message count
	// Failures tracks recent provider failures per student for parent diagnostics.
	failMu   sync.Mutex
	failures map[string]int // studentID -> count
}

// NewPolicy wraps inner with cfg. If inner is nil, FakeService is used.
func NewPolicy(inner Provider, cfg PolicyConfig) *PolicyService {
	if inner == nil {
		inner = NewFake()
	}
	if cfg.MaxSentences <= 0 {
		cfg.MaxSentences = 2
	}
	if cfg.MaxMessagesPerSession <= 0 {
		cfg.MaxMessagesPerSession = 20
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 8 * time.Second
	}
	return &PolicyService{
		inner:    inner,
		cfg:      cfg,
		counts:   make(map[string]int),
		failures: make(map[string]int),
	}
}

// ProviderName returns the wrapped provider name.
func (p *PolicyService) ProviderName() string {
	if p.inner == nil {
		return "fake"
	}
	return p.inner.Name()
}

// Enabled reports whether tutoring is globally on.
func (p *PolicyService) Enabled() bool { return p.cfg.Enabled }

// RecentFailureCount returns provider failure count for a student (process-local).
func (p *PolicyService) RecentFailureCount(studentID string) int {
	p.failMu.Lock()
	defer p.failMu.Unlock()
	return p.failures[studentID]
}

// Coach implements Service.
func (p *PolicyService) Coach(ctx context.Context, req Request) (Response, error) {
	fallback := FallbackHint(req.Activity, req.PriorHints, req.HintLevel)
	providerName := p.ProviderName()

	if !p.cfg.Enabled {
		return Response{
			Reply:    fallback,
			Provider: providerName,
			Fallback: true,
			Disabled: true,
		}, nil
	}

	// Rate limit before calling the provider.
	if req.SessionID != "" {
		p.mu.Lock()
		n := p.counts[req.SessionID]
		if n >= p.cfg.MaxMessagesPerSession {
			p.mu.Unlock()
			return Response{
				Reply:       fallback,
				Provider:    providerName,
				Fallback:    true,
				RateLimited: true,
			}, nil
		}
		p.counts[req.SessionID] = n + 1
		p.mu.Unlock()
	}

	// Never pass raw student text as system policy; sanitize first.
	req.StudentMessage = SanitizeStudentMessage(req.StudentMessage)

	callCtx := ctx
	var cancel context.CancelFunc
	if p.cfg.Timeout > 0 {
		callCtx, cancel = context.WithTimeout(ctx, p.cfg.Timeout)
		defer cancel()
	}

	raw, err := p.inner.Complete(callCtx, req)
	if err != nil {
		p.recordFailure(req.StudentID)
		// Timeout / cancel / provider error → activity-local fallback.
		return Response{
			Reply:    fallback,
			Provider: providerName,
			Fallback: true,
		}, nil
	}

	reply, filtered := ApplyResponsePolicy(raw, p.cfg)
	if reply == "" {
		p.recordFailure(req.StudentID)
		return Response{
			Reply:    fallback,
			Provider: providerName,
			Fallback: true,
			Filtered: true,
		}, nil
	}

	return Response{
		Reply:    reply,
		Provider: providerName,
		Filtered: filtered,
	}, nil
}

func (p *PolicyService) recordFailure(studentID string) {
	if studentID == "" {
		return
	}
	p.failMu.Lock()
	p.failures[studentID]++
	p.failMu.Unlock()
}

// codeLineRE matches lines that look like shell/code paste (not prose).
var codeLineRE = regexp.MustCompile(`(?i)^\s*(\$ |# |>>> |\w+\s*=\s*\S+|` +
	`cd\s+\S+|ls\s|pwd\b|mkdir\s|rm\s|cat\s|echo\s|export\s|` +
	`func\s|package\s|import\s|def\s|class\s|#!/).*$`)

// ApplyResponsePolicy enforces sentence limits and optional code stripping.
// Returns the cleaned reply and whether filtering changed the text.
func ApplyResponsePolicy(raw string, cfg PolicyConfig) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}
	orig := raw

	if cfg.StripCodeLines {
		var lines []string
		for _, line := range strings.Split(raw, "\n") {
			trim := strings.TrimSpace(line)
			if trim == "" {
				continue
			}
			if codeLineRE.MatchString(trim) {
				continue
			}
			// Drop fenced code blocks entirely.
			if strings.HasPrefix(trim, "```") {
				continue
			}
			lines = append(lines, trim)
		}
		raw = strings.Join(lines, " ")
	} else {
		raw = strings.Join(strings.Fields(raw), " ")
	}

	raw = strings.TrimSpace(raw)
	raw = limitSentences(raw, cfg.MaxSentences)
	raw = strings.TrimSpace(raw)
	filtered := raw != strings.TrimSpace(orig) && raw != orig
	// Normalize whitespace comparison for filtered flag.
	if collapseSpace(raw) != collapseSpace(orig) {
		filtered = true
	}
	return raw, filtered
}

func collapseSpace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func limitSentences(s string, max int) string {
	if max <= 0 || s == "" {
		return s
	}
	// Simple sentence split on .!? followed by space or end.
	var sentences []string
	start := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c != '.' && c != '!' && c != '?' {
			continue
		}
		// Include trailing punctuation.
		end := i + 1
		sent := strings.TrimSpace(s[start:end])
		if sent != "" {
			sentences = append(sentences, sent)
		}
		// Skip spaces after terminator.
		j := end
		for j < len(s) && (s[j] == ' ' || s[j] == '\n' || s[j] == '\t') {
			j++
		}
		start = j
		i = j - 1
		if len(sentences) >= max {
			return strings.Join(sentences, " ")
		}
	}
	rest := strings.TrimSpace(s[start:])
	if rest != "" {
		sentences = append(sentences, rest)
	}
	if len(sentences) > max {
		sentences = sentences[:max]
	}
	return strings.Join(sentences, " ")
}

// IsDisabledError reports whether err is ErrDisabled.
func IsDisabledError(err error) bool {
	return errors.Is(err, ErrDisabled)
}
