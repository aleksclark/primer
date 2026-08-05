// Package observe implements broker-owned command boundary observation for
// interactive bash sessions (Phase 2 MVP).
//
// Trust model
// -----------
// Trusted (when instrumentation is healthy):
//   - exit status from PROMPT_COMMAND ($?)
//   - cwd after command via pwd -P written beside the event
//   - submitted line / BASH_COMMAND from DEBUG trap (candidate argv)
//   - simple argv tokenization of the submitted line (best-effort)
//
// Not trusted:
//   - PTY screen scrollback as stdout/stderr
//   - student-writable files under the workspace (events land outside workspace
//     on a broker-owned spool directory)
//
// Stdout/stderr from the interactive PTY path are marked untrusted. Scripted
// process-wait execution (RunShell) remains the path with trusted stream capture.
// StructuredCommandEvidence is set only when exit + cwd + argv quality meet the bar.
package observe

import (
	"bufio"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
)

// InstrumentationVersion tags the bash hook contract.
const InstrumentationVersion = "observe-bash/1"

// Excerpt limits for event payloads.
const (
	MaxSubmittedLine = 1024
	MaxExcerptBytes  = 2048
)

// Spool is a broker-owned directory that receives one JSON line per finished command.
type Spool struct {
	Dir string
}

// Prepare creates a unique spool directory under baseDir.
func Prepare(baseDir string) (*Spool, error) {
	if baseDir == "" {
		return nil, fmt.Errorf("observe: baseDir required")
	}
	if err := os.MkdirAll(baseDir, 0o700); err != nil {
		return nil, err
	}
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return nil, err
	}
	dir := filepath.Join(baseDir, "obs-"+hex.EncodeToString(b[:]))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	// Event file is append-only from the shell; mode allows the sandboxed uid.
	events := filepath.Join(dir, "events.ndjson")
	f, err := os.OpenFile(events, os.O_CREATE|os.O_WRONLY, 0o666)
	if err != nil {
		return nil, err
	}
	_ = f.Close()
	return &Spool{Dir: dir}, nil
}

// EventsPath is the NDJSON file the shell appends to.
func (s *Spool) EventsPath() string {
	if s == nil {
		return ""
	}
	return filepath.Join(s.Dir, "events.ndjson")
}

// RCPath is the generated bash rcfile.
func (s *Spool) RCPath() string {
	if s == nil {
		return ""
	}
	return filepath.Join(s.Dir, "rc.bash")
}

// Close removes the spool directory.
func (s *Spool) Close() error {
	if s == nil || s.Dir == "" {
		return nil
	}
	return os.RemoveAll(s.Dir)
}

// WriteBashRC writes a --rcfile that emits structured command markers.
// eventsPath and workspace are host paths visible to the shell process.
// When sandboxed, pass the in-sandbox path for eventsPath (and workspace as /workspace).
func WriteBashRC(rcPath, eventsPath, workspace string) error {
	if rcPath == "" || eventsPath == "" {
		return fmt.Errorf("observe: rc and events paths required")
	}
	// Bash instrumentation: DEBUG captures BASH_COMMAND; PROMPT_COMMAND emits on prompt.
	// Events are JSON lines. Student can disrupt this; missing events fail closed for
	// command-sensitive checks rather than synthesizing success from the screen.
	body := fmt.Sprintf(`# Primer observe-bash %s — generated; do not edit
unset PROMPT_COMMAND HISTCONTROL
export HISTFILE=/dev/null
export PS1='$ '
_primer_ws=%q
_primer_events=%q
_primer_cmd=
_primer_cwd_before=
_primer_in_prompt=

_primer_json_escape() {
  # minimal JSON string escape to stdout
  local s=$1
  s=${s//\\/\\\\}
  s=${s//\"/\\\"}
  s=${s//$'\n'/\\n}
  s=${s//$'\r'/\\r}
  s=${s//$'\t'/\\t}
  printf '%%s' "$s"
}

_primer_debug() {
  # Skip our own prompt machinery and empty commands.
  case "$BASH_COMMAND" in
    _primer_*|*PROMPT_COMMAND*) return 0 ;;
  esac
  if [ -n "$_primer_in_prompt" ]; then
    return 0
  fi
  _primer_cmd=$BASH_COMMAND
  _primer_cwd_before=$(pwd -P 2>/dev/null || pwd)
}

_primer_prompt() {
  local ec=$?
  _primer_in_prompt=1
  if [ -n "$_primer_cmd" ]; then
    local cwd_after cmd_e cwd_b_e cwd_a_e
    cwd_after=$(pwd -P 2>/dev/null || pwd)
    cmd_e=$(_primer_json_escape "$_primer_cmd")
    cwd_b_e=$(_primer_json_escape "$_primer_cwd_before")
    cwd_a_e=$(_primer_json_escape "$cwd_after")
    # Append one NDJSON event. Failure is silent; broker treats missing events as no evidence.
    printf '{"v":1,"cmd":"%%s","cwd_before":"%%s","cwd_after":"%%s","exit":%%d,"ts":%%s}\n' \
      "$cmd_e" "$cwd_b_e" "$cwd_a_e" "$ec" "$(date +%%s 2>/dev/null || echo 0)" \
      >>"$_primer_events" 2>/dev/null || true
  fi
  _primer_cmd=
  _primer_cwd_before=
  _primer_in_prompt=
  return 0
}

trap '_primer_debug' DEBUG
PROMPT_COMMAND=_primer_prompt
`, InstrumentationVersion, workspace, eventsPath)

	return os.WriteFile(rcPath, []byte(body), 0o644)
}

// rawEvent is one NDJSON line from the bash hook.
type rawEvent struct {
	V         int    `json:"v"`
	Cmd       string `json:"cmd"`
	CwdBefore string `json:"cwd_before"`
	CwdAfter  string `json:"cwd_after"`
	Exit      int    `json:"exit"`
	TS        int64  `json:"ts"`
}

// Reader tails the events file and converts lines into ShellEvents.
type Reader struct {
	path   string
	offset int64
	seq    int64
	// Workspace is the host workspace path for relativizing cwd.
	Workspace string
	// SandboxWorkspace is the in-sandbox workspace root (e.g. /workspace) when applicable.
	SandboxWorkspace string
	SessionID        string
	RunnerVersion    string
	VerifierVersion  string
}

// NewReader starts reading from the beginning of the events file.
func NewReader(spool *Spool) *Reader {
	if spool == nil {
		return &Reader{}
	}
	return &Reader{path: spool.EventsPath()}
}

// Drain reads any new events since the last call.
func (r *Reader) Drain() ([]contracts.ShellEvent, error) {
	if r == nil || r.path == "" {
		return nil, nil
	}
	f, err := os.Open(r.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	if r.offset > 0 {
		if _, err := f.Seek(r.offset, 0); err != nil {
			return nil, err
		}
	}
	var out []contracts.ShellEvent
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var raw rawEvent
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			// Malformed line: skip (fail closed for that command, do not crash session).
			continue
		}
		if raw.V != 1 || strings.TrimSpace(raw.Cmd) == "" {
			continue
		}
		r.seq++
		ev := r.toShellEvent(raw)
		out = append(out, ev)
	}
	if err := sc.Err(); err != nil {
		return out, err
	}
	off, err := f.Seek(0, 1)
	if err == nil {
		r.offset = off
	}
	return out, nil
}

func (r *Reader) toShellEvent(raw rawEvent) contracts.ShellEvent {
	finished := time.Now().UTC()
	if raw.TS > 0 {
		finished = time.Unix(raw.TS, 0).UTC()
	}
	line := raw.Cmd
	if len(line) > MaxSubmittedLine {
		line = line[:MaxSubmittedLine]
	}
	exe, argv, argvOK, pipe := ParseCommandLine(line)
	cwdBefore := r.relCwd(raw.CwdBefore)
	cwdAfter := r.relCwd(raw.CwdAfter)
	q := contracts.EvidenceQuality{
		Exit: true,
		Cwd:  cwdAfter != "" || raw.CwdAfter != "",
		Argv: argvOK,
		// Interactive PTY does not capture stream pipes.
		Stdout: false,
		Stderr: false,
	}
	structured := q.MeetsStructuredBar()
	return contracts.ShellEvent{
		SchemaVersion:        contracts.ShellEventSchemaVersion,
		SessionID:            r.SessionID,
		Sequence:             r.seq,
		FinishedAt:           finished,
		SubmittedLine:        line,
		Executable:           exe,
		Argv:                 argv,
		ArgvAvailable:        argvOK,
		CwdBefore:            cwdBefore,
		CwdAfter:             cwdAfter,
		CwdAvailable:         q.Cwd,
		ExitCode:             raw.Exit,
		ExitAvailable:        true,
		Stdout:               contracts.Excerpt{Trusted: false},
		Stderr:               contracts.Excerpt{Trusted: false},
		Pipeline:             pipe,
		RunnerVersion:        r.RunnerVersion,
		ShellInstrumentation: InstrumentationVersion,
		VerifierVersion:      r.VerifierVersion,
		Source:               contracts.SourceObserveBash,
		Structured:           structured,
		Quality:              q,
	}
}

func (r *Reader) relCwd(cwd string) string {
	cwd = strings.TrimSpace(cwd)
	if cwd == "" {
		return ""
	}
	// Prefer sandbox mapping.
	if r.SandboxWorkspace != "" {
		if cwd == r.SandboxWorkspace {
			return "."
		}
		if strings.HasPrefix(cwd, r.SandboxWorkspace+"/") {
			return filepath.ToSlash(strings.TrimPrefix(cwd, r.SandboxWorkspace+"/"))
		}
	}
	if r.Workspace != "" {
		if rel, err := filepath.Rel(r.Workspace, cwd); err == nil && !strings.HasPrefix(rel, "..") {
			if rel == "" {
				return "."
			}
			return filepath.ToSlash(rel)
		}
	}
	// Already relative?
	if !filepath.IsAbs(cwd) {
		return filepath.ToSlash(filepath.Clean(cwd))
	}
	return ""
}

// ParseCommandLine tokenizes a simple bash command for argv predicates.
// Complex constructs (subshells, expansions, quotes with escapes) may mark argv unavailable.
func ParseCommandLine(line string) (exe string, argv []string, ok bool, pipe *contracts.PipelineInfo) {
	line = strings.TrimSpace(line)
	if line == "" {
		return "", nil, false, nil
	}
	// Reject constructs we will not invent argv for.
	if strings.ContainsAny(line, "`") || strings.Contains(line, "$(") || strings.Contains(line, "${") {
		return "", nil, false, &contracts.PipelineInfo{}
	}
	pipe = &contracts.PipelineInfo{}
	if strings.Contains(line, "|") {
		pipe.HasPipe = true
	}
	if strings.Contains(line, ">") {
		pipe.HasRedirectOut = true
	}
	if strings.Contains(line, "<") {
		pipe.HasRedirectIn = true
	}

	// For pipelines, record stage executables and use the first stage for primary exe/argv.
	stages := splitPipeline(line)
	for _, st := range stages {
		toks, tokOK := tokenize(st)
		if !tokOK || len(toks) == 0 {
			return "", nil, false, pipe
		}
		// Drop redirections from tokens for argv match.
		toks = dropRedirs(toks)
		if len(toks) == 0 {
			return "", nil, false, pipe
		}
		pipe.Stages = append(pipe.Stages, toks[0])
	}
	if len(stages) == 0 {
		return "", nil, false, pipe
	}
	toks, tokOK := tokenize(stages[0])
	if !tokOK {
		return "", nil, false, pipe
	}
	toks = dropRedirs(toks)
	if len(toks) == 0 {
		return "", nil, false, pipe
	}
	return toks[0], toks[1:], true, pipe
}

func splitPipeline(line string) []string {
	// Split on | not inside simple single/double quotes.
	var parts []string
	var b strings.Builder
	inSingle, inDouble := false, false
	for _, r := range line {
		switch {
		case r == '\'' && !inDouble:
			inSingle = !inSingle
			b.WriteRune(r)
		case r == '"' && !inSingle:
			inDouble = !inDouble
			b.WriteRune(r)
		case r == '|' && !inSingle && !inDouble:
			parts = append(parts, strings.TrimSpace(b.String()))
			b.Reset()
		default:
			b.WriteRune(r)
		}
	}
	if s := strings.TrimSpace(b.String()); s != "" || len(parts) > 0 {
		parts = append(parts, s)
	}
	return parts
}

func tokenize(s string) ([]string, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, false
	}
	var tokens []string
	var b strings.Builder
	inSingle, inDouble := false, false
	flush := func() {
		if b.Len() > 0 {
			tokens = append(tokens, b.String())
			b.Reset()
		}
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '\\' && !inSingle && i+1 < len(s):
			b.WriteByte(s[i+1])
			i++
		case c == '\'' && !inDouble:
			inSingle = !inSingle
		case c == '"' && !inSingle:
			inDouble = !inDouble
		case unicode.IsSpace(rune(c)) && !inSingle && !inDouble:
			flush()
		default:
			b.WriteByte(c)
		}
	}
	if inSingle || inDouble {
		return nil, false
	}
	flush()
	if len(tokens) == 0 {
		return nil, false
	}
	return tokens, true
}

func dropRedirs(toks []string) []string {
	out := make([]string, 0, len(toks))
	for i := 0; i < len(toks); i++ {
		t := toks[i]
		if t == ">" || t == ">>" || t == "<" || t == "2>" || t == "2>>" || t == "&>" || t == ">&" {
			i++ // skip target
			continue
		}
		if strings.HasPrefix(t, "2>") || strings.HasPrefix(t, ">") || strings.HasPrefix(t, "<") {
			continue
		}
		// fd redirs like 1>file glued
		if len(t) > 1 && t[0] >= '0' && t[0] <= '9' && (t[1] == '>' || t[1] == '<') {
			continue
		}
		out = append(out, t)
	}
	return out
}

// BoundExcerpt builds a display excerpt with digest of the full text.
func BoundExcerpt(full string, trusted bool) contracts.Excerpt {
	sum := sha256.Sum256([]byte(full))
	ex := contracts.Excerpt{
		SHA256:  hex.EncodeToString(sum[:]),
		ByteLen: len(full),
		Trusted: trusted,
	}
	if len(full) <= MaxExcerptBytes {
		ex.Text = full
		return ex
	}
	ex.Text = full[:MaxExcerptBytes]
	ex.Truncated = true
	return ex
}

// ParseExit is a small helper for tests.
func ParseExit(s string) (int, error) {
	return strconv.Atoi(strings.TrimSpace(s))
}
