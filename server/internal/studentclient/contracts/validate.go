package contracts

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var (
	slugRE     = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
	idRE       = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_.-]*$`)
	modeRE     = regexp.MustCompile(`^[0-7]{3,4}$`)
	standardRE = regexp.MustCompile(`^[A-Z0-9]+(?:\.[A-Z0-9]+)+$`)
)

// ValidateDocument validates a full activity document including content.
func ValidateDocument(doc *ActivityDocument) error {
	if doc == nil {
		return fmt.Errorf("activity document is nil")
	}
	if doc.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported schema_version %q (want %q)", doc.SchemaVersion, SchemaVersion)
	}
	if !slugRE.MatchString(doc.Slug) {
		return fmt.Errorf("slug %q is invalid", doc.Slug)
	}
	if strings.TrimSpace(doc.Title) == "" {
		return fmt.Errorf("title is required")
	}
	switch doc.Kind {
	case KindTerminal, KindTyping:
	default:
		return fmt.Errorf("unknown kind %q", doc.Kind)
	}
	if strings.TrimSpace(doc.SubjectCode) == "" {
		return fmt.Errorf("subject_code is required")
	}
	if len(doc.Standards) == 0 {
		return fmt.Errorf("at least one standard is required")
	}
	seenStd := map[string]bool{}
	for i, s := range doc.Standards {
		if !standardRE.MatchString(s.Code) {
			return fmt.Errorf("standards[%d]: invalid code %q", i, s.Code)
		}
		key := s.Code + "|" + s.Role
		if seenStd[key] {
			return fmt.Errorf("standards[%d]: duplicate standard %s role %s", i, s.Code, s.Role)
		}
		seenStd[key] = true
		switch s.Role {
		case StandardRolePrimary, StandardRoleReinforcement:
		default:
			return fmt.Errorf("standards[%d]: unknown role %q", i, s.Role)
		}
	}
	if err := ValidateContent(doc.Kind, &doc.Content); err != nil {
		return err
	}
	if err := validateReferenceSolution(doc.ReferenceSolution); err != nil {
		return err
	}
	return nil
}

func validateReferenceSolution(ref *ReferenceSolution) error {
	if ref == nil {
		return nil
	}
	if len(ref.Steps) == 0 {
		return fmt.Errorf("referenceSolution.steps must not be empty")
	}
	for i, step := range ref.Steps {
		if len(step.Argv) == 0 {
			return fmt.Errorf("referenceSolution.steps[%d].argv must not be empty", i)
		}
		for j, a := range step.Argv {
			if strings.TrimSpace(a) == "" {
				return fmt.Errorf("referenceSolution.steps[%d].argv[%d] must not be empty", i, j)
			}
		}
		if step.WorkDir != "" {
			if _, err := SafeRelPath(step.WorkDir); err != nil {
				return fmt.Errorf("referenceSolution.steps[%d].workDir: %w", i, err)
			}
		}
	}
	return nil
}

// ValidateContent validates the revision content body for a kind.
func ValidateContent(kind string, c *ActivityContent) error {
	if c == nil {
		return fmt.Errorf("content is nil")
	}
	if strings.TrimSpace(c.Objective) == "" {
		return fmt.Errorf("content.objective is required")
	}
	if strings.TrimSpace(c.Instructions) == "" {
		return fmt.Errorf("content.instructions is required")
	}
	switch kind {
	case KindTerminal:
		if c.Terminal == nil {
			return fmt.Errorf("content.terminal is required for kind %s", kind)
		}
		if c.Typing != nil {
			return fmt.Errorf("content.typing must be empty for kind %s", kind)
		}
		if err := validateTerminal(c.Terminal); err != nil {
			return err
		}
	case KindTyping:
		if c.Typing == nil {
			return fmt.Errorf("content.typing is required for kind %s", kind)
		}
		if c.Terminal != nil {
			return fmt.Errorf("content.terminal must be empty for kind %s", kind)
		}
		if err := validateTyping(c.Typing); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown kind %q", kind)
	}

	checkIDs := map[string]bool{}
	for i, ch := range c.Checks {
		if err := validateCheck(i, ch, checkIDs); err != nil {
			return err
		}
	}
	if len(c.Checks) == 0 {
		return fmt.Errorf("content.checks must not be empty")
	}

	hintIDs := map[string]bool{}
	for i, h := range c.Hints {
		if !idRE.MatchString(h.ID) {
			return fmt.Errorf("hints[%d]: invalid id %q", i, h.ID)
		}
		if hintIDs[h.ID] {
			return fmt.Errorf("hints[%d]: duplicate id %q", i, h.ID)
		}
		hintIDs[h.ID] = true
		if strings.TrimSpace(h.Text) == "" {
			return fmt.Errorf("hints[%d]: text is required", i)
		}
	}

	if err := validateInstructionBlocks(c.Blocks); err != nil {
		return err
	}

	if len(c.Tasks) == 0 {
		return fmt.Errorf("content.tasks must not be empty")
	}
	taskIDs := map[string]bool{}
	for i, t := range c.Tasks {
		if !idRE.MatchString(t.ID) {
			return fmt.Errorf("tasks[%d]: invalid id %q", i, t.ID)
		}
		if taskIDs[t.ID] {
			return fmt.Errorf("tasks[%d]: duplicate id %q", i, t.ID)
		}
		taskIDs[t.ID] = true
		if strings.TrimSpace(t.Title) == "" {
			return fmt.Errorf("tasks[%d]: title is required", i)
		}
		if strings.TrimSpace(t.Instructions) == "" {
			return fmt.Errorf("tasks[%d]: instructions are required", i)
		}
		kind := t.Kind
		if kind == "" {
			kind = TaskKindAction
		}
		switch kind {
		case TaskKindAction:
			if t.Response != nil {
				return fmt.Errorf("tasks[%d]: response is only valid for short_response tasks", i)
			}
		case TaskKindShortResponse:
			if err := validateResponseSpec(fmt.Sprintf("tasks[%d].response", i), t.Response); err != nil {
				return err
			}
		default:
			return fmt.Errorf("tasks[%d]: unknown kind %q", i, t.Kind)
		}
		for _, pre := range t.Prerequisites {
			if !taskIDs[pre] && pre != t.ID {
				// prerequisites may only reference earlier tasks; full graph check below
			}
			if !idRE.MatchString(pre) {
				return fmt.Errorf("tasks[%d]: invalid prerequisite id %q", i, pre)
			}
		}
		for _, hid := range t.HintIDs {
			if !hintIDs[hid] {
				return fmt.Errorf("tasks[%d]: unknown hint id %q", i, hid)
			}
		}
		if err := validateCheckTree(fmt.Sprintf("tasks[%d].completion", i), t.Completion, checkIDs); err != nil {
			return err
		}
	}
	for i, t := range c.Tasks {
		for _, pre := range t.Prerequisites {
			if !taskIDs[pre] {
				return fmt.Errorf("tasks[%d]: unknown prerequisite %q", i, pre)
			}
			if pre == t.ID {
				return fmt.Errorf("tasks[%d]: task cannot prerequisite itself", i)
			}
		}
	}
	if err := detectTaskCycles(c.Tasks); err != nil {
		return err
	}

	if c.Artifacts != nil && c.Artifacts.Enabled {
		if c.Artifacts.MaxFiles < 0 || c.Artifacts.MaxBytesEach < 0 || c.Artifacts.MaxBytesTotal < 0 {
			return fmt.Errorf("artifacts: limits must be non-negative")
		}
	}
	return nil
}

func validateTerminal(t *TerminalContent) error {
	switch t.RuntimeProfile {
	case RuntimeCoreutilsBasic, RuntimeTextProcessing:
	case "":
		return fmt.Errorf("terminal.runtime_profile is required")
	default:
		return fmt.Errorf("terminal.runtime_profile %q is unknown", t.RuntimeProfile)
	}
	if t.InitialCwd != "" {
		if _, err := SafeRelPath(t.InitialCwd); err != nil {
			return fmt.Errorf("terminal.initial_cwd: %w", err)
		}
	}
	seen := map[string]bool{}
	for i, f := range t.Fixtures {
		rel, err := SafeRelPath(f.Path)
		if err != nil {
			return fmt.Errorf("terminal.fixtures[%d].path: %w", i, err)
		}
		if seen[rel] {
			return fmt.Errorf("terminal.fixtures[%d]: duplicate path %q", i, rel)
		}
		seen[rel] = true
		switch f.Type {
		case FixtureFile, FixtureDirectory:
		default:
			return fmt.Errorf("terminal.fixtures[%d]: unknown type %q", i, f.Type)
		}
		if f.Type == FixtureDirectory && f.Content != "" {
			return fmt.Errorf("terminal.fixtures[%d]: directory must not have content", i)
		}
		if f.Mode != "" && !modeRE.MatchString(f.Mode) {
			return fmt.Errorf("terminal.fixtures[%d]: invalid mode %q", i, f.Mode)
		}
	}
	return nil
}

func validateTyping(t *TypingContent) error {
	if strings.TrimSpace(t.PromptSetID) == "" {
		return fmt.Errorf("typing.prompt_set_id is required")
	}
	if len(t.Prompts) == 0 {
		return fmt.Errorf("typing.prompts must not be empty")
	}
	seen := map[string]bool{}
	for i, p := range t.Prompts {
		if !idRE.MatchString(p.ID) {
			return fmt.Errorf("typing.prompts[%d]: invalid id %q", i, p.ID)
		}
		if seen[p.ID] {
			return fmt.Errorf("typing.prompts[%d]: duplicate id %q", i, p.ID)
		}
		seen[p.ID] = true
		if strings.TrimSpace(p.Text) == "" {
			return fmt.Errorf("typing.prompts[%d]: text is required", i)
		}
	}
	if t.TimeLimitSec < 0 {
		return fmt.Errorf("typing.time_limit_sec must be non-negative")
	}
	return nil
}

func validateCheck(i int, ch Check, seen map[string]bool) error {
	if !idRE.MatchString(ch.ID) {
		return fmt.Errorf("checks[%d]: invalid id %q", i, ch.ID)
	}
	if seen[ch.ID] {
		return fmt.Errorf("checks[%d]: duplicate id %q", i, ch.ID)
	}
	seen[ch.ID] = true
	if ch.Params == nil {
		ch.Params = map[string]any{}
	}
	if err := validateCheckStages(i, ch); err != nil {
		return err
	}
	switch ch.Kind {
	case CheckFileExists, CheckFileNotExists:
		if err := requireExactParams(ch.Params, "path"); err != nil {
			return fmt.Errorf("checks[%d] (%s): %w", i, ch.ID, err)
		}
		if err := requireSafePathParam(ch.Params, "path"); err != nil {
			return fmt.Errorf("checks[%d] (%s): %w", i, ch.ID, err)
		}
	case CheckContentEquals, CheckContentContains:
		if err := requireExactParams(ch.Params, "path", "value"); err != nil {
			return fmt.Errorf("checks[%d] (%s): %w", i, ch.ID, err)
		}
		if err := requireSafePathParam(ch.Params, "path"); err != nil {
			return fmt.Errorf("checks[%d] (%s): %w", i, ch.ID, err)
		}
		if _, ok := stringParam(ch.Params, "value"); !ok {
			return fmt.Errorf("checks[%d] (%s): params.value is required", i, ch.ID)
		}
	case CheckContentMatch:
		if err := requireExactParams(ch.Params, "path", "pattern"); err != nil {
			return fmt.Errorf("checks[%d] (%s): %w", i, ch.ID, err)
		}
		if err := requireSafePathParam(ch.Params, "path"); err != nil {
			return fmt.Errorf("checks[%d] (%s): %w", i, ch.ID, err)
		}
		pat, ok := stringParam(ch.Params, "pattern")
		if !ok || pat == "" {
			return fmt.Errorf("checks[%d] (%s): params.pattern is required", i, ch.ID)
		}
		if _, err := regexp.Compile(pat); err != nil {
			return fmt.Errorf("checks[%d] (%s): invalid pattern: %w", i, ch.ID, err)
		}
	case CheckPathType:
		if err := requireExactParams(ch.Params, "path", "type"); err != nil {
			return fmt.Errorf("checks[%d] (%s): %w", i, ch.ID, err)
		}
		if err := requireSafePathParam(ch.Params, "path"); err != nil {
			return fmt.Errorf("checks[%d] (%s): %w", i, ch.ID, err)
		}
		pt, ok := stringParam(ch.Params, "type")
		if !ok {
			return fmt.Errorf("checks[%d] (%s): params.type is required", i, ch.ID)
		}
		switch pt {
		case PathTypeFile, PathTypeDirectory, PathTypeSymlink:
		default:
			return fmt.Errorf("checks[%d] (%s): unknown path type %q", i, ch.ID, pt)
		}
	case CheckPathMode:
		if err := requireExactParams(ch.Params, "path", "mode"); err != nil {
			return fmt.Errorf("checks[%d] (%s): %w", i, ch.ID, err)
		}
		if err := requireSafePathParam(ch.Params, "path"); err != nil {
			return fmt.Errorf("checks[%d] (%s): %w", i, ch.ID, err)
		}
		mode, ok := stringParam(ch.Params, "mode")
		if !ok || !modeRE.MatchString(mode) {
			return fmt.Errorf("checks[%d] (%s): params.mode must be octal like 0644", i, ch.ID)
		}
	case CheckCwd:
		if err := requireExactParams(ch.Params, "path"); err != nil {
			return fmt.Errorf("checks[%d] (%s): %w", i, ch.ID, err)
		}
		if err := requireSafePathParam(ch.Params, "path"); err != nil {
			return fmt.Errorf("checks[%d] (%s): %w", i, ch.ID, err)
		}
	case CheckCommandProperties:
		allowed := map[string]bool{
			"executable": true, "args": true, "exitCode": true,
			"stdoutContains": true, "stdoutEquals": true, "stdoutPattern": true,
			"stderrContains": true, "stderrEquals": true, "stderrPattern": true,
		}
		if err := requireParamsSubset(ch.Params, allowed); err != nil {
			return fmt.Errorf("checks[%d] (%s): %w", i, ch.ID, err)
		}
		if _, ok := stringParam(ch.Params, "executable"); !ok {
			return fmt.Errorf("checks[%d] (%s): params.executable is required", i, ch.ID)
		}
		if args, ok := ch.Params["args"]; ok {
			if _, err := stringSlice(args); err != nil {
				return fmt.Errorf("checks[%d] (%s): params.args must be a string list", i, ch.ID)
			}
		}
		if v, ok := ch.Params["exitCode"]; ok {
			if _, err := intParam(v); err != nil {
				return fmt.Errorf("checks[%d] (%s): params.exitCode must be an int", i, ch.ID)
			}
		}
		for _, key := range []string{"stdoutPattern", "stderrPattern"} {
			if pat, ok := stringParam(ch.Params, key); ok && pat != "" {
				if _, err := regexp.Compile(pat); err != nil {
					return fmt.Errorf("checks[%d] (%s): invalid %s: %w", i, ch.ID, key, err)
				}
			}
		}
	case CheckPipelineOutput:
		allowed := map[string]bool{"value": true, "contains": true, "pattern": true}
		if err := requireParamsSubset(ch.Params, allowed); err != nil {
			return fmt.Errorf("checks[%d] (%s): %w", i, ch.ID, err)
		}
		if _, ok := stringParam(ch.Params, "value"); !ok {
			if _, ok2 := stringParam(ch.Params, "contains"); !ok2 {
				if _, ok3 := stringParam(ch.Params, "pattern"); !ok3 {
					return fmt.Errorf("checks[%d] (%s): one of value, contains, or pattern is required", i, ch.ID)
				}
			}
		}
		if pat, ok := stringParam(ch.Params, "pattern"); ok && pat != "" {
			if _, err := regexp.Compile(pat); err != nil {
				return fmt.Errorf("checks[%d] (%s): invalid pattern: %w", i, ch.ID, err)
			}
		}
	case CheckTypingMetrics:
		allowed := map[string]bool{
			"min_wpm": true, "minWpm": true,
			"min_accuracy": true, "minAccuracy": true,
		}
		if err := requireParamsSubset(ch.Params, allowed); err != nil {
			return fmt.Errorf("checks[%d] (%s): %w", i, ch.ID, err)
		}
		if _, err := floatParam(ch.Params["min_wpm"]); err != nil {
			// Also accept camelCase from JSON content.
			if _, err2 := floatParam(ch.Params["minWpm"]); err2 != nil {
				return fmt.Errorf("checks[%d] (%s): params.min_wpm is required and must be a number", i, ch.ID)
			}
		}
		if _, err := floatParam(ch.Params["min_accuracy"]); err != nil {
			if _, err2 := floatParam(ch.Params["minAccuracy"]); err2 != nil {
				return fmt.Errorf("checks[%d] (%s): params.min_accuracy is required and must be a number", i, ch.ID)
			}
		}
		acc, err := typingMinAccuracy(ch.Params)
		if err != nil {
			return fmt.Errorf("checks[%d] (%s): %w", i, ch.ID, err)
		}
		if acc < 0 || acc > 1 {
			return fmt.Errorf("checks[%d] (%s): params.min_accuracy must be between 0 and 1", i, ch.ID)
		}
		wpm, err := typingMinWPM(ch.Params)
		if err != nil {
			return fmt.Errorf("checks[%d] (%s): %w", i, ch.ID, err)
		}
		if wpm < 0 {
			return fmt.Errorf("checks[%d] (%s): params.min_wpm must be non-negative", i, ch.ID)
		}
	case CheckResponseSubmitted:
		allowed := map[string]bool{"taskId": true, "task_id": true}
		if err := requireParamsSubset(ch.Params, allowed); err != nil {
			return fmt.Errorf("checks[%d] (%s): %w", i, ch.ID, err)
		}
		tid, ok := stringParam(ch.Params, "taskId")
		if !ok {
			tid, ok = stringParam(ch.Params, "task_id")
		}
		if !ok || !idRE.MatchString(tid) {
			return fmt.Errorf("checks[%d] (%s): params.taskId is required", i, ch.ID)
		}
	default:
		return fmt.Errorf("checks[%d] (%s): unknown kind %q", i, ch.ID, ch.Kind)
	}
	return nil
}

func validateInstructionBlocks(blocks []InstructionBlock) error {
	seen := map[string]bool{}
	for i, b := range blocks {
		if !idRE.MatchString(b.ID) {
			return fmt.Errorf("blocks[%d]: invalid id %q", i, b.ID)
		}
		if seen[b.ID] {
			return fmt.Errorf("blocks[%d]: duplicate id %q", i, b.ID)
		}
		seen[b.ID] = true
		switch b.Kind {
		case BlockProse, BlockWarning, BlockQuestion, BlockPractice, BlockParentNote:
			if strings.TrimSpace(b.Text) == "" {
				return fmt.Errorf("blocks[%d] (%s): text is required", i, b.ID)
			}
			if len(b.Text) > MaxInstructionBlockText {
				return fmt.Errorf("blocks[%d] (%s): text exceeds %d characters", i, b.ID, MaxInstructionBlockText)
			}
			if looksLikeUnsafeMarkup(b.Text) {
				return fmt.Errorf("blocks[%d] (%s): unsafe markup or links are not allowed", i, b.ID)
			}
			if len(b.Terms) > 0 || b.Resource != nil || b.Input != "" || b.Output != "" {
				return fmt.Errorf("blocks[%d] (%s): unexpected fields for kind %s", i, b.ID, b.Kind)
			}
		case BlockVocabulary:
			if len(b.Terms) == 0 {
				return fmt.Errorf("blocks[%d] (%s): terms are required", i, b.ID)
			}
			for j, term := range b.Terms {
				if strings.TrimSpace(term.Term) == "" || strings.TrimSpace(term.Definition) == "" {
					return fmt.Errorf("blocks[%d].terms[%d]: term and definition are required", i, j)
				}
				if looksLikeUnsafeMarkup(term.Term) || looksLikeUnsafeMarkup(term.Definition) {
					return fmt.Errorf("blocks[%d].terms[%d]: unsafe markup is not allowed", i, j)
				}
			}
			if b.Text != "" || b.Resource != nil {
				return fmt.Errorf("blocks[%d] (%s): unexpected fields for vocabulary", i, b.ID)
			}
		case BlockExample:
			if strings.TrimSpace(b.Input) == "" && strings.TrimSpace(b.Output) == "" && strings.TrimSpace(b.Explanation) == "" {
				return fmt.Errorf("blocks[%d] (%s): example needs input, output, or explanation", i, b.ID)
			}
			for _, s := range []string{b.Input, b.Output, b.Explanation, b.Text} {
				if looksLikeUnsafeMarkup(s) {
					return fmt.Errorf("blocks[%d] (%s): unsafe markup is not allowed", i, b.ID)
				}
			}
			if len(b.Terms) > 0 || b.Resource != nil {
				return fmt.Errorf("blocks[%d] (%s): unexpected fields for example", i, b.ID)
			}
		case BlockResource:
			if b.Resource == nil {
				return fmt.Errorf("blocks[%d] (%s): resource is required", i, b.ID)
			}
			if err := validateResourceRef(fmt.Sprintf("blocks[%d].resource", i), b.Resource); err != nil {
				return err
			}
			if b.Text != "" || len(b.Terms) > 0 {
				return fmt.Errorf("blocks[%d] (%s): unexpected fields for resource", i, b.ID)
			}
		default:
			return fmt.Errorf("blocks[%d] (%s): unknown kind %q", i, b.ID, b.Kind)
		}
	}
	return nil
}

func validateResourceRef(path string, r *ResourceRef) error {
	if r == nil {
		return fmt.Errorf("%s is required", path)
	}
	if !sha256HexRE.MatchString(r.SHA256) {
		return fmt.Errorf("%s.sha256 must be 64 hex chars", path)
	}
	if strings.TrimSpace(r.Label) == "" {
		return fmt.Errorf("%s.label is required", path)
	}
	if strings.TrimSpace(r.MediaType) == "" {
		return fmt.Errorf("%s.mediaType is required", path)
	}
	// Reject remote schemes in labels/media types that look like URLs.
	if strings.Contains(r.Label, "://") || strings.HasPrefix(strings.ToLower(r.MediaType), "text/html") {
		return fmt.Errorf("%s: remote or HTML resources are not allowed", path)
	}
	if r.ByteSize < 0 || r.ByteSize > MaxResourceBytes {
		return fmt.Errorf("%s.byteSize must be between 0 and %d", path, MaxResourceBytes)
	}
	return nil
}

func validateResponseSpec(path string, spec *ResponseTaskSpec) error {
	if spec == nil {
		return fmt.Errorf("%s is required for short_response tasks", path)
	}
	if strings.TrimSpace(spec.Prompt) == "" {
		return fmt.Errorf("%s.prompt is required", path)
	}
	if looksLikeUnsafeMarkup(spec.Prompt) {
		return fmt.Errorf("%s.prompt: unsafe markup is not allowed", path)
	}
	max := spec.MaxChars
	if max == 0 {
		max = DefaultResponseMaxChars
	}
	if max < 1 || max > MaxResponseMaxChars {
		return fmt.Errorf("%s.maxChars must be between 1 and %d", path, MaxResponseMaxChars)
	}
	if len(spec.Rubric) == 0 {
		return fmt.Errorf("%s.rubric must not be empty", path)
	}
	seen := map[string]bool{}
	for i, c := range spec.Rubric {
		if !idRE.MatchString(c.ID) {
			return fmt.Errorf("%s.rubric[%d]: invalid id %q", path, i, c.ID)
		}
		if seen[c.ID] {
			return fmt.Errorf("%s.rubric[%d]: duplicate id %q", path, i, c.ID)
		}
		seen[c.ID] = true
		if strings.TrimSpace(c.Description) == "" {
			return fmt.Errorf("%s.rubric[%d]: description is required", path, i)
		}
	}
	return nil
}

var (
	sha256HexRE   = regexp.MustCompile(`^[a-fA-F0-9]{64}$`)
	unsafeLinkRE  = regexp.MustCompile(`(?i)(https?://|javascript:|data:)`)
	unsafeTagRE   = regexp.MustCompile(`(?i)<\s*(script|iframe|object|embed|link|meta|style)\b`)
)

func looksLikeUnsafeMarkup(s string) bool {
	if s == "" {
		return false
	}
	if unsafeLinkRE.MatchString(s) {
		return true
	}
	if unsafeTagRE.MatchString(s) {
		return true
	}
	return false
}

// StudentBlocks returns instructional blocks visible to the student client
// (excludes parent_note).
func StudentBlocks(blocks []InstructionBlock) []InstructionBlock {
	if len(blocks) == 0 {
		return nil
	}
	out := make([]InstructionBlock, 0, len(blocks))
	for _, b := range blocks {
		if b.Kind == BlockParentNote {
			continue
		}
		out = append(out, b)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// TaskKindOrDefault returns the effective task kind.
func TaskKindOrDefault(t Task) string {
	if t.Kind == "" {
		return TaskKindAction
	}
	return t.Kind
}

func validateCheckStages(i int, ch Check) error {
	seenStage := map[string]bool{}
	for _, s := range ch.Stages {
		switch s {
		case StageFixture, StageTask, StageFinal:
		default:
			return fmt.Errorf("checks[%d] (%s): unknown stage %q", i, ch.ID, s)
		}
		if seenStage[s] {
			return fmt.Errorf("checks[%d] (%s): duplicate stage %q", i, ch.ID, s)
		}
		seenStage[s] = true
	}
	seenInv := map[string]bool{}
	for _, b := range ch.InvariantAt {
		switch b {
		case InvariantAtFixture, InvariantAtAfterTask, InvariantAtFinal:
		default:
			return fmt.Errorf("checks[%d] (%s): unknown invariantAt %q", i, ch.ID, b)
		}
		if seenInv[b] {
			return fmt.Errorf("checks[%d] (%s): duplicate invariantAt %q", i, ch.ID, b)
		}
		seenInv[b] = true
	}
	return nil
}

func requireExactParams(params map[string]any, keys ...string) error {
	allowed := make(map[string]bool, len(keys))
	for _, k := range keys {
		allowed[k] = true
	}
	return requireParamsSubset(params, allowed)
}

func requireParamsSubset(params map[string]any, allowed map[string]bool) error {
	for k := range params {
		if !allowed[k] {
			return fmt.Errorf("unknown params.%s", k)
		}
	}
	return nil
}

func validateCheckTree(prefix string, tree CheckTree, checkIDs map[string]bool) error {
	n := 0
	if tree.CheckID != "" {
		n++
	}
	if len(tree.All) > 0 {
		n++
	}
	if len(tree.Any) > 0 {
		n++
	}
	if n != 1 {
		return fmt.Errorf("%s: exactly one of check_id, all, or any is required", prefix)
	}
	if tree.CheckID != "" {
		if !checkIDs[tree.CheckID] {
			return fmt.Errorf("%s: unknown check_id %q", prefix, tree.CheckID)
		}
		return nil
	}
	children := tree.All
	label := "all"
	if len(tree.Any) > 0 {
		children = tree.Any
		label = "any"
	}
	if len(children) == 0 {
		return fmt.Errorf("%s.%s must not be empty", prefix, label)
	}
	for i, child := range children {
		if err := validateCheckTree(fmt.Sprintf("%s.%s[%d]", prefix, label, i), child, checkIDs); err != nil {
			return err
		}
	}
	return nil
}

func detectTaskCycles(tasks []Task) error {
	idx := map[string]int{}
	for i, t := range tasks {
		idx[t.ID] = i
	}
	const (
		white = 0
		gray  = 1
		black = 2
	)
	color := make([]int, len(tasks))
	var visit func(int) error
	visit = func(i int) error {
		if color[i] == gray {
			return fmt.Errorf("tasks: prerequisite cycle involving %q", tasks[i].ID)
		}
		if color[i] == black {
			return nil
		}
		color[i] = gray
		for _, pre := range tasks[i].Prerequisites {
			j, ok := idx[pre]
			if !ok {
				continue
			}
			if err := visit(j); err != nil {
				return err
			}
		}
		color[i] = black
		return nil
	}
	for i := range tasks {
		if err := visit(i); err != nil {
			return err
		}
	}
	return nil
}

func requireSafePathParam(params map[string]any, key string) error {
	p, ok := stringParam(params, key)
	if !ok || p == "" {
		return fmt.Errorf("params.%s is required", key)
	}
	if _, err := SafeRelPath(p); err != nil {
		return err
	}
	return nil
}

func stringParam(params map[string]any, key string) (string, bool) {
	v, ok := params[key]
	if !ok || v == nil {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

func intParam(v any) (int, error) {
	switch n := v.(type) {
	case int:
		return n, nil
	case int64:
		return int(n), nil
	case float64:
		return int(n), nil
	case jsonNumber:
		i, err := n.Int64()
		if err != nil {
			return 0, err
		}
		return int(i), nil
	case string:
		return strconv.Atoi(n)
	default:
		return 0, fmt.Errorf("not an int")
	}
}

func floatParam(v any) (float64, error) {
	switch n := v.(type) {
	case float64:
		return n, nil
	case float32:
		return float64(n), nil
	case int:
		return float64(n), nil
	case int64:
		return float64(n), nil
	case jsonNumber:
		return n.Float64()
	case string:
		return strconv.ParseFloat(n, 64)
	default:
		return 0, fmt.Errorf("not a number")
	}
}

// jsonNumber is satisfied by encoding/json.Number without importing encoding/json here.
type jsonNumber interface {
	Float64() (float64, error)
	Int64() (int64, error)
}

// TypingMinWPM extracts min_wpm / minWpm from typing_metrics check params.
func TypingMinWPM(params map[string]any) (float64, error) {
	return typingMinWPM(params)
}

// TypingMinAccuracy extracts min_accuracy / minAccuracy from typing_metrics check params.
func TypingMinAccuracy(params map[string]any) (float64, error) {
	return typingMinAccuracy(params)
}

func typingMinWPM(params map[string]any) (float64, error) {
	if params == nil {
		return 0, fmt.Errorf("params.min_wpm is required")
	}
	if v, err := floatParam(params["min_wpm"]); err == nil {
		return v, nil
	}
	if v, err := floatParam(params["minWpm"]); err == nil {
		return v, nil
	}
	return 0, fmt.Errorf("params.min_wpm is required")
}

func typingMinAccuracy(params map[string]any) (float64, error) {
	if params == nil {
		return 0, fmt.Errorf("params.min_accuracy is required")
	}
	if v, err := floatParam(params["min_accuracy"]); err == nil {
		return v, nil
	}
	if v, err := floatParam(params["minAccuracy"]); err == nil {
		return v, nil
	}
	return 0, fmt.Errorf("params.min_accuracy is required")
}

func stringSlice(v any) ([]string, error) {
	switch t := v.(type) {
	case []string:
		return t, nil
	case []any:
		out := make([]string, 0, len(t))
		for _, x := range t {
			s, ok := x.(string)
			if !ok {
				return nil, fmt.Errorf("non-string element")
			}
			out = append(out, s)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("not a list")
	}
}

// ParseMode parses a fixture/check mode string like "0644" or "755".
func ParseMode(s string) (uint32, error) {
	if !modeRE.MatchString(s) {
		return 0, fmt.Errorf("invalid mode %q", s)
	}
	v, err := strconv.ParseUint(s, 8, 32)
	if err != nil {
		return 0, err
	}
	return uint32(v), nil
}
