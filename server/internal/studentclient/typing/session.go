// Package typing implements the local typing activity runner and metrics.
//
// Typing evidence supports typing/digital-fluency standards only. Completing a
// typing session never implies terminal-command mastery.
package typing

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
)

// VerifierVersion identifies the observation shape for typing_metrics checks.
const VerifierVersion = "1"

// Metrics is a snapshot of typing performance for the current session.
type Metrics struct {
	CorrectChars    int
	IncorrectChars  int
	TotalKeystrokes int
	CompletedPrompts int
	TotalPrompts    int
	// Elapsed is wall time since the first keystroke (zero before any input).
	Elapsed time.Duration
	// WPM is net words-per-minute using (correctChars/5) / minutes.
	WPM float64
	// Accuracy is correct / (correct+incorrect); 1.0 when nothing typed yet.
	Accuracy float64
	// Done is true when every prompt has been completed exactly.
	Done bool
	// ThresholdsMet is true when Done and WPM/accuracy meet Success* thresholds.
	ThresholdsMet bool
}

// Session is an interactive typing practice session over an assigned prompt set.
type Session struct {
	content contracts.TypingContent
	checks  []contracts.Check

	promptIdx int
	input     []rune

	correct   int
	incorrect int
	keys      int

	startedAt time.Time
	now       func() time.Time
}

// NewSession builds a typing session from revision typing content and checks.
// checks may be nil; Observations still work with content SuccessWPM/Accuracy.
func NewSession(content *contracts.TypingContent, checks []contracts.Check) (*Session, error) {
	if content == nil {
		return nil, fmt.Errorf("typing content is required")
	}
	if len(content.Prompts) == 0 {
		return nil, fmt.Errorf("typing prompts must not be empty")
	}
	for i, p := range content.Prompts {
		if strings.TrimSpace(p.Text) == "" {
			return nil, fmt.Errorf("typing prompts[%d]: empty text", i)
		}
	}
	return &Session{
		content: *content,
		checks:  append([]contracts.Check(nil), checks...),
		now:     time.Now,
	}, nil
}

// TypeKey records one typed rune against the current prompt.
// Correct next characters advance the input; wrong characters count as errors
// without advancing (student must type the expected character).
func (s *Session) TypeKey(r rune) {
	if s.Done() {
		return
	}
	if r < 32 && r != '\t' {
		return
	}
	s.markStarted()
	s.keys++

	prompt := []rune(s.content.Prompts[s.promptIdx].Text)
	pos := len(s.input)
	if pos >= len(prompt) {
		// Extra characters after a full match count incorrect until enter/advance.
		s.incorrect++
		return
	}
	if r == prompt[pos] {
		s.input = append(s.input, r)
		s.correct++
		if len(s.input) == len(prompt) {
			s.advancePrompt()
		}
		return
	}
	s.incorrect++
}

// Backspace removes the last typed character of the current prompt (no error credit).
func (s *Session) Backspace() {
	if s.Done() || len(s.input) == 0 {
		return
	}
	s.markStarted()
	s.keys++
	// Undoing a correct character reduces the correct count so accuracy stays honest.
	s.correct--
	if s.correct < 0 {
		s.correct = 0
	}
	s.input = s.input[:len(s.input)-1]
}

// SubmitLine is an explicit "accept current line" for UI enter handling.
// When the current input exactly matches the prompt it advances; otherwise it
// is a no-op (students continue typing/correcting).
func (s *Session) SubmitLine() bool {
	if s.Done() {
		return false
	}
	prompt := s.content.Prompts[s.promptIdx].Text
	if string(s.input) != prompt {
		return false
	}
	// Already matched via TypeKey advance in the common path.
	if len(s.input) == utf8.RuneCountInString(prompt) {
		// If still on this prompt (edge), advance.
		if s.promptIdx < len(s.content.Prompts) &&
			s.content.Prompts[s.promptIdx].Text == prompt &&
			string(s.input) == prompt {
			s.advancePrompt()
			return true
		}
	}
	return false
}

// CurrentPrompt returns the active prompt text and id, or empty when done.
func (s *Session) CurrentPrompt() (id, text string) {
	if s.Done() {
		return "", ""
	}
	p := s.content.Prompts[s.promptIdx]
	return p.ID, p.Text
}

// CurrentInput is the in-progress buffer for the active prompt.
func (s *Session) CurrentInput() string {
	return string(s.input)
}

// PromptIndex is the zero-based index of the active prompt (or len when done).
func (s *Session) PromptIndex() int {
	return s.promptIdx
}

// RemainingPrompts counts prompts not yet completed.
func (s *Session) RemainingPrompts() int {
	n := len(s.content.Prompts) - s.promptIdx
	if n < 0 {
		return 0
	}
	return n
}

// Done reports whether every prompt has been completed.
func (s *Session) Done() bool {
	return s.promptIdx >= len(s.content.Prompts)
}

// Metrics returns the current performance snapshot.
func (s *Session) Metrics() Metrics {
	m := Metrics{
		CorrectChars:     s.correct,
		IncorrectChars:   s.incorrect,
		TotalKeystrokes:  s.keys,
		CompletedPrompts: s.promptIdx,
		TotalPrompts:     len(s.content.Prompts),
		Done:             s.Done(),
	}
	if !s.startedAt.IsZero() {
		m.Elapsed = s.now().UTC().Sub(s.startedAt)
		if m.Elapsed < 0 {
			m.Elapsed = 0
		}
	}
	m.Accuracy = accuracy(s.correct, s.incorrect)
	m.WPM = wpm(s.correct, m.Elapsed)
	m.ThresholdsMet = m.Done && thresholdsMet(m, s.content)
	return m
}

// MeetsThresholds reports Done + SuccessWPM/SuccessAccuracy (content defaults apply).
func (s *Session) MeetsThresholds() bool {
	return s.Metrics().ThresholdsMet
}

// Observations synthesizes Observation rows for each check on the activity.
// typing_metrics checks pass when thresholds are met; other kinds fail closed.
func (s *Session) Observations() []contracts.Observation {
	m := s.Metrics()
	now := s.now().UTC()
	out := make([]contracts.Observation, 0, len(s.checks))
	if len(s.checks) == 0 {
		// Synthetic single observation when activity omitted checks (should not
		// happen for validated docs, but keeps unit tests simple).
		out = append(out, contracts.Observation{
			SchemaVersion: contracts.ObservationSchemaVersion,
			CheckID:       "typing-metrics",
			Kind:          contracts.CheckTypingMetrics,
			Passed:        m.ThresholdsMet,
			ObservedAt:    now,
			Message:       metricsMessage(m),
			Details:       metricsDetails(m),
		})
		return out
	}
	for _, ch := range s.checks {
		obs := contracts.Observation{
			SchemaVersion: contracts.ObservationSchemaVersion,
			CheckID:       ch.ID,
			Kind:          ch.Kind,
			Optional:      ch.Optional,
			ObservedAt:    now,
			Details:       map[string]any{},
		}
		switch ch.Kind {
		case contracts.CheckTypingMetrics:
			minWPM, errW := contracts.TypingMinWPM(ch.Params)
			minAcc, errA := contracts.TypingMinAccuracy(ch.Params)
			// Fall back to content thresholds when params missing (defensive).
			if errW != nil {
				minWPM = s.content.SuccessWPM
			}
			if errA != nil {
				minAcc = s.content.SuccessAccuracy
			}
			if minWPM <= 0 {
				minWPM = s.content.SuccessWPM
			}
			if minAcc <= 0 {
				minAcc = s.content.SuccessAccuracy
			}
			passed := m.Done && m.WPM >= minWPM && m.Accuracy+1e-9 >= minAcc
			obs.Passed = passed
			obs.Message = metricsMessage(m)
			obs.Details = metricsDetails(m)
			obs.Details["minWpm"] = minWPM
			obs.Details["minAccuracy"] = minAcc
		default:
			obs.Passed = false
			obs.Message = fmt.Sprintf("check kind %q is not evaluated by typing runner", ch.Kind)
		}
		out = append(out, obs)
	}
	return out
}

// TypeString types each rune of text (test helper path).
func (s *Session) TypeString(text string) {
	for _, r := range text {
		s.TypeKey(r)
	}
}

// SetClock replaces time.Now for deterministic tests.
func (s *Session) SetClock(now func() time.Time) {
	if now != nil {
		s.now = now
	}
}

func (s *Session) markStarted() {
	if s.startedAt.IsZero() {
		s.startedAt = s.now().UTC()
	}
}

func (s *Session) advancePrompt() {
	s.promptIdx++
	s.input = s.input[:0]
}

func accuracy(correct, incorrect int) float64 {
	total := correct + incorrect
	if total == 0 {
		return 1
	}
	return float64(correct) / float64(total)
}

func wpm(correctChars int, elapsed time.Duration) float64 {
	if correctChars <= 0 {
		return 0
	}
	minutes := elapsed.Minutes()
	if minutes <= 0 {
		// Treat sub-second bursts as a tiny positive duration so perfect
		// short sets still report a high WPM rather than zero/Inf.
		minutes = 1.0 / 60.0
	}
	return (float64(correctChars) / 5.0) / minutes
}

func thresholdsMet(m Metrics, content contracts.TypingContent) bool {
	if !m.Done {
		return false
	}
	needWPM := content.SuccessWPM
	needAcc := content.SuccessAccuracy
	if needWPM < 0 {
		needWPM = 0
	}
	if needAcc <= 0 {
		// Default: require perfect-enough accuracy when unset.
		needAcc = 1
	}
	if m.WPM+1e-9 < needWPM {
		return false
	}
	if m.Accuracy+1e-9 < needAcc {
		return false
	}
	return true
}

func metricsMessage(m Metrics) string {
	if !m.Done {
		return fmt.Sprintf("prompts %d/%d incomplete", m.CompletedPrompts, m.TotalPrompts)
	}
	if m.ThresholdsMet {
		return fmt.Sprintf("met thresholds: %.1f wpm, %.0f%% accuracy", m.WPM, m.Accuracy*100)
	}
	return fmt.Sprintf("thresholds not met: %.1f wpm, %.0f%% accuracy", m.WPM, m.Accuracy*100)
}

func metricsDetails(m Metrics) map[string]any {
	return map[string]any{
		"wpm":              m.WPM,
		"accuracy":         m.Accuracy,
		"correctChars":     m.CorrectChars,
		"incorrectChars":   m.IncorrectChars,
		"totalKeystrokes":  m.TotalKeystrokes,
		"completedPrompts": m.CompletedPrompts,
		"totalPrompts":     m.TotalPrompts,
		"elapsedMs":        m.Elapsed.Milliseconds(),
		"done":             m.Done,
		"thresholdsMet":    m.ThresholdsMet,
	}
}
