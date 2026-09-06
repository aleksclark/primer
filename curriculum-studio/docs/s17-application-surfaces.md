# S17 application surfaces — browser / review handoff

**Not whole-phase acceptance.** The serialization backend was accepted by
b010fc06 at `d98ac903`. This newer unit adds product paths, UI and a test-only host;
it needs independent backend/UI/browser and whole-phase review. No push, PR,
merge, live identity/provider work or production deployment is included. CSC-00
is not waived. P3/original-roadmap reconciliation and Tasks remain untouched.

## Implemented application scope

- Materialized-item comments: POST/GET
  `/studio/v1/materialized-items/{itemId}/comments`. Same comment abstraction and
  Unicode body bound, canonical `mit_` target, server-derived subject/name,
  actual item→run→revision→workspace lookup, FK and database ownership trigger.
  Shared readers cannot read private comments or post them.
- Closed policy GET/PUT
  `/studio/v1/workspaces/{workspaceId}/collaboration-policy`:
  - `requireApprovalForPublish: false` by default;
  - `sharingEnabled: true` by default;
  - active local owner/admin may configure; unknown/non-boolean knobs rejected;
  - disabling sharing **revokes all outgoing grants** and blocks new ones;
    re-enabling does not restore them. Authors can still revoke.
- Policy→curriculum/revision→membership locking protects configuration and
  publication. Publication enforces a current approved fingerprint and current
  human reviewer authority when required. The HTTP authoring path also locks and
  checks the publishing actor and validates the graph in the publication
  transaction. Existing trusted repo publication paths also honor the policy.
  No provider/Identity role dependency. Accepted approval/copy locks are retained.
- Bounded outgoing-share list with workspace names; bounded SQL revision pages
  with revision titles. A graph GET returns a fingerprint while holding its
  revision read lock; the reviewer UI only enables decisions if the displayed
  graph fingerprint matches the current review fingerprint. Stale POSTs fail
  closed, with explicit reload instructions.
- Migration **00014** only; accepted 00013 and older migration bytes unchanged.

## UI organization and boundaries

`web/src/collaboration/` separates revision selection/diff, review,
CommentInspector, library/sharing, materialized items and workspace controls.
`App.tsx` keeps the existing shell/project designer and composes the new panels.
It now surfaces validation/publication failures and asks before publication.

The primary surface is **Command/Inspect**, with **Configure** policy/template
forms. Existing Studio cyan (`#3DE0F0` in app.css), Archivo/Newsreader/IBM Plex Mono,
dark/light tokens and shell are preserved; no SPA restyle. New CSS is scoped in
`web/src/styles/collaboration.css`. Native dialog provides modal keyboard focus;
close restores the opener, Escape closes except while posting. Mobile CSS stacks
comparison columns and uses full-width drawer / 44px controls.

Every new call uses the generated client through the package facade. Core app
models alias generated types; old `as never` transport escapes were removed.
Source DTOs now declare the non-null response arrays the handlers already emit.
Collections send offset/limit and render only the server page. Graph node/edge
selection transforms a single revision aggregate, not a downloaded catalog.
Current-user `/auth/me` membership projections provide the workspace selector;
there is no bulk membership/catalog search. Aborted/stale loads are discarded.
Server guards remain authoritative after membership changes.

## Credential-free fixture (not production BFF)

`internal/testutil/browserfixture/host_test.go` exists **only in a test binary**.
Production `app.Run` / REST / MCP mounting is unchanged. No production test-login
route and no live BFF/Clerk/Authstack acceptance claim.

The fixture starts a separate real PostgreSQL testcontainer and serves the actual
built SPA and real Huma/chi API on an ephemeral **127.0.0.1** port. It refuses an
external `STUDIO_TEST_DATABASE_URL`. It does not touch :9090 or existing browsers.
A narrowly scoped fixture issuer/session edge reuses the Phase-2 ES256 JWT/JWKS
helpers and the actual JWT validator. Opaque fixture cookies are HttpOnly,
SameSite=Strict, host-only, HTTP-loopback-only; mutations require exact Origin.
JWTs and private signing keys stay server-side. Only fixture session lookup uses
memory; comments, grants, policies, library/templates and graph state use PG.
This memory session edge is not a production-ready BFF.

### Start (from repository root)

```sh
unset STUDIO_TEST_DATABASE_URL
(cd curriculum-studio && GOWORK=off make clients-generate)
(cd curriculum-studio/web && npm ci --ignore-scripts --no-audit --no-fund && npm run build)
./curriculum-studio/scripts/browser-fixture.sh > /tmp/s17-browser-host.log 2>&1 &
launcher_pid=$!
grep '^S17_FIXTURE_' /tmp/s17-browser-host.log
```

Wait for the two printed lines (build/container startup takes a few seconds):

```text
S17_FIXTURE_URL=http://127.0.0.1:<ephemeral-port>
S17_FIXTURE_DIR=/tmp/primer-s17-browser.<unique-suffix>
```

Open **`<base-url>/_fixture/`**. Use separate browser contexts/profiles for the two
sessions, not two tabs sharing one cookie jar:

1. Author context: **Ada Author (author)** → Start fixture session.
2. Reviewer context: **Ruth Reviewer (reviewer)** → Start fixture session.
3. Use **Olivia Owner (owner)** in a separate context or rotate a fixture session
   to configure policies. Ada deliberately has no admin or review authority.
4. **Co-op Author (author)** belongs only to W2; **Unshared Author (author)** only
   to W3. Neither receives W1 membership from sharing.

`<base-url>/_fixture/evidence` and `$fixture_dir/fixture.json` contain only public
fixture IDs: curriculum, draft revision, item, run, library, W1/W2/W3 workspace
IDs. The independent fixture DB URL is saved **0600** at
`$fixture_dir/database-url` for local SQL evidence; it is never served to browsers
or printed. Do not paste credentials into evidence or screenshots.

### Stop

```sh
# Use the exact directory printed by this launch:
kill -TERM "$(cat "$fixture_dir/host.pid")"
# Or stop the launcher:
kill -TERM "$launcher_pid"
```

The launcher forwards termination, closes the host/JWKS servers, exits the test
process (testcontainer cleanup), and removes its temporary directory. Restart
for a fresh dataset. The smoke-run URL was `http://127.0.0.1:41353`; that run was
**stopped**. Do not reuse it as a permanent endpoint.

### Fixture data and suggested exploration (not selectors/specs)

- W1 **Workshop Authors**, W2 **Co-op Readers**, W3 **Unshared Workshop**.
- Curriculum **Workshop geometry** has baseline and draft revisions. Diff shows
  **Read a scale** removed and **Draw a scale plan** added.
- Named **Workshop geometry unit**, **Reusable workshop geometry** library entry,
  **Workshop starter** template with **Craftsmanship / First workshop** names.
- One ready run/item: **Scale drawing practice**. This is explicitly seeded
  fixture content, not evidence of provider generation or materialization quality.
- Open the curriculum; choose revisions/baseline. Inspect plan comments and the
  materialized item's comments, post with one persona, reload with the other.
- Reviewer decisions bind to the visible graph snapshot. To exercise stale
  rejection, have the author add a node after the reviewer loads, then try the
  old reviewer decision. Reload plan/review and decide again.
- Create a curriculum from a named template, import the library unit, inspect
  preserved names, save an outcome/unit template. Templates preserve names and
  memberships only; the UI explicitly directs full-unit reuse to the library.
- Owner enables required review. Author publication without a fresh approved
  review fails; only approved current content/current reviewer authority passes.
  Invalid generated/template drafts can also fail graph validation honestly.
- Share read-only to the W2 ID from evidence, confirm, reload W2. W2 can compare
  the graph but cannot comment/review/edit; W3 cannot read it. Revoke or disable
  sharing (confirmation warns it revokes all grants), then reload W2.

No Playwright selectors/specs were authored. The browser leaf must explore the
actual Chrome flow first, then codify it, including keyboard/focus, mobile,
dark/light parity, stale/error states and persisted evidence.

### Evidence lookup

Through each browser's authenticated API requests, use graph/diff/comments,
approval, shares and collaboration-policy endpoints above. For local PG checks:

```sh
psql "$(cat "$fixture_dir/database-url")" -c '
SELECT c.title,r.revision,r.status,a.status AS review,a.content_fingerprint
FROM curriculum_studio.curricula c
JOIN curriculum_studio.plan_revisions r ON r.curriculum_id=c.id
LEFT JOIN curriculum_studio.plan_approvals a ON a.plan_revision_id=r.id
ORDER BY c.title,r.revision;'
psql "$(cat "$fixture_dir/database-url")" -c '
SELECT author_display_name,body,item_id,plan_revision_id
FROM curriculum_studio.plan_comments ORDER BY created_at;'
psql "$(cat "$fixture_dir/database-url")" -c '
SELECT * FROM curriculum_studio.workspace_policies;
SELECT curriculum_id,source_workspace_id,target_workspace_id,permission
FROM curriculum_studio.curriculum_shares;'
```

SQL/API checks supplement, not replace, real browser acceptance.

## Verification and remaining gates

Logs: `/tmp/primer-s17-surfaces/` (`domain.log`, `clients.log`, `full.log`,
`race.log`, `python.log`, `web.log`, `fixture-host.log`, `fixture-smoke.log`).
Final observed command results (cwd `curriculum-studio` unless noted):

- `GOWORK=off make clients-generate` — **exit 0** (`clients.log`).
- `GOWORK=off go test ./... -count=1 -timeout 180s` — **exit 0** (`full.log`),
  including real HTTP fixture session isolation and migration tests.
- `GOWORK=off go test -race ./internal/api ./internal/repo -count=1 -run 'P17|Policy|Comment' -timeout 180s`
  — **exit 0** (`race.log`), including accepted serialization regressions and
  new item/policy/role/default/stale/concurrent-policy cases.
- `python3 -m pytest tests -q` in `db/` — **exit 0** (`python.log`).
- `GOWORK=off go build ./...`, `GOWORK=off go run ./cmd/migrate -check-freeze`,
  `bash tools/contract-gates/check_no_tracked_generated.sh` — **exit 0** (`build.log`).
- `npm run build && npm run lint` in `web/` — **exit 0** (`web.log`). Existing
  CSS `@theme` minifier warning remains; no lint warnings.

New regression names: `TestP17ItemCommentsOwnershipAndDurability`,
`TestP17PolicyHTTPDefaultsRolesAndPublication`, `TestP17ConcurrentPolicyPublication`,
`TestP17PolicyRoleRevocationAndClosedSchema`, and
`TestFixtureSessionsUseRealValidatorAndLocalMemberships`. `TestBrowserFixtureHost`
is deliberately opt-in and skipped during normal suite runs; the explicit
launcher was separately started, HTTP-smoked, and stopped (PASS). Earlier errors
from a wrong test item prefix, omitted required share permission, and generated
nullable array types were repaired; no failing tests or type checks were skipped.

The credential-free host was started and stopped successfully. HTTP composition
smoke verified the real SPA, distinct author/reviewer cookie sessions through the
actual JWT validator and DB memberships, reviewer item-comment POST 201, and a
subsequent author GET of the committed comment. This is **not Chrome/browser or
live external identity evidence**.

Browser screenshots/journeys and whole-phase independent review are pending.
Coverage remains RED against unchanged 85% (last 76.9–77.0% on earlier tips); no
coverage chase or new coverage claim. Existing vet warning remains untouched.
Schema/contract track acceptance and CSC-00/live identity integration remain
separate gates. No full-S17 completion claim.
