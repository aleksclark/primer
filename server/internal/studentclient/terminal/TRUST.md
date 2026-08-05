# Terminal evidence trust boundaries (Phase 2 MVP)

## Architecture chosen

**Option B hybrid with bash observe hooks (pragmatic MVP toward Option A).**

- **Scripted path (`RunLine` / process-wait):** stdout/stderr/exit are captured from OS pipes. argv is recovered from the submitted line when the runner wraps `sh -c`. These events set `Structured=true` when exit + cwd + argv quality meet the bar. Source: `structured`.
- **Interactive PTY path:** broker/engine starts bash with a generated `--rcfile` that installs `DEBUG` + `PROMPT_COMMAND` hooks. Hooks append NDJSON command boundary events to a **broker-owned spool directory outside the student workspace**. The engine drains the spool after newline idle / explicit `Verify` and feeds `ShellEvent`s into the terminal runner. Source: `observe-bash`.
- **Removed:** idle-screen → synthetic `pty-shell` success with screen text as stdout.

## Trusted when instrumentation is healthy

| Field | Scripted process-wait | Observe-bash (PTY) |
|-------|----------------------|--------------------|
| Exit code | yes | yes (`$?` at prompt) |
| Cwd after | yes (runner tracks) | yes (`pwd -P`) |
| Executable/argv | yes (parsed line / direct argv) | best-effort parse of `BASH_COMMAND` |
| Stdout/stderr text | yes (pipes) | **no** (not captured) |
| Workspace manifest digest | yes (before/after walk) | after-command digest attached on drain |

`StructuredCommandEvidence` / capability is granted only when `EvidenceQuality.MeetsStructuredBar()` — currently **exit + cwd + argv**.

## Not trusted

- PTY screen scrollback / prompt text as command stdout or stderr
- Student-forged content inside the workspace as control frames (events go to the outside spool; student can disrupt hooks → missing evidence, not forged success)
- Complex bash constructs (`$(…)`, backticks, unclosed quotes): argv marked unavailable; outcome/filesystem checks still apply
- Other shells (no zsh/fish integration)

## Fail closed

- If bash/observe setup fails, capability may still be advertised when bash exists for future sessions, but **this session** will not produce observe events; command-sensitive checks fail until scripted `RunLine` or a successful observe event appears.
- Malformed NDJSON lines are skipped.
- Closing/corrupting the event file yields missing evidence, never synthetic success.

## Gaps vs full Phase 2 plan

- No MAC-signed launcher frames or out-of-process root launcher (Option A complete design)
- No 50-form Bash corpus stress suite / 100-run PTY soak
- No physical UAT of lessons 1/9/13/15/16/19
- Stdout/stderr still untrusted on interactive PTY (pipeline_output needs scripted path or future pipe capture)
- No server-side event sequence MAC validation upload path beyond existing outbox
- Workspace manifests are pragmatic digests (bounded walk), not full provenance audit
