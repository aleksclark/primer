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
const VerifierVersion = "1"

// ShellState is optional runtime context for command/cwd/pipeline checks.
type ShellState struct {
	Cwd        string
	Executable string
	Args       []string
	ExitCode   int
	Stdout     string
	Stderr     string
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
		exe, err := stringParam(params, "executable")
		if err != nil {
			return false, "", nil, err
		}
		details["executable"] = shell.Executable
		if filepath.Base(shell.Executable) != exe && shell.Executable != exe {
			return false, fmt.Sprintf("executable want %s got %s", exe, shell.Executable), details, nil
		}
		if raw, ok := params["args"]; ok {
			wantArgs, err := asStringSlice(raw)
			if err != nil {
				return false, "", nil, err
			}
			details["args"] = shell.Args
			if !stringSlicesEqual(shell.Args, wantArgs) {
				return false, "args mismatch", details, nil
			}
		}
		if raw, ok := params["exitCode"]; ok {
			want, err := asInt(raw)
			if err != nil {
				return false, "", nil, err
			}
			details["exitCode"] = shell.ExitCode
			if shell.ExitCode != want {
				return false, fmt.Sprintf("exit code want %d got %d", want, shell.ExitCode), details, nil
			}
		}
		return true, "command ok", details, nil

	case contracts.CheckPipelineOutput:
		if shell == nil {
			return false, "no pipeline observation available", nil, nil
		}
		out := normalizeOutput(shell.Stdout)
		details["stdoutNorm"] = truncate(out, 256)
		if want, ok := params["value"].(string); ok {
			if out == normalizeOutput(want) {
				return true, "output equals", details, nil
			}
			return false, "output mismatch", details, nil
		}
		if contains, ok := params["contains"].(string); ok {
			if strings.Contains(out, normalizeOutput(contains)) {
				return true, "output contains", details, nil
			}
			return false, "output missing substring", details, nil
		}
		if pat, ok := params["pattern"].(string); ok && pat != "" {
			re, err := regexp.Compile(pat)
			if err != nil {
				return false, "", nil, err
			}
			if re.MatchString(out) {
				return true, "output matches", details, nil
			}
			return false, "output pattern not matched", details, nil
		}
		return false, "no output expectation", details, nil

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

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
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
