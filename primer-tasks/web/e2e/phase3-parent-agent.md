# P3 parent-agent browser qualification

This extends the existing Tasks Playwright stack (`playwright.config.ts`,
`browser:promoted`, `make tasks-e2e`), not a second runner. The scenarios were
promoted from independent full Chrome DevTools exploration of product source
`e0c0a8cab9b217a65fa310e14afad128184ebe98`. Earlier aa7 failures remain separate
workspace evidence; they are not passing receipts for current source.

## Run only against an owned disposable host fixture

From the repository root, use a unique name and this worktree's default host
state location. Host Make is the default, not Compose:

```sh
export TASKS_HOST_NAME=primer-p3-browser-your-unique-run
export TASKS_HOST_STATE_DIR="$PWD/primer-tasks/tmp/host-stack"
export CHROME_EXECUTABLE=/opt/google/chrome/chrome
export PRIMER_TASKS_E2E_NO_TRACE=1
export PRIMER_TASKS_EXPLORATORY_BROWSER_PASS=1 # only with actual exploration PASS
export TASKS_MODEL_PROVIDER=scripted TASKS_AGENT_MODE=scripted
export TASKS_AGENT_SCRIPTED_DELAY_MS=1500

# Focused gate: real PostgreSQL/API/issuer/Vite ready BEFORE browser startup.
make tasks-host-up
set -a; . "$TASKS_HOST_STATE_DIR/endpoints.env"; set +a
(cd primer-tasks/web && npm run browser:promoted -- e2e/phase3-parent-agent.spec.ts)
make tasks-host-down

# Existing full regression gate; starts/stops the same named fixture itself.
# Do not run concurrently with any manual browser or another fixture writer.
make tasks-e2e
```

Always perform owned cleanup after a failing focused command too. `up` resets
only its named disposable PostgreSQL fixtures; do not point these commands at a
household, donor, completed-release database, production, or live Clerk/model.
No port9090/9222 attachment or fleet operation is part of this lane.

The fixture uses the supported **development BFF + local OIDC issuer**, real
Fantasy with explicitly scripted inference, actual public WebSockets/domain
handlers and real PostgreSQL. It is not live Clerk/provider qualification.
`CHROME_EXECUTABLE` must be full Chrome, **not headless-shell**. The host's
normal workspace-selected transport is recorded separately from the direct
Tasks module pin (e0 direct coder/websocket1.8.14; normal workspace/running
binary1.8.15). Do not pretend a Node WHATWG socket proves native Chrome closure.

## Isolation and safety

Each scenario creates fresh A/B cookie jars and a new opaque conversation. The
scripted provider's disable/retire selection assumes one active task/schedule.
Before each scenario the helper therefore **normalizes only this verified owned
fixture's parent A active rows using the ordinary public parent APIs** (disable
schedules, retire templates). It does not delete SQL, mutate donor data, fake a
model result, or bypass confirmation on the feature under test. Archived history
and issued work remain. This makes focused, full-regression and repeated runs
independent of whether phase1/phase2 or another P3 scenario ran first. Actual
assertions use canonical collection baselines and unique UI-created names.

The only SQL access is an allowlisted single **SELECT** for a running-job barrier
and its durable terminal readback. No direct database setup/effect writes exist.
The barrier is armed before the UI command, catches the uniquely named actual
`running` row, and immediately invokes the existing owned `host-stack crash-api`
control. Merely seeing a spinner then killing an already-committed preview is not
counted as running-job restart proof.

All P3 traces are unconditionally disabled. `PRIMER_TASKS_E2E_NO_TRACE=1` also
turns off trace capture for the full existing suite without changing its
behavioral assertions. No cookies, input-only credential subprotocols, raw input
frame dumps, process log buffers, or `storageState` are attached. The native
observer passes constructor/send traffic unchanged, records safe output fields
and cursor metadata only, and keeps confirmation handles ephemeral. Public
native probes do not mock or filter application frames. Screenshots and safe
JSON attachments stay in the existing ignored `test-results/` and
`test-artifacts/` locations; do not commit bulky evidence.

Console/page/HTTP/request-failure checks remain fail-closed. Only exact named
windows around **owned API restart**, **missing CSRF**, and **opaque Origin**
accept their corresponding handshake diagnostics. Backpressure1006 and
`Close received after close` are never allowlisted. Normalization fetches drain
their bodies before navigation; request aborts are not globally ignored.

The inherited240s scenario budget covers genuine multi-step work and the30s
PostgreSQL lease takeover. Normal assertions remain10s or less; stale CAS must
terminalize within5s, not at the120s production deadline. Running-crash terminal
wait45s is explicitly lease-derived. No retries, skipped tests, timeout-as-proof,
fake provider frames or arbitrary selector `.first()` are used. Foreground/paint
readiness is required before native Chrome screenshots; screenshot timeout is
bounded10s, not increased to conceal a stalled background compositor.

## Browser scenario inventory

1. **Canonical domain receipt / handles / current CAS**
   - Ordinary UI student; cancel first preview with unchanged public counts and
     no success receipt; fresh same intent.
   - Real foreign B handle/subscribe/cancel rejection with an ordered own-scope
     sentinel; altered active handle; canceled/spent duplicate confirmations.
   - Ordinary rename after preview; committed `source=domain` text_start,
     semantic clauses and authoritative text_end name actual current records.
     UI calls it **Confirmed result**, not a synthetic model answer. First
     receipt-frame public reads see committed effects. One canonical task and
     schedule; ordinary pages lead with title/student names.
   - Explicit duplicate acknowledgements, reload/opaque conversation, one final
     receipt and no duplicated effects.
   - Ordinary schedule save and task draft revision cause immediate durable
     `confirmation_stale`/failed, actionable copy, no effect/receipt and no live
     stale Confirm control. Fresh proposals succeed; late old errors cannot
     retarget them.
   - 1440desktop/390responsive-mobile dark/light error wrapping, no overlap or
     overflow, and safe screenshots.
2. **Native Chrome transport / worker lifetime**
   - History generated through real public agent commands, not injected events.
   - BOTH Vite and direct API: exactly64 unacknowledged frames, native1013 and
     safe reason, healthy reader and new A/B commands continue.
   - Concurrent/idempotent3 sends/2 keys/2 runs; disconnect while thinking;
     cursor subscribe, ordered replay, one terminal and no duplicated effect.
   - Committed pending preview stays inert across real API restart until fresh
     same-session authenticated confirmation.
   - Condition-observed RUNNING crash yields durable exact reauthorization
     terminal, not fake completion/resumption; public/UI/SELECT/replay agree.
3. **Scope / bounded failure / manual boundary**
   - Two UI-created Alex students clarify without guessing; foreign-scope
     prompt cannot expose or mutate the other household.
   - Active UI cancel, mobile keyboard solid focus and Enter submission.
   - Actual native missing-CSRF and opaque-origin403 denials. Sandbox opaque
     origin may also suppress SameSite cookies; this is not a production
     signed-Clerk Origin-only qualification, nor is dev loopback wildcard
     acceptance misrepresented as a production negative.
   - Restart with restricted tool allowlist fails safely; disabled provider
     shows plain unavailable state, no effect/receipt, disabled composer, and
     an independent ordinary manual draft still works.

## Separate source-test obligations (not inferred from browser green)

These exact existing tests are complementary. Do not rerun the long race/full
source gates just because the browser suite ran, and do not label them live
provider/Clerk smoke:

| Obligation | Existing source test(s), relative to `primer-tasks/` |
|---|---|
| Same-household different actor; atomic admission | `internal/api/agent_authority_ws_test.go`: `TestPublicAgentAtomicAdmissionAndActorIsolation` |
| Signed Clerk verifier/credential transport | `internal/api/agent_clerk_ws_test.go`: `TestClerkAgentUpgradeUsesAuthstackWithoutCredentialEcho` |
| Queued authority expiry / lost ephemeral credentials | same file: `TestClerkAgentQueuedAuthorityExpiresAndRestartFailsClosed`, `TestLegacyAgentQueuedCredentialLossAlsoFailsClosed` |
| Fresh pending confirmation authority / local revocation | same file: `TestClerkAgentPendingPreviewRequiresFreshCurrentAuthority`, `TestClerkAgentLocalRevocationClosesReplayAndRejectsEffects` |
| Semantic revocation-vs-private-write race and bounded locks | `internal/api/agent_delivery_revocation_test.go`: `TestPublicAgentRevocationBeforePrivateWriteUsesFreshPostWaitState`, `TestPublicAgentPrivateWriteBeforeRevocationHoldsLocksUntilFrameReturns`, `TestPublicAgentStalledPrivateWriteBoundsRevocationLockLifetime` |
| Raw RFC exactly one Close then EOF (Node is insufficient) | `internal/api/agent_native_close_test.go`: `TestPublicAgentBackpressureSendsExactlyOneCloseFrame`, `TestPublicAgentBackpressureNativeCloseHandshake` |
| Subscriber/worker cleanup | `internal/api/agent_backpressure_ws_test.go`: `TestPublicAgentSlowSubscriberCloses1013WithoutBlockingHealthyTenant`; `internal/jobs/worker_test.go`: `TestWorkerLeaseLossDoesNotPublishCompetingTerminal`, `TestWorkerRunStopsOnCancellation` |
| Atomic receipt/effect rollback and replay | `internal/api/agent_receipt_ws_test.go`: `TestPublicConfirmationStreamsCommittedCanonicalReceiptAndReplaysOnce`, `TestPublicFailedOrCanceledConfirmationNeverCommitsSuccessReceipt`, `TestPublicStaleScheduleRetiresOnlyObsoletePreviewAndFreshProposalSucceeds`; `internal/api/agent_mutations_integration_test.go`: `TestPublicConfirmationFailureRollsBackEveryEffectThenRetries` |
| Malformed/disallowed actions and unsafe event variants | `internal/api/agent_rejection_integration_test.go`: `TestAgentProposalValidationCannotPersistInvalidOrDisallowedActions` |
| Finite step/token/deadline validation; emitter failure | `internal/agent/runtime_more_test.go`: `TestRunLimitsTransitionsAndPreviewValidation`, `TestRuntimeRejectsEmitterFailureAndInvalidLimits`; `internal/agent/model_test.go`: `TestRunStateTransitionsAndFiniteLimits` |
| Expired/unowned work cannot invoke provider/effects | `internal/api/agent_mutations_integration_test.go`: `TestAgentExpiredAndUnownedWorkFailsWithoutProviderOrEffect` |
| Typed Fantasy loop, safe callbacks, cancellation/provider failure | `internal/agent/fantasy_qualification_test.go`: `TestFantasyQualificationUsesPublicTypedToolAndSafeCallbacks`, `TestFantasyQualificationContextCancelAndProviderFailureShape` |
| Public origin/CSRF/wire redaction and released student boundary | `internal/api/agent_ws_more_test.go`: `TestAgentOriginCSRFAndWireRedaction`; `internal/api/clerk_integration_test.go`: `TestClerkReleaseParentAndUnchangedStudentBoundary` |

The browser does not surgically pause mid-token, inject a final-event transaction
failure, claim live provider timeout quality, or count goroutines from a close
callback. The named source tests cover their stated validation/lifetime cases;
a browser PASS alone does not expand those claims. Original phase05 acceptance
also requires the separately bound source review/full85%/race10/CI gates and the
required independent Chrome re-drive **after** codification. Never substitute a
prior exploration receipt for that same-invocation promotion gate.
