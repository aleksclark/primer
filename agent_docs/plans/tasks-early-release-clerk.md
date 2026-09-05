# Tasks-only early release amendment

Baseline: approved P1/P2 master **e173e6cc777ffc0829f75217041f2ce651999f29**.
The user authorizes a **Tasks-only Phase 13 carve-out**, pulling forward only
Phase 7 production packaging, migrations/bootstrap, health, protected routing,
and public smoke. Old exhaustive/full-migration gates are not prerequisites.
P3–P6, remaining Phase 7/13, Android, TV, AI/media, Identity retirement, Ultracore
and Studio stay open. Later donor merges must preserve this release's IDs,
student credentials, mount and authorization tests, not restore old parent
Identity wiring. Protected checks/merge remain L1-coordinated; no direct deploy.

## Frozen contract

- One independent Tasks service/database at **https://api.primerlms.com/tasks/**.
  No optional app hostname or separate UI service. Ingress preserves Host/path;
  app returns 308 `/tasks` → `/tasks/` and strips `/tasks/api` **once**. Native
  `/students`, `/tasks`, `/schedules`, `/student/*`, `/device/*` stay unchanged.
  LMS `/api/v1` and catch-all stay separate/lower priority. No P2 websocket.
- Runtime names: `TASKS_ENV=production`, `TASKS_AUTH_MODE=clerk`,
  `TASKS_DATABASE_URL`, `TASKS_HOST`, `TASKS_PORT` (8080),
  `TASKS_BASE_PATH=/tasks`, `TASKS_WEB_DIR` (image `/app/web`),
  `TASKS_PUBLIC_ORIGIN` (exact origin above, no path), `TASKS_CLERK_ISSUER`,
  `TASKS_CLERK_JWKS_URL` (explicit URLs), optional `TASKS_CLERK_AUDIENCE` **only
  when the real session profile carries that audience**. No Clerk secret key,
  parent shared key, Identity password/client secret or M2M acquisition API.
- Build names: `VITE_TASKS_BASE_PATH=/tasks/`, `VITE_TASKS_API_BASE=/tasks/api`,
  `VITE_CLERK_PUBLISHABLE_KEY`. Missing release key/base fails the build.
  Vite output: `primer-tasks/web/dist`. Router/assets and Clerk modal sign-in /
  sign-out return use `/tasks/parent/students`; no production OAuth callback.
- Build `./cmd/tasks-server`, `./cmd/tasks-migrate`, `./cmd/tasks-bootstrap` from
  `primer-tasks`. Go **1.26.6** is required by published Authstack
  **af1841573db40fda84a29d33abe8f7507a11e068** (the only production verifier).
  Its vanity metadata points at split DNS: `go.mod` uses the public VCS-qualified
  `git.clark.team/aleksclark/authstack.git` transport at that exact commit, not
  a fork/vendor/local verifier. Fresh public proxy/checksum downloads work
  without private DNS, Git credentials, or `GOPRIVATE`.
- `GET /health` is unauthenticated process health (`/tasks/api/health` publicly).
  Production startup neither migrates nor seeds. Before startup run
  `/app/tasks-migrate` (no args; `TASKS_DATABASE_URL` only). Legacy test/dev
  authorization-code compatibility is never allowed in production.

## Stable identity and revocation

Clerk SDK supplies short-lived session JWTs in memory to the owned generated
client, **only on parent requests**. Student pages do not initialize Clerk.
Exact issuer/`azp` and nonempty verified `sid` are required; service/M2M tokens
and old `tasks_parent` cookies are rejected. Clerk org/email/roles grant nothing.
Canonical `(issuer, subject)` maps explicitly through `parent_identities` to
existing `(tenant_id, subject_ref)` local admin membership. No auto-enrollment.
Every parent request rechecks local membership, identity and session revocation;
existing tenant-scoped queries retain local household/actor IDs.

Operator-only bootstrap after backup/migration, using `TASKS_DATABASE_URL`:

```sh
/app/tasks-bootstrap --issuer '<approved exact Clerk issuer>' \
  --subject '<approved Clerk user subject>' --tenant-id '<local household UUID>' \
  --actor-ref '<local parent subject_ref>'
# Initial household only: additionally --create-household --household-name '<name>'
```

Same-link reruns are safe; rebind/reactivation is refused. Never infer IDs from
email/another product DB. Immediate local removal: set `parent_memberships.revoked_at`
or `parent_identities.revoked_at` and audit the operator action. Logout persists
`(issuer,sid)` in `parent_session_revocations` before Clerk logout. **Provider-only
revocation is not instant**: offline JWT verification is bounded by token expiry.
Qualify real `exp`/`nbf`/`azp`/`sid`/optional `aud`; the published verifier checks
provided lifetime claims, and Tasks adds no second parser. JWKS refresh: 5-minute
normal interval, failed-verification/unknown-key retry with 5-second cooldown;
known keys survive outage at most one hour, startup requires reachable JWKS.

Student opaque IDs, hashes and revocation are untouched. Cookie remains
`tasks_student`, host-only, `Path=/`, HttpOnly, Secure in production, SameSite=Lax,
90-day lifetime. Only QR API mount metadata changes to `/tasks/api`.

## Evidence and remaining gate

Passed: full `go test ./...` with real PostgreSQL; `go vet ./...`; isolated
`GOWORK=off` binary builds; existing `make cover` **85.5% ≥ 85%**; generated clients;
web lint/typecheck/default build; release build rejects absent approved key.
Focused tests cover wrong/missing tokens, issuer/party/audience/lifetime/kind,
canonical bootstrap/rebind denial, membership/identity/sid revocation, database
outage, cross-household actions, manual schedule → paired browser submission →
parent approval, unchanged student/device revocation, key rotation and mount.
These signed local test JWTs are **not public Clerk proof**.

`python3 primer-tasks/scripts/release-smoke.py` checks public mount/health/401s /
redirects without credentials. Required real browser smoke remains:
1. Actual Clerk LOGIN at the public URL; operator-approved local membership.
2. Existing web: create/publish manual approval task, schedule student, issue pairing.
3. Separate student browser: pair, start, submit; parent approves; both see completed.
4. Archive test student and verify old pairing denied; sign out and deny old parent sid.
Record outcomes without credentials. L1 owns final public browser verification.

At code freeze, C found coherent **test-instance** Clerk metadata/JWKS (RS256),
but configured party list lacked the public origin. Owner-approved profile reuse,
origin qualification, secure publishable-key delivery and explicit subject/local
bootstrap mapping remain prerequisites. No invented key, fixture-as-public-proof,
production deployment, or completed public-auth acceptance is claimed here.
