# Phase 2: Authz boundary

**File:** `phase-02-authz-boundary.md`
**Depends on:** Phase 1
**Duration guess:** 4–6 days
**Handoff wave:** see index orchestrator map

## Goal

Establish authentication **validation** and workspace authorization at the HTTP edge: local JWKS JWT verification for `aud=curriculum-studio`, credential-free auth that consumes a **protocol-compatible loopback/test Identity** (preferred) or a narrowly test-only verifier using the **same** principal/JWT middleware path production uses, membership-based RBAC, CSRF on cookie mutations, and hard fail-closed production guards. Studio **never** becomes an auth/session issuer (no password stores, no OP, no access/refresh token mint for clients). This unblocks authenticated E2E; production JWKS/BFF login still blocks on Identity milestones (see delivery roadmap).

## Scope

### In scope

- `internal/auth` JWT validate (iss, aud, exp, nbf, kid via JWKS cache, plus required signed public `client_id`; reject `azp` and internal OAuth-client UUIDs)
- `STUDIO_AUTH_MODE=jwks|test` with production refuse of `test` and of test/loopback Identity providers when `STUDIO_ENV=production`
- Preferred credential-free path: real loopback/test Identity process (or in-harness OP) issuing single-aud JWTs; Studio validates via JWKS only
- Narrow test-only verifier alternative: inject JWKS URL + pre-minted JWTs from test helper **outside** Studio process; Studio middleware identical to production
- BFF routes under `/studio/v1/bff/*` or `/bff/*` on same host: session **attach**/logout/csrf for cookie bridge — may call test Identity token endpoint; must not mint raw access tokens inside Studio domain code
- Host-only session cookie (`Secure; HttpOnly; SameSite=Lax`; `__Host-studio-session` when HTTPS path allows) holds BFF session only, never long-lived access/refresh for SPA
- Middleware: resolve subject_ref (`identity:<uuid>` / `identity:svc:<id>`), load memberships, enforce roles
- Deny cross-workspace access by default
- Service scope checks placeholder map (e.g. `materialize:write`) for machine callers
- Negative tests: wrong aud, expired, unknown kid, missing membership, CSRF absence

### Out of scope / YAGNI

- Stytch B2B broker / Primer Identity token broker implementation (identity track + Phase 18 live)
- Full BFF OAuth code exchange with real Identity (wired in Phase 9 against test mode; live Phase 18)
- SPA UI (Phase 9)

## BDD Success Criteria

#### Scenario: P2-S1 — Valid JWT accepted

- **Given** JWKS available and a signed JWT with aud=curriculum-studio, valid exp, known kid, and registered public `client_id`
- **When** client calls an authenticated probe route with Authorization Bearer
- **Then** request authorized
- **And** subject_ref derived as identity:<sub>

#### Scenario: P2-S2 — Invalid JWT rejected

- **Given** server running
- **When** client presents expired, wrong-aud, bad-sig, unknown-kid, missing/wrong/overlong/control-bearing `client_id`, internal UUID as `client_id`, or any `azp` token
- **Then** 401 responses
- **And** no workspace data leaked in body

#### Scenario: P2-S3 — Credential-free session via test Identity or narrow verifier

- **Given** non-production env and either (a) loopback/test Identity issuing JWT `aud=curriculum-studio`, or (b) `STUDIO_AUTH_MODE=test` with external test JWKS + pre-minted JWT
- **When** test client completes BFF session attach (`/bff/test/session` or OIDC test login against loopback Identity) with subject and workspace seed hooks
- **Then** Set-Cookie host-only session is returned
- **And** subsequent API calls succeed through the **same** JWT/principal middleware as production
- **And** session subject matches requested identity UUID
- **And** Studio process logs show validate/JWKS path — not a Studio-local token mint API for clients

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

1. Implement JWKS cache with kid rotation window (dual-key accept). Studio is a **validator only**.
2. Define `AuthContext` on request context: SubjectRef, Kind (human/service), Scopes, SessionID, and validated public ClientID. Never map `azp` or an internal Identity OAuth-client UUID into ClientID.
3. Credential-free: prefer harness that starts loopback Identity (identity plan `internal/oauthtest` / test OP) and points `STUDIO_JWKS_URL` at it. Alternative narrow test mode: test helper mints JWT with external key material; Studio only loads JWKS — **do not** expose Studio as OP or `/oauth/token`.
4. BFF session store: server-side session table **or** encrypted cookie binding subject; prefer server-side `bff_sessions` only if additive migration filed through db track — otherwise signed cookie session v1 with short TTL documented as interim **with** same cookie name production will keep. Cookie is not an access-token substitute for machine callers.
5. If new tables needed, open blocker to db track; until then signed cookie is acceptable if documented in phase PR.
6. Seed helper in testutil: `SeedMembership(workspace, subject, role)`.
7. Probe routes: `GET /studio/v1/auth/me` returns subject + memberships (human-readable workspace names when joined).
8. CSRF: double-submit cookie or header `X-CSRF-Token` matching session.
9. Production validator/cutover: credential-free test Identity/narrow verifier work is independent of production auth. Promote `STUDIO_AUTH_MODE=jwks` only after **I6 / IB2** provides the Primer ES256/JWKS bridge, **I8 / IB4** provides signed webhook/two-plane revocation, and the applicable **I12 / IB8** Studio integration chain is green; set it against the real Identity issuer and keep middleware identical. The live BFF path separately requires **I7 / IB3 + I8 / IB4**. Live Stytch remains BLOCKED until the approved live gate.

## End-to-End Test Plan

#### P2-E1 — JWT happy path

- **Setup:** test JWKS + membership seed + real HTTP server
- **Action:** Bearer call /auth/me
- **Assert:**
  - 200
  - subject_ref prefix identity:
  - workspace list names present when membership exists
  - public client_id present in AuthContext; no azp accepted
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
- Missing/wrong `client_id`, `azp`, or an internal OAuth-client UUID cannot be normalized into a valid AuthContext

## Completion Gate

- [ ] P2-S* green
- [ ] P2-E* green on real HTTP+Postgres
- [ ] Production test-mode rejection proven
- [ ] Documented cookie/JWT shape for Phase 9 reuse
- [ ] Anti-cheat clean


## Dependencies

- Upstream: Phase 1
- Sibling tracks: schema (`curriculum-studio/db`), contracts (`curriculum-studio/contracts`), identity (credential-free test Identity/narrow verifier is separate; production validator/cutover requires I6+I8 and applicable I12; live BFF requires I7+I8)
- Downstream consumers: later phases listed in index

## Rollback

- Revert the phase PR(s); drop any additive migrations only via forward-fix sibling db track (never destructive rollback in prod without backup).
- Feature flags: prefer `STUDIO_*` env gates over half-applied routes.
- SPA: revert `curriculum-studio/web` routes; keep generated client in sync with OpenAPI.


## Stytch-backed identity reconciliation (authoritative)

Stytch B2B is upstream human authentication/session authority. Primer Identity is its sole SDK/API client and downstream Primer token broker: it validates opaque Stytch sessions, maps exact `(project_id, organization_id, member_id)` to a local account, and issues only short-lived single-audience Primer JWTs/JWKS. Studio, LMS, TV, MCP, product APIs, and browser JS never receive, store, forward, log, or validate a Stytch session token, SessionJWT, tuple, or role.

Stytch organization/member roles are eligibility hints only. Studio workspace membership, LMS educator roles, and TV device authentication remain local systems of record; a Stytch organization does not create a Studio tenant/workspace. Provisioning is explicit invite/admin only, and distinct cross-org tuples stay distinct personas without email merge/linking. IA is library-only and unapproved; production auth waits for IA-R and IB1–IB4, with signed webhook/cache+grant revocation as a hard BFF/MCP gate.

Required E2Es: mapped tuple to local membership; valid no-membership token denied; same-email cross-org isolation; Stytch admin-like role denied without local role; raw Stytch bearer rejected; outage fails closed/no negative cache; signed webhook forgery/replay/dedupe/out-of-order; explicit revocation bound; LMS local-role dual run; and no token/provider payload in audit logs.
