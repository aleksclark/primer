# B17 product diagnosis and re-exploration handoff

Parent: `a0ec0cf16ac7cd21e205ca491da97d14dc01b236`. The independent browser
CALL1 is still **FAIL**. Leaf **730fb5b2 must re-explore** before Playwright
promotion. These are product-writer diagnostics, not independent acceptance.
No Playwright suite was authored. The browser worktree/evidence was read only.
Accepted locks, policies, sharing and item-comment behavior are unchanged.

## B17-1: invalid graphs, misleading error, missing mapping control

Reproduced the original form payloads against a fresh real fixture on the parent
source. Both outcomes had portfolio evidence but **no standardCodes**:

- `OUTCOME_UNMAPPED` — outcome `Stale-review mutation outcome` has no standards mapping
- `OUTCOME_UNMAPPED` — outcome `Policy-gated publication outcome` has no standards mapping

`POST /validate` returned `passed:false` with exactly these two error findings.
Publish returned 409 `revision is not editable` both before and after a current
reviewer approval. Persisted state remained draft + approved. This was **not an
approval/publication authorization failure**: graph validation correctly denied
publication, but `PublishAuthorized` wrapped it as a lifecycle error. CALL1's
first 409 also did not establish the approval gate: the invalid graph dominated.
The test followed the available UI; that form could not create a mapped outcome.

Changes:

- `GraphValidationError` preserves real validator findings separately from
  immutable/lifecycle errors. The API returns 409 with actionable detail and
  blocking finding codes/messages/locations. UI error formatting displays them.
  No rule, severity or standards requirement in `validation.Run` was changed.
- Add outcome requires an explicit choice from **real workspace-visible
  catalogs/standards**, with server pagination/search. No catalog/standard is
  created or guessed. Existing POST/nodes performs the real mapping transaction.
- The valid publication diagnostic revealed a separate stale-cache issue: the
  old revision list could overwrite the successful response's `published` state
  with `draft`. Revision state is now part of that list's resource key. Verified
  persisted `published`, selector `published`, and disabled Publish after success.

### Correct re-exploration setup

Start a **fresh fixture**. In Add node, choose Outcome, enter the test name,
choose the catalog containing `S17.SCALE` (the fixture currently displays
`Framework 7`), then select **S17.SCALE — Draw and interpret scale plans**.
Add node stays disabled without the explicit mapping. Use this for **both** the
stale-review mutation and the policy-gated publication mutation.

Keep all criteria:
1. Old reviewer fingerprint after a mapped edit → 409 conflict.
2. Valid graph, required approval, no current approval → 409 with
   `current reviewer approval required` (not a graph error).
3. Reload plan/review, approve current fingerprint, author confirms publish →
   200, durable published revision and published UI state.
4. Separately, an intentionally invalid approved graph must still be refused,
   now with `Revision validation failed` and `OUTCOME_UNMAPPED` findings.

The old CALL1 draft was invalid; do not reuse it for the successful-publication
branch or drop that branch. Existing invalid/template content is not silently
repaired by this fix; validate it and curate it, or use the fresh mapped setup.

Permanent HTTP regressions in `internal/api/b17_publication_test.go`:
- `TestB17InvalidApprovedGraphReportsActionableFindings`
- `TestB17MappedOutcomeEditRequiresFreshReviewThenPublishes`
- `TestB17W2ValidMutationDeniedWithoutPersistence`

## B17-2: tool coordinate mismatch plus actual mobile layout defects

Reproduced the UID click reporting success without opening the curriculum in a
fresh owner session at **390×844, DPR3, mobile/touch**. Instrumentation established:

- document/layout width **406**, visual viewport width **390**;
- visual viewport offsetTop **35**;
- button rectangle height **19.1875**, center clientY **752.39**;
- the UID click generated pointer/mouse/click events on **FOOTER**, clientY
  **787.39** — the 35px offset — rather than on the button;
- no open dialog/overlay; pointer-events auto; elementFromPoint at the actual
  button center resolved the button;
- clicking the visible coordinates (offset corrected), and native CDP touch at
  those visible coordinates, **already opened the original UI**. Thus the
  handler itself was not broken. This was not proven to be a physical tap bug.

The old single-row create form also visibly clipped outside the mobile viewport,
and the curriculum button failed the 44px mobile target size. Narrow CSS changes
stack/wrap those library controls instead of hiding overflow and size the actual
curriculum button to 44px. No click handler was replaced or synthetic click added.

Final diagnostics, including the same two-row library as CALL1:
- document/layout/visual width **390**, height **844**, DPR3, offsetTop **0**;
- target height **44**;
- UID click, native `Input.dispatchTouchEvent` (pointerType `touch`) and keyboard
  Enter all opened the configured panel;
- mobile baseline diff showed added/removed names; comments drawer opened,
  Escape closed it and restored focus.

The shared DevTools MCP browser profile was busy. It was not stopped/reused.
Diagnostics used a separate installed Chrome DevTools **CLI 1.7.0** daemon with
session ID `e13df83b-fefa-4d0a-bb17-1ec2d898125a` and owned profile under
`/tmp/primer-s17-b17/`. Native touch used CDP only on that owned browser's ephemeral
port (not 9222). No agent/model/runtime configuration changed. The independent
leaf should use its own normal Chrome session to re-verify, not copy coordinates.

Four actual missing id/name controls were found (policy checkboxes, curriculum
search, export selector). IDs/names were added. Final DOM probe found zero, and
no form-id console warning recurred. The only final console error was the
intentional 409 response in the invalid-graph denial diagnostic.

## Conclusive W2 request shape

Use a source **draft** ID, W2's authenticated fixture session, and JSON content
encoding. This exact shape was first accepted as an author control (201), then
refused as W2 (404, not 422), with the graph/fingerprint unchanged:

```http
POST /studio/v1/revisions/<source-draft-id>/nodes
Content-Type: application/json

{"kind":"unit","title":"W2 authorization probe","position":0}
```

The fixture browser sends its cookie automatically for same-origin requests;
never expose or invent a bearer token. A raw browser fetch needs `method: 'POST'`,
`headers: {'Content-Type':'application/json'}`, `credentials:'include'`, and
`body: JSON.stringify(theObjectAbove)`. A 422 remains inconclusive authorization
evidence. No authorization implementation change was needed.

## Logs / artifacts (writer-owned, not the independent evidence tree)

`/tmp/primer-s17-b17/`:
- `original-publish.json`: original codes, before/after approval 409s, draft+approved.
- `original-hit-test.json`, `original-events.json`, `original-mobile.png`,
  `original-native-touch.json`: original geometry/mis-hit and real-touch control.
- `final-mobile-geometry.json`, `final-two-row-geometry.json`,
  `final-native-touch.json`, `final-keyboard.snapshot.txt`,
  `final-mobile-drawer.png`, `final-mobile-diff.snapshot.txt`: mobile diagnostics.
- `author-picker-*.snapshot.txt`, `author-mapped-outcome.snapshot.txt`,
  `final-mapped-validation.json`, `final-published-network.txt`: actual picker,
  persisted S17.SCALE mapping, validation success, confirmed browser publish 200.
- `published-ui-stale-cache.json` / `final-transition-state.json`: status-cache
  failure and corrected published/disabled state.
- `final-invalid-and-w2.json`, `final-validation-error.snapshot.txt`: retained
  invalid-approved denial, actionable UI finding, valid W2 probe and no mutation.
- `codegen.log`, `focused.log`, `full.log`, `race.log`, `web-final.log`,
  `checks.log`: verification outputs. All final commands below exited **0**:

```sh
# cwd curriculum-studio
GOWORK=off make clients-generate
GOWORK=off go test ./internal/api ./internal/repo -count=1 -run 'B17|P17|Policy' -timeout 120s
GOWORK=off go test ./... -count=1 -timeout 180s
GOWORK=off go test -race ./internal/api ./internal/repo -count=1 -run 'B17|P17|Policy|Comment' -timeout 180s
GOWORK=off go build ./...
GOWORK=off go run ./cmd/migrate -check-freeze
bash tools/contract-gates/check_no_tracked_generated.sh
(cd web && npm run build && npm run lint)
```

Existing @theme minifier warning remains. No migrations changed, no coverage
chase/threshold change, no whole-phase completion claim.

## Fresh fixture handoff

Use the unchanged Studio-owned launcher from the new tip, as documented in
[s17-application-surfaces.md](s17-application-surfaces.md). Diagnostic databases
were intentionally mutated and are not the fresh acceptance dataset.

```sh
unset STUDIO_TEST_DATABASE_URL
./curriculum-studio/scripts/browser-fixture.sh > /tmp/s17-browser-rerun.log 2>&1 &
grep '^S17_FIXTURE_' /tmp/s17-browser-rerun.log
```

Open the printed URL's `/_fixture/` in independent author/reviewer/owner/W2/W3
contexts. IDs come from `/_fixture/evidence`. Stop only that host using the
printed fixture directory's `host.pid`. If a fresh writer-provided host is
running, its URL/directory are in `/tmp/primer-s17-b17/handoff-host.log`; the
parent handoff states whether it is still running. The mutated diagnostic hosts
at 37969 and 33749 are stopped before handoff.

Browser acceptance remains **FAIL pending re-exploration by leaf730fb5b2** and
whole-phase review. Keep publication success, stale-review, mobile and W2
persistence criteria. No Playwright promotion or live identity/CSC-00 claim.
