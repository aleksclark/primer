# Phase 13: LMS dual-login and service dual-accept (S3–S5)

**File:** `phase-13-lms-dual-login-and-service-cutover.md`
**Depends on:** Phases 7–9, 11
**Duration guess:** 8–12 days
**Migration stage:** S3, S4, S5
**Handoff wave:** W7

## Goal

Migrate Primer LMS human and service authentication onto Identity **without lockouts**: additive `educators.identity_subject`, dual human login (legacy password session **and** Identity BFF/JWT), explicit Google link step-up, `SharedSecretGuard` dual-accept of service JWT + legacy static with **production fail-closed empty secret**, and SPA cutover from `localStorage` bearer to host-only BFF cookies. Student/device tokens unchanged.

## Scope

### In scope

**LMS DB / API (`server/`):**

- Goose migration additive: `educators.identity_subject TEXT UNIQUE` nullable
- `ParentSessionGuard` evolution:
  1. Prefer Bearer JWT → JWKS verify aud=`primer-lms` → resolve educator by identity_subject
  2. Else legacy opaque parent_session token path
- Keep `POST /auth/login` password path until S7 per account
- New BFF routes on LMS host (or sidecar): `/auth/login`, `/auth/callback`, `/auth/logout` per Phase 7 contract using client `lms-bff`
- Link endpoints: authenticated “Link Google” → Identity link API
- JIT parent provision flag `LMS_ALLOW_JUST_IN_TIME_PARENT` default false for multi-user; single-family may enable
- `SharedSecretGuard` / config:
  - Production: empty `SERVICE_TOKEN` **and** JWT-only mode unset → **refuse start** (flip fail-open)
  - Dual-accept: Bearer JWT (aud+scope) OR legacy static equality
  - Metric `legacy_service_secret_used`
  - Accept `X-Service-Token: <JWT>` as discouraged alias during migration
- Startup warnings → hard errors in production for missing auth config

**SPA (`web/`):**

- Remove production dependency on `localStorage` token (`web/src/api/auth.ts`)
- Same-origin credentialed fetch; CSRF header from companion cookie
- Login button → BFF redirect; logout via BFF
- Dev-only escape hatch behind explicit flag if needed

**Tests:**

- Extend `server/internal/api` auth tests + testcontainers
- Browser Playwright optional in this phase if harness exists; else Go cookie jar E2E minimum

### Out of scope

- S7 password disable globally (Phase 14)
- TV admin SPA (Phase 14)
- Student Identity accounts

## BDD Success Criteria

#### Scenario: P13-S1 — Legacy password login still works (S3)

- **Given** educator with password_hash and null identity_subject
- **When** POST `/auth/login` email/password
- **Then** legacy session token still authenticates parent routes
- **And** no Identity dependency required for this path

#### Scenario: P13-S2 — Identity JWT login works (S3)

- **Given** educator with identity_subject set to Identity sub
- **When** request presents valid JWT aud=primer-lms
- **Then** ParentSessionGuard accepts
- **And** role checks still enforce parent/admin in LMS

#### Scenario: P13-S3 — Link Google populates identity_subject

- **Given** password-authenticated educator
- **When** completes explicit link step-up to Google subject G
- **Then** educators.identity_subject set
- **And** external identity bound only via Identity service not LMS email merge

#### Scenario: P13-S4 — Lockout prevention

- **Given** educator password-only unlinked
- **When** ops attempts to disable password globally for that user
- **Then** system refuses or migration job reports blocker
- **And** password path remains until Google linked or recovery enrolled

#### Scenario: P13-S5 — Explicit link required for email collision

- **Given** password educator email E and separate Identity Google account email E
- **When** Google login alone
- **Then** does not silently attach to educator row
- **And** linking requires authenticated proof path

#### Scenario: P13-S6 — Service JWT accepted (S4)

- **Given** dual-accept enabled; Identity service client grant ingest
- **When** TV/caller sends Authorization Bearer service JWT
- **Then** ingest authorized
- **And** legacy metric not incremented

#### Scenario: P13-S7 — Legacy static still accepted during dual-run

- **Given** dual-accept mode; valid SERVICE_TOKEN
- **When** X-Service-Token static presented
- **Then** authorized
- **And** metric legacy_service_secret_used increments

#### Scenario: P13-S8 — Production empty secret fail-closed

- **Given** ENV=production, empty SERVICE_TOKEN, JWT auth not fully configured
- **When** primer-server starts
- **Then** non-zero exit before listen
- **And** no fail-open SharedSecretGuard inert path

#### Scenario: P13-S9 — SPA uses host-only cookie (S5)

- **Given** browser login via BFF
- **When** inspect storage
- **Then** no primer-parent-token in localStorage
- **And** session cookie host-only HttpOnly

#### Scenario: P13-S10 — SPA CSRF on mutations

- **Given** cookie session
- **When** mutating API without CSRF
- **Then** 403
- **And** with CSRF succeeds

#### Scenario: P13-S11 — Stage flags S3–S5 observable

- **Given** deployment config
- **When** operators read status/metrics
- **Then** dual-login and dual-accept modes visible
- **And** legacy acceptance count queryable

#### Scenario: P13-S12 — Rollback to password-only

- **Given** Identity path disabled by flag
- **When** users login with password
- **Then** still works for unlinked and linked accounts with password enabled
- **And** no mass lockout

## Implementation Instructions

1. Migration only additive; never drop password_hash here.
2. Refactor `server/internal/api/secret.go` SharedSecretGuard to support modes: `legacy`, `dual`, `jwt_only`; production Validate in `server/internal/config`.
3. Add JWKS cache client in LMS (`server/internal/auth/identity` or similar) reusing Phase 3/10 verifier rules.
4. BFF can live in primer-server same process (recommended) under `/auth/*` + cookie, proxying API with injected Bearer — mirrors design BFF pattern without extra binary.
5. Update OpenAPI security schemes carefully; regenerate `make openapi` + `make client`.
6. Rewrite `web/src/api/auth.ts` and parent-token-gate component to session/BFF.
7. Keep device/student guards untouched; assert tests still pass.
8. Migration job report: SQL/admin list password-only / linked / locked.
9. Commands: `make test`, `make cover`, focused auth packages with `-race`.

## End-to-End Test Plan

| ID | Setup | Action | Assert | Command |
| --- | --- | --- | --- | --- |
| P13-E1 | testcontainer LMS + Identity harness | password login + JWT login | both access parent route | `go test ./internal/api -run DualLogin -race` |
| P13-E2 | password user link flow | link Google loopback | identity_subject set; lockout rules | `go test ./internal/api -run LinkGoogle` |
| P13-E3 | email collision | google login without link | no auto educator bind | `go test ./internal/api -run NoEmailMergeEducator` |
| P13-E4 | dual-accept | JWT + static + bad | matrix; metric | `go test ./internal/api -run ServiceDualAccept` |
| P13-E5 | production config empty secret | start | fail closed | `go test ./internal/config -run ServiceAuthProd` |
| P13-E6 | cookie jar browser-less | BFF login | no localStorage token; CSRF | `go test ./internal/api -run BFFSession` |

## Anti-Cheating Audit

- SharedSecretGuard empty-secret inert path **gone** in production (read `secret.go` historically fail-open).
- Parent guard must not accept JWT without aud check.
- SPA tests must fail if token written to localStorage in production build.
- identity_subject not set by email match job.
- Student routes still device-token only.
- Do not remove password path while unlinked users exist.

## Completion Gate

- [ ] P13-S*/E* green
- [ ] `make test` auth-related packages green; cover gate not regressed beyond agreed
- [ ] OpenAPI/client regenerated
- [ ] Rollback flag documented and tested
- [ ] Anti-cheat clean
- [ ] S3–S5 checklist signed in PR description

## Dependencies

- Upstream: Identity phases 7–9, 11
- Downstream: Phase 14 S6–S7
- Cross: TV server may start using service JWT here for LMS ingest

## Rollback

- Flags: `LMS_IDENTITY_LOGIN=off`, `LMS_SERVICE_AUTH_MODE=legacy`
- Re-enable localStorage gate only in development
- Keep identity_subject column (harmless)
