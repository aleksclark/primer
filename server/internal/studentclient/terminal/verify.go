package terminal

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
)

// VerifierVersion identifies the observation shape produced by this package.
const VerifierVersion = "2"

// ShellState is optional runtime context for command/cwd/pipeline checks.
type ShellState struct {
	Cwd        string
	Executable string
	Args       []string
	ExitCode   int
	Stdout     string
	Stderr     string
	// StructuredCommandEvidence is true only when the observation came from
	// trusted command instrumentation. Synthetic PTY screen text must leave
	// this false so command_properties / pipeline_output cannot pass.
	StructuredCommandEvidence bool
	// Source labels the observation origin (e.g. "structured", "observe-bash").
	Source string
	// History is task-scoped command history for predicates that match any
	// successful command since the current task started.
	History *History
	// TaskIndex selects the active task window inside History (0-based).
	TaskIndex int
	// ManifestDigest is the latest workspace manifest digest when captured.
	ManifestDigest string
	// SubmittedTasks maps task IDs with a durable local/server conceptual response.
	SubmittedTasks map[string]bool
}

// VerifyCheck evaluates one check against workspace root and optional shell state.
func VerifyCheck(root string, check contracts.Check, shell *ShellState) contracts.Observation {
	obs := contracts.Observation{
		SchemaVersion: contracts.ObservationSchemaVersion,
		CheckID:       check.ID,
		Kind:          check.Kind,
		Optional:      check.Optional,
		ObservedAt:    time.Now().UTC(),
		Details:       map[string]any{},
	}
	passed, msg, details, err := evalCheck(root, check, shell)
	if err != nil {
		obs.Passed = false
		obs.Message = err.Error()
		return obs
	}
	obs.Passed = passed
	obs.Message = msg
	for k, v := range details {
		obs.Details[k] = v
	}
	return obs
}

// VerifyAll evaluates every check and returns observations in definition order.
func VerifyAll(root string, checks []contracts.Check, shell *ShellState) []contracts.Observation {
	out := make([]contracts.Observation, 0, len(checks))
	for _, ch := range checks {
		out = append(out, VerifyCheck(root, ch, shell))
	}
	return out
}

// EvalTree evaluates an all/any check tree. Optional failing leaves do not fail
// an all-node; required failing leaves do. Unknown check IDs fail closed.
func EvalTree(tree contracts.CheckTree, byID map[string]contracts.Observation) (passed bool, detail string) {
	if tree.CheckID != "" {
		obs, ok := byID[tree.CheckID]
		if !ok {
			return false, fmt.Sprintf("missing observation for %s", tree.CheckID)
		}
		if obs.Passed {
			return true, ""
		}
		if tree.Optional || obs.Optional {
			return true, fmt.Sprintf("optional check %s failed", tree.CheckID)
		}
		return false, obs.Message
	}
	if len(tree.All) > 0 {
		for _, child := range tree.All {
			ok, msg := EvalTree(child, byID)
			if !ok {
				return false, msg
			}
		}
		return true, ""
	}
	if len(tree.Any) > 0 {
		var msgs []string
		for _, child := range tree.Any {
			ok, msg := EvalTree(child, byID)
			if ok {
				return true, ""
			}
			if msg != "" {
				msgs = append(msgs, msg)
			}
		}
		return false, "none of any-branch passed: " + strings.Join(msgs, "; ")
	}
	return false, "empty check tree"
}

func evalCheck(root string, check contracts.Check, shell *ShellState) (bool, string, map[string]any, error) {
	params := check.Params
	if params == nil {
		params = map[string]any{}
	}
	details := map[string]any{}
	switch check.Kind {
	case contracts.CheckFileExists:
		path, err := pathParam(params, "path")
		if err != nil {
			return false, "", nil, err
		}
		full, err := contracts.JoinUnder(root, path)
		if err != nil {
			return false, "", nil, err
		}
		_, err = os.Lstat(full)
		ok := err == nil
		details["path"] = path
		if ok {
			return true, "exists", details, nil
		}
		return false, "path does not exist", details, nil

	case contracts.CheckFileNotExists:
		path, err := pathParam(params, "path")
		if err != nil {
			return false, "", nil, err
		}
		full, err := contracts.JoinUnder(root, path)
		if err != nil {
			return false, "", nil, err
		}
		_, err = os.Lstat(full)
		details["path"] = path
		if err == nil {
			return false, "path still exists", details, nil
		}
		if os.IsNotExist(err) {
			return true, "absent", details, nil
		}
		return false, "", nil, err

	case contracts.CheckContentEquals:
		path, want, err := pathAndString(params, "value")
		if err != nil {
			return false, "", nil, err
		}
		got, err := readWorkspaceFile(root, path)
		if err != nil {
			return false, err.Error(), map[string]any{"path": path}, nil
		}
		details["path"] = path
		if got == want {
			return true, "content equals", details, nil
		}
		return false, "content mismatch", details, nil

	case contracts.CheckContentContains:
		path, want, err := pathAndString(params, "value")
		if err != nil {
			return false, "", nil, err
		}
		got, err := readWorkspaceFile(root, path)
		if err != nil {
			return false, err.Error(), map[string]any{"path": path}, nil
		}
		details["path"] = path
		if strings.Contains(got, want) {
			return true, "content contains", details, nil
		}
		return false, "substring not found", details, nil

	case contracts.CheckContentMatch:
		path, err := pathParam(params, "path")
		if err != nil {
			return false, "", nil, err
		}
		pat, err := stringParam(params, "pattern")
		if err != nil {
			return false, "", nil, err
		}
		re, err := regexp.Compile(pat)
		if err != nil {
			return false, "", nil, err
		}
		got, err := readWorkspaceFile(root, path)
		if err != nil {
			return false, err.Error(), map[string]any{"path": path}, nil
		}
		details["path"] = path
		if re.MatchString(got) {
			return true, "content matches", details, nil
		}
		return false, "pattern not matched", details, nil

	case contracts.CheckPathType:
		path, err := pathParam(params, "path")
		if err != nil {
			return false, "", nil, err
		}
		want, err := stringParam(params, "type")
		if err != nil {
			return false, "", nil, err
		}
		full, err := contracts.JoinUnder(root, path)
		if err != nil {
			return false, "", nil, err
		}
		st, err := os.Lstat(full)
		if err != nil {
			return false, "path does not exist", map[string]any{"path": path}, nil
		}
		got := pathTypeOf(st)
		details["path"] = path
		details["type"] = got
		if got == want {
			return true, "type ok", details, nil
		}
		return false, fmt.Sprintf("want type %s got %s", want, got), details, nil

	case contracts.CheckPathMode:
		path, err := pathParam(params, "path")
		if err != nil {
			return false, "", nil, err
		}
		modeStr, err := stringParam(params, "mode")
		if err != nil {
			return false, "", nil, err
		}
		want, err := contracts.ParseMode(modeStr)
		if err != nil {
			return false, "", nil, err
		}
		full, err := contracts.JoinUnder(root, path)
		if err != nil {
			return false, "", nil, err
		}
		st, err := os.Lstat(full)
		if err != nil {
			return false, "path does not exist", map[string]any{"path": path}, nil
		}
		got := uint32(st.Mode().Perm())
		details["path"] = path
		details["mode"] = fmt.Sprintf("%04o", got)
		// Compare permission bits only; allow either 3 or 4 digit expectation.
		if got&0o777 == want&0o777 {
			return true, "mode ok", details, nil
		}
		return false, fmt.Sprintf("want mode %s got %04o", modeStr, got), details, nil

	case contracts.CheckCwd:
		path, err := pathParam(params, "path")
		if err != nil {
			return false, "", nil, err
		}
		if shell == nil || shell.Cwd == "" {
			return false, "no shell cwd available", map[string]any{"path": path}, nil
		}
		wantFull, err := contracts.JoinUnder(root, path)
		if err != nil {
			return false, "", nil, err
		}
		got := filepath.Clean(shell.Cwd)
		wantFull = filepath.Clean(wantFull)
		details["path"] = path
		details["cwd"] = got
		if got == wantFull {
			return true, "cwd ok", details, nil
		}
		// Also accept workspace-relative equality if shell reports relative.
		if rel, err := filepath.Rel(root, got); err == nil && filepath.Clean(rel) == filepath.Clean(path) {
			return true, "cwd ok", details, nil
		}
		return false, "cwd mismatch", details, nil

	case contracts.CheckCommandProperties:
		if shell == nil {
			return false, "no command observation available", nil, nil
		}
		if shell.ManifestDigest != "" {
			details["workspaceManifestDigest"] = shell.ManifestDigest
		}
		match, err := commandMatchFromParams(params, true)
		if err != nil {
			return false, "", nil, err
		}
		// Prefer history so any matching successful command satisfies the check
		// (not only the latest observation). Search from the active task window
		// first, then fall back to session start so completed earlier tasks keep
		// their evidence when later tasks are active.
		if shell.History != nil && len(shell.History.Events) > 0 {
			if hit, ok := findHistoryMatch(shell, match); ok {
				details["source"] = hit.Source
				details["structuredCommandEvidence"] = hit.Structured
				details["capability"] = contracts.CapStructuredCommandEvidence
				details["executable"] = hit.Executable
				details["args"] = hit.Argv
				details["exitCode"] = hit.ExitCode
				details["sequence"] = hit.Sequence
				if hit.ManifestAfter != "" {
					details["workspaceManifestDigest"] = hit.ManifestAfter
				}
				return true, "command ok (history)", details, nil
			}
			details["source"] = shell.Source
			details["structuredCommandEvidence"] = false
			if !historyHasStructured(shell.History, 0) {
				details["capability"] = ""
				return false, "structured command evidence unavailable", details, nil
			}
			return false, "no matching command in task history", details, nil
		}
		details["source"] = shell.Source
		details["structuredCommandEvidence"] = shell.StructuredCommandEvidence
		if !shell.StructuredCommandEvidence || shell.Executable == "pty-shell" || shell.Source == "pty-shell" || shell.Source == "synthetic-pty" || shell.Source == "screen" {
			details["capability"] = ""
			return false, "structured command evidence unavailable", details, nil
		}
		details["capability"] = contracts.CapStructuredCommandEvidence
		obs := contracts.CommandObservation{
			Executable:    shell.Executable,
			Argv:          shell.Args,
			ArgvAvailable: shell.Executable != "" || len(shell.Args) > 0,
			ExitCode:      shell.ExitCode,
			ExitAvailable: true,
			Stdout:        contracts.Excerpt{Text: shell.Stdout, Trusted: true},
			Stderr:        contracts.Excerpt{Text: shell.Stderr, Trusted: true},
			Source:        shell.Source,
			Structured:    shell.StructuredCommandEvidence,
			Quality: contracts.EvidenceQuality{
				Exit: true, Cwd: shell.Cwd != "", Argv: true, Stdout: true, Stderr: true,
			},
		}
		if !matchObservation(obs, match) {
			details["executable"] = shell.Executable
			details["args"] = shell.Args
			details["exitCode"] = shell.ExitCode
			return false, "command mismatch", details, nil
		}
		details["executable"] = shell.Executable
		details["args"] = shell.Args
		details["exitCode"] = shell.ExitCode
		return true, "command ok", details, nil

	case contracts.CheckPipelineOutput:
		if shell == nil {
			return false, "no pipeline observation available", nil, nil
		}
		if shell.ManifestDigest != "" {
			details["workspaceManifestDigest"] = shell.ManifestDigest
		}
		match, err := pipelineMatchFromParams(params)
		if err != nil {
			return false, "", nil, err
		}
		if shell.History != nil && len(shell.History.Events) > 0 {
			if hit, ok := findHistoryMatch(shell, match); ok {
				details["source"] = hit.Source
				details["structuredCommandEvidence"] = hit.Structured
				details["capability"] = contracts.CapStructuredCommandEvidence
				out := normalizeOutput(hit.Stdout.Text)
				details["stdoutNorm"] = truncate(out, 256)
				details["sequence"] = hit.Sequence
				return true, "output ok (history)", details, nil
			}
			details["source"] = shell.Source
			if !historyHasStructured(shell.History, 0) {
				details["capability"] = ""
				return false, "structured command evidence unavailable", details, nil
			}
			return false, "no matching output in task history", details, nil
		}
		details["source"] = shell.Source
		details["structuredCommandEvidence"] = shell.StructuredCommandEvidence
		if !shell.StructuredCommandEvidence || shell.Executable == "pty-shell" || shell.Source == "pty-shell" || shell.Source == "synthetic-pty" || shell.Source == "screen" {
			details["capability"] = ""
			return false, "structured command evidence unavailable", details, nil
		}
		// Last-observation path requires trusted stdout.
		if match.RequireStdoutTrusted {
			// ShellState.Stdout from structured process-wait is trusted.
			if shell.Source != contracts.SourceStructured && shell.Source != "structured" {
				details["capability"] = contracts.CapStructuredCommandEvidence
				return false, "stdout not trusted for pipeline check", details, nil
			}
		}
		details["capability"] = contracts.CapStructuredCommandEvidence
		out := normalizeOutput(shell.Stdout)
		details["stdoutNorm"] = truncate(out, 256)
		obs := contracts.CommandObservation{
			ExitAvailable: true,
			ExitCode:      shell.ExitCode,
			Stdout:        contracts.Excerpt{Text: shell.Stdout, Trusted: true},
			Source:        shell.Source,
			Structured:    true,
			Quality:       contracts.EvidenceQuality{Exit: true, Cwd: true, Argv: true, Stdout: true, Stderr: true},
		}
		if !matchObservation(obs, match) {
			return false, "output mismatch", details, nil
		}
		return true, "output ok", details, nil

	case contracts.CheckResponseSubmitted:
		taskID, err := stringParam(params, "taskId")
		if err != nil {
			// Accept snake_case alias used in some authored content.
			taskID, err = stringParam(params, "task_id")
			if err != nil {
				return false, "", nil, err
			}
		}
		details["taskId"] = taskID
		if shell != nil && shell.SubmittedTasks != nil && shell.SubmittedTasks[taskID] {
			return true, "response submitted", details, nil
		}
		return false, "response not submitted", details, nil

	default:
		return false, "", nil, fmt.Errorf("unknown check kind %q", check.Kind)
	}
}

func readWorkspaceFile(root, rel string) (string, error) {
	full, err := contracts.JoinUnder(root, rel)
	if err != nil {
		return "", err
	}
	b, err := os.ReadFile(full)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", rel, err)
	}
	return string(b), nil
}

func pathTypeOf(st os.FileInfo) string {
	mode := st.Mode()
	switch {
	case mode&os.ModeSymlink != 0:
		return contracts.PathTypeSymlink
	case mode.IsDir():
		return contracts.PathTypeDirectory
	default:
		return contracts.PathTypeFile
	}
}

func pathParam(params map[string]any, key string) (string, error) {
	s, err := stringParam(params, key)
	if err != nil {
		return "", err
	}
	if _, err := contracts.SafeRelPath(s); err != nil {
		return "", err
	}
	return s, nil
}

func pathAndString(params map[string]any, valueKey string) (path, value string, err error) {
	path, err = pathParam(params, "path")
	if err != nil {
		return "", "", err
	}
	value, err = stringParam(params, valueKey)
	if err != nil {
		return "", "", err
	}
	return path, value, nil
}

func stringParam(params map[string]any, key string) (string, error) {
	v, ok := params[key]
	if !ok || v == nil {
		return "", fmt.Errorf("params.%s is required", key)
	}
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("params.%s must be a string", key)
	}
	return s, nil
}

func asStringSlice(v any) ([]string, error) {
	switch t := v.(type) {
	case []string:
		return t, nil
	case []any:
		out := make([]string, 0, len(t))
		for _, x := range t {
			s, ok := x.(string)
			if !ok {
				return nil, fmt.Errorf("string list element is not string")
			}
			out = append(out, s)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("expected string list")
	}
}

func asInt(v any) (int, error) {
	switch n := v.(type) {
	case int:
		return n, nil
	case int64:
		return int(n), nil
	case float64:
		return int(n), nil
	default:
		return 0, fmt.Errorf("expected int")
	}
}

func normalizeOutput(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.TrimRight(s, "\n")
	return s
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func commandMatchFromParams(params map[string]any, requireStructured bool) (CommandMatch, error) {
	m := CommandMatch{RequireStructured: requireStructured}
	if exe, err := stringParam(params, "executable"); err == nil {
		m.Executable = exe
	} else {
		return m, err
	}
	if raw, ok := params["args"]; ok {
		wantArgs, err := asStringSlice(raw)
		if err != nil {
			return m, err
		}
		m.Args = wantArgs
		m.ArgsSet = true
	}
	if raw, ok := params["exitCode"]; ok {
		want, err := asInt(raw)
		if err != nil {
			return m, err
		}
		m.ExitCode = &want
	} else {
		// Default: at least one matching successful command.
		m.RequireSuccess = true
	}
	// Optional stream predicates on command_properties (Phase 2).
	if v, ok := params["stdoutContains"].(string); ok {
		m.StdoutContains = v
		m.RequireStdoutTrusted = true
	}
	if v, ok := params["stdoutEquals"].(string); ok {
		m.StdoutEquals = v
		m.RequireStdoutTrusted = true
	}
	if v, ok := params["stdoutPattern"].(string); ok {
		m.StdoutPattern = v
		m.RequireStdoutTrusted = true
	}
	if v, ok := params["stderrContains"].(string); ok {
		m.StderrContains = v
		m.RequireStderrTrusted = true
	}
	if v, ok := params["stderrEquals"].(string); ok {
		m.StderrEquals = v
		m.RequireStderrTrusted = true
	}
	if v, ok := params["stderrPattern"].(string); ok {
		m.StderrPattern = v
		m.RequireStderrTrusted = true
	}
	return m, nil
}

func pipelineMatchFromParams(params map[string]any) (CommandMatch, error) {
	m := CommandMatch{
		RequireStructured:    true,
		RequireStdoutTrusted: true,
	}
	if v, ok := params["value"].(string); ok {
		m.StdoutEquals = v
	}
	if v, ok := params["contains"].(string); ok {
		m.StdoutContains = v
	}
	if v, ok := params["pattern"].(string); ok {
		m.StdoutPattern = v
	}
	if m.StdoutEquals == "" && m.StdoutContains == "" && m.StdoutPattern == "" {
		return m, fmt.Errorf("no output expectation")
	}
	return m, nil
}

func historyHasStructured(h *History, taskIndex int) bool {
	if h == nil {
		return false
	}
	for _, e := range h.SinceTask(taskIndex) {
		if e.Structured && e.Quality.MeetsStructuredBar() {
			return true
		}
	}
	return false
}

// findHistoryMatch looks for a matching command in the active task window, then
// broader session history so evidence from earlier completed tasks remains valid.
func findHistoryMatch(shell *ShellState, match CommandMatch) (contracts.CommandObservation, bool) {
	if shell == nil || shell.History == nil {
		return contracts.CommandObservation{}, false
	}
	// Try active task window first (pedagogical "since task start").
	if hit, ok := shell.History.FindMatch(shell.TaskIndex, match); ok {
		return hit, true
	}
	// Fall back to session start so completed prior tasks keep command evidence.
	if shell.TaskIndex != 0 {
		if hit, ok := shell.History.FindMatch(0, match); ok {
			return hit, true
		}
	}
	// Also try each recorded task start (covers task index past end when done).
	if shell.History.TaskStartSeq != nil {
		for idx := range shell.History.TaskStartSeq {
			if idx == shell.TaskIndex || idx == 0 {
				continue
			}
			if hit, ok := shell.History.FindMatch(idx, match); ok {
				return hit, true
			}
		}
	}
	return contracts.CommandObservation{}, false
}
