# Phase 9: Opt-in live billable LLM qualification

## Goal

Qualify one narrowly bounded, real billable LLM request through the deployed `primer-agents` process and its Identity-authenticated HTTP boundary. This phase turns the Phase 8 live-provider blocker into explicit evidence without changing the safety contract of CI, `make test`, `make agents-test`, or ordinary local development. It is a qualification probe, not a product rollout, efficacy claim, load test, or permission to accept ambient provider credentials.

## BDD Success Criteria

#### Scenario: Ordinary test commands never make a billable call

- **Given** a checkout with ambient `OPENAI_API_KEY`, other provider keys, or no provider keys
- **When** `make test`, `make agents-test`, the default Go test packages, or ordinary `primer-agents` startup runs
- **Then** no billable provider endpoint is called
- **And** no ambient provider-key variable is read
- **And** the deterministic/noop provider remains the default

#### Scenario: Live qualification requires two explicit opt-ins

- **Given** `PRIMER_AGENTS_LIVE_LLM` is absent or not exactly `1`
- **When** the live target or live-tagged harness is invoked
- **Then** it refuses before starting `primer-agents` or making a provider request
- **Given** the flag is `1` but a clearly named live env file/key is absent
- **Then** it fails closed without making a provider request
- **And** secrets are never taken from `.env`, `OPENAI_API_KEY`, shell history, or another service's configuration

#### Scenario: One cheap request crosses the real service boundary

- **Given** the explicit live opt-in, approved provider key, explicit approved HTTPS provider URL, agents-only test database, and loopback Identity JWKS
- **When** the harness starts the real `primer-agents` executable, waits for readiness, and submits one tiny parent/tutor prompt with a strict Identity test JWT (`aud=primer-agents`)
- **Then** the request is authenticated by the loopback Identity/JWKS path, queued, executed by the real worker, and completed by the real provider adapter
- **And** the harness does not call LMS code or an in-process agents app
- **And** the fixed cheap model, tiny prompt, no-tools tutor profile, one-request budget, and hard run timeout are enforced

#### Scenario: Safety negatives fail closed

- **Given** a missing key, wrong JWT audience, student profile/token, malformed key, non-HTTPS/loopback provider URL, production-looking default URL, or a provider response that would require tool looping
- **When** the live harness validates configuration or submits a probe
- **Then** it refuses or records a failed qualification before an unsafe billable call
- **And** the student route/profile is never used by this qualification
- **And** the harness never retries or follows a tool loop

#### Scenario: Qualification evidence is redacted and reproducible

- **When** the probe completes or fails
- **Then** it records the repository SHA, fixed model, request count, process-boundary and Identity-loopback assertions, outcome, and `secrets_in_logs=false`
- **And** logs, reports, test output, HTTP errors, and persisted event assertions contain no provider key or bearer token
- **And** the evidence distinguishes `SKIPPED/REFUSED` from a successful live-provider qualification

## Implementation Instructions

- Add a live-only harness under `primer-agents/internal/testutil/live/` with a build tag such as `live_llm`; keep it out of every default package and Make target. It must launch the built `cmd/primer-agents` binary as a separate OS process against an agents-only PostgreSQL test database, not call `app.Run` in-process.
- Require `PRIMER_AGENTS_LIVE_LLM=1` exactly. Load only an explicitly named `PRIMER_AGENTS_LIVE_LLM_ENV_FILE`, or the clearly named `$HOME/.config/primer/primer-agents-live-llm.env` when it exists; parse only `PRIMER_AGENTS_LIVE_*` settings and never fall back to ambient provider variables. Do not commit the file or secrets.
- Use one fixed inexpensive model and an explicit provider URL; do not provide a production endpoint default. Require HTTPS and an allowlisted approved provider host/path, reject loopback and production-looking accidental defaults, and refuse missing/malformed credentials. Pass the key to the child process only through the opt-in live namespaced environment.
- Compose the provider only when the live opt-in is enabled. Disable function/tool auto-call, use the tutor profile, cap the run context with a hard timeout, and issue exactly one short prompt. Ordinary worker composition must continue using the deterministic noop provider and must not inspect ambient provider keys.
- Start a loopback JWKS/Identity fixture, mint a strict ES256 test JWT whose exact audience is `primer-agents`, and use the bearer token only for the HTTP request. Include wrong-audience and student-profile checks that prove refusal without provider traffic.
- Write a machine-readable report (defaulting to an ignored local `tmp/` path) containing SHA, model, request count, status, process boundary, Identity loopback, and secret-log scan results. Never include the key, token, full prompt, raw provider error, or unrestricted response text.
- Add `make agents-live-llm` as the only convenient entry point. It must gate the flag before Go is invoked, use the live build tag, set a bounded timeout, and remain absent from `agents-test`, default CI, coverage, and race targets. Do not change coverage thresholds and do not implement S19, MCP, or Stytch work in this phase.

## End-to-End Test Plan

- With no flag, no key, only ambient `OPENAI_API_KEY`, wrong audience, student scope/profile, missing URL, and loopback URL, run the refusal matrix and assert zero provider requests and no child process for preflight failures.
- With an approved key in the named live env file, build `primer-agents`, start the real process with a disposable agents PostgreSQL database and loopback Identity JWKS, wait for `/readyz`, submit one authenticated tutor run, poll through the HTTP API, and assert a successful terminal run plus bounded persisted events.
- Kill the process or cancel the run if the hard timeout fires; assert the harness exits within its outer timeout and never retries the provider call. Capture stdout/stderr and scan for planted key/token markers.
- Preserve the JSON evidence artifact and command, SHA, model, request count, and outcome for human review. A skipped/refused run is not live-provider evidence.

## Anti-Cheating Audit

- Inspect the live build tag, Make target, environment loader, and child-process environment; reject any `OPENAI_API_KEY`/`ANTHROPIC_API_KEY` fallback, `.env` scraping, committed secret, or default billable URL.
- Trace the successful request from the external executable through Identity JWT/JWKS, HTTP API, durable queue/worker, MAF provider adapter, and PostgreSQL; reject an in-process shortcut, fake provider, direct handler invocation, hard-coded success, or direct database status patch.
- Verify exactly one fixed cheap model, one tiny prompt, no student profile, no tools, no retries, and a hard timeout. Treat a provider/tool loop or an unbounded request count as a failed qualification.
- Inspect logs, child environment handling, reports, and persisted events for provider keys, bearer JWTs, full prompts, raw response content, DSNs, and unbounded error text. Verify SHA/model/request-count fields are present and truthful.
- Run ordinary `make test`, `make agents-test`, `make agents-race`, and CI-path checks with planted ambient provider keys and confirm no live-tagged test or provider call is selected. Do not lower coverage gates or reclassify a skipped run as proof.

## Completion Gate

- [ ] Phase docs are linked from the plan index and explicitly replace the Phase 8 live-LLM blocker only for this opt-in qualification.
- [ ] Default CI, `make test`, `make agents-test`, and ordinary service startup never read ambient provider keys or call a billable endpoint.
- [ ] Missing opt-in, missing/malformed key, wrong audience, student profile, unsafe URL, and tool-loop probes fail closed before an unsafe call.
- [ ] The approved-key run, when authorized and available, succeeds through the real `primer-agents` process, loopback Identity JWT/JWKS, HTTP API, worker, provider, and agents PostgreSQL.
- [ ] Hard timeout, one fixed cheap model, tiny prompt, one-request budget, no-tools tutor policy, and no-retry evidence is recorded.
- [ ] Evidence records SHA, model, request count, process/Identity assertions, outcome, and `secrets_in_logs=false` without secrets or unrestricted content.
- [ ] Build/race/coverage thresholds and all existing tests retain their meaning; no S19, MCP, or Stytch implementation is included.
