# Phase 2: Authz boundary

**File:** `phase-02-authz-boundary.md`
**Depends on:** Phase 1
**Duration guess:** 4–6 days
**Handoff wave:** see index orchestrator map

## Goal

Establish authentication validation and workspace authorization at the HTTP edge: local JWKS JWT verification for `aud=curriculum-studio`, credential-free test auth mode that mints the **same** host-only session cookie and Bearer shape production will use, membership-based RBAC, CSRF on cookie mutations, and hard fail-closed production guards. This unblocks authenticated E2E for all later phases without implementing Primer Identity.

## Scope

### In scope

- `internal/auth` JWT validate (iss, aud, exp, nbf, kid via JWKS cache)
- `STUDIO_AUTH_MODE=test|jwks` with production refuse of test mode when `STUDIO_ENV=production`
- Loopback JWKS endpoint or static test JWKS for tests
- BFF routes under `/studio/v1/bff/*` or `/bff/*` on same host: test session mint, logout, csrf token
- Host-only session cookie (`Secure; HttpOnly; SameSite=Lax`; `__Host-studio-session` when HTTPS path allows)
- Middleware: resolve subject_ref (`identity:<uuid>` / `identity:svc:<id>`), load memberships, enforce roles
- Deny cross-workspace access by default
- Service scope checks placeholder map (e.g. `materialize:write`) for machine callers
- Negative tests: wrong aud, expired, unknown kid, missing membership, CSRF absence

### Out of scope / YAGNI

- Google OIDC / Identity OP implementation (identity track + Phase 18 live)
- Full BFF OAuth code exchange with real Identity (wired in Phase 9 against test mode; live Phase 18)
- SPA UI (Phase 9)

## BDD Success Criteria

#### Scenario: P2-S1 — Valid JWT accepted

- **Given** JWKS available and a signed JWT with aud=curriculum-studio, valid exp, known kid
- **When** client calls an authenticated probe route with Authorization Bearer
- **Then** request authorized
- **And** subject_ref derived as identity:<sub>

#### Scenario: P2-S2 — Invalid JWT rejected

- **Given** server running
- **When** client presents expired, wrong-aud, bad-sig, or unknown-kid token
- **Then** 401 responses
- **And** no workspace data leaked in body

#### Scenario: P2-S3 — Test session mint

- **Given** `STUDIO_AUTH_MODE=test` and non-production env
- **When** test client POSTs `/bff/test/session` with subject and workspace seed hooks allowed in test
- **Then** Set-Cookie host-only session is returned
- **And** subsequent API calls with cookie succeed via BFF attach or cookie session bridge
- **And** session subject matches requested identity UUID

#### Scenario: P2-S4 — Production rejects test auth mode

- **Given** `STUDIO_ENV=production` and `STUDIO_AUTH_MODE=test`
- **When** process starts
- **Then** process fails closed (non-zero exit) or refuses to serve
- **And** log explicitly cites forbidden test auth

#### Scenario: P2-S5 — Role allows author mutation

- **Given** membership role=author on workspace W for subject S
- **When** S creates a protected resource in W (probe or workspace update)
- **Then** 2xx success
- **And** audit_events row records subject_ref

#### Scenario: P2-S6 — Viewer cannot mutate

- **Given** membership role=viewer on workspace W
- **When** subject attempts POST/PATCH mutation
- **Then** 403 Forbidden
- **And** no row change

#### Scenario: P2-S7 — Cross-workspace isolation

- **Given** subject member of W1 only; resource exists in W2
- **When** subject GETs W2 resource by id
- **Then** 404 or 403 (no existence leak preferred 404)
- **And** no row payload from W2

#### Scenario: P2-S8 — Service principal scopes

- **Given** JWT for identity:svc:primer-lms with scope materialize:write
- **When** service hits machine probe requiring that scope
- **Then** allowed
- **And** token without scope denied 403

#### Scenario: P2-S9 — CSRF required on cookie mutation

- **Given** browser-like client with session cookie but no CSRF header/token
- **When** POST state-changing BFF or cookie-authed route
- **Then** request rejected 403
- **And** same request with valid CSRF succeeds

## Implementation Instructions

1. Implement JWKS cache with kid rotation window (dual-key accept).
2. Define `AuthContext` on request context: SubjectRef, Kind (human/service), Scopes, SessionID.
3. Test mode: ed25519/RSA keypair generated in process; JWKS exposed at `/studio/v1/.well-known/jwks.json` only in test mode OR injected via `STUDIO_JWKS_URL` pointing at test server.
4. Session store: server-side session table **or** encrypted cookie session binding subject; prefer server-side table `bff_sessions` only if additive migration filed through db track — otherwise signed cookie session v1 with short TTL documented as interim **with** same cookie name production will keep.
5. If new tables needed, open blocker to db track; until then signed cookie is acceptable if documented in phase PR.
6. Seed helper in testutil: `SeedMembership(workspace, subject, role)`.
7. Probe routes: `GET /studio/v1/auth/me` returns subject + memberships (human-readable workspace names when joined).
8. CSRF: double-submit cookie or header `X-CSRF-Token` matching session.
9. Document replacement path: Phase 9/18 swaps test mint for Identity OAuth while keeping cookie name + middleware.

## End-to-End Test Plan

#### P2-E1 — JWT happy path

- **Setup:** test JWKS + membership seed + real HTTP server
- **Action:** Bearer call /auth/me
- **Assert:**
  - 200
  - subject_ref prefix identity:
  - workspace list names present when membership exists
- **Command:** `make studio-test`

#### P2-E2 — Test session cookie E2E

- **Setup:** AUTH_MODE=test
- **Action:** mint session then call /auth/me with cookie
- **Assert:**
  - 200
  - Set-Cookie without Domain=
  - HttpOnly present
- **Command:** `go test ./internal/bff -run TestSession`

#### P2-E3 — Prod guard

- **Setup:** env production+test mode
- **Action:** start
- **Assert:**
  - fails closed
- **Command:** `go test ./internal/config -run RejectTestAuth`

#### P2-E4 — RBAC matrix

- **Setup:** roles owner/admin/author/reviewer/viewer
- **Action:** each attempts mutation probe
- **Assert:**
  - matrix matches SCHEMA roles
  - viewer/reviewer denied writes as designed
- **Command:** `go test ./internal/authz -run RoleMatrix`

#### P2-E5 — Tenant isolation

- **Setup:** two workspaces two users
- **Action:** cross GET/PATCH
- **Assert:**
  - denied
  - DB unchanged for foreign workspace
- **Command:** `go test ./internal/testutil/e2e -run Isolation`

#### P2-E6 — Service scope

- **Setup:** service JWT with/without scope
- **Action:** machine probe
- **Assert:**
  - allow/deny
- **Command:** `go test ./internal/auth -run ServiceScope`

#### P2-E7 — CSRF

- **Setup:** cookie session
- **Action:** POST without and with CSRF
- **Assert:**
  - 403 then 2xx
- **Command:** `go test ./internal/bff -run CSRF`

## Anti-Cheating Audit

- No middleware that trusts `X-User-Id` header without signature
- Test mode cannot compile-enable in production builds without env guard
- Authz not only in SPA
- JWKS fetch failures fail closed (not allow-all)
- CSRF not disabled via broad `if test` on all envs
- Membership checks hit Postgres, not hard-coded map in handler forever without DB

## Completion Gate

- [ ] P2-S* green
- [ ] P2-E* green on real HTTP+Postgres
- [ ] Production test-mode rejection proven
- [ ] Documented cookie/JWT shape for Phase 9 reuse
- [ ] Anti-cheat clean


## Dependencies

- Upstream: Phase 1
- Sibling tracks: schema (`curriculum-studio/db`), contracts (`curriculum-studio/contracts`), identity (when leaving test auth)
- Downstream consumers: later phases listed in index

## Rollback

- Revert the phase PR(s); drop any additive migrations only via forward-fix sibling db track (never destructive rollback in prod without backup).
- Feature flags: prefer `STUDIO_*` env gates over half-applied routes.
- SPA: revert `curriculum-studio/web` routes; keep generated client in sync with OpenAPI.
