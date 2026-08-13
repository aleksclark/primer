# Phase 14: TV admin SSO, S7 cutover, ops, and live Google (BLOCKED)

**File:** `phase-14-tv-admin-s7-ops-live.md`
**Depends on:** Phase 13
**Duration guess:** 7–14 days (live items indefinite until approval)
**Migration stage:** S6, S7 + ops
**Handoff wave:** W8

## Goal

Complete product migration: human TV admin SSO via Identity (retire browser `X-Admin-Key` localStorage), keep TV **device** tokens product-local, execute **S7** disable of legacy static secrets and optional passwords with lockout-safe gates, ship deploy/backup/restore/runbooks, and provide a **credential-gated** live Google proof path that remains **BLOCKED** until human approval. Credential-free loopback evidence must not close live boxes.

## Scope

### In scope

**TV admin human (S6):**

- TV admin BFF (`tv-admin-bff`) or shared LMS host admin session with role `tv_admin` / LMS admin
- Replace `tv-web` localStorage admin key gate with cookie session
- `requireAdmin` accepts Identity JWT aud=`primer-tv-admin` or `primer-lms`+role — **not** device tokens
- Feature flag restore `X-Admin-Key` for emergency rollback

**Device tokens:**

- Explicit tests: device bearer cannot call admin routes; admin JWT cannot call device-only routes without device record

**S7 cutover:**

- `SERVICE_AUTH_MODE=jwt_only` on LMS/Studio/TV machine paths after metrics ~0 legacy
- Password disable per-account only when second factor present; global password off optional policy
- Remove SPA dead code for token paste in production builds

**Ops:**

- `deploy/primer-identity.nomad.hcl.tmpl` or compose prod sample
- Backup: accounts, external_identities, clients, hashed secrets, signing key material references; sessions optional
- Restore drill: keys+accounts restore; everyone re-login acceptable
- Health/alerts: JWKS, login fail rate, legacy secret count=0 gate
- Runbooks: rotation, break-glass, rollback S6/S7

**Live Google (BLOCKED):**

- Checklist: real client id/secret, redirect URIs, HTTPS issuer, monitored
- Suite `make identity-test-live-google` exits 78 / skips without credentials
- Never green on loopback alone

### Out of scope

- Student Google accounts
- Passkeys/DPoP/mTLS hardening beyond prior phases
- Claiming production AI/other unrelated systems

## BDD Success Criteria

#### Scenario: P14-S1 — TV admin SSO login

- **Given** Identity account with TV admin authorization projection
- **When** admin uses BFF login
- **Then** tv-web session cookie works for admin APIs
- **And** no admin key in localStorage

#### Scenario: P14-S2 — X-Admin-Key rollback flag

- **Given** emergency flag enabled
- **When** static admin key presented
- **Then** admin routes work
- **And** flag default off in production templates

#### Scenario: P14-S3 — Device tokens remain product-local

- **Given** paired TV device token
- **When** used on admin route
- **Then** 401/403
- **And** device routes still accept device token not human JWT alone

#### Scenario: P14-S4 — S7 jwt_only services

- **Given** legacy acceptance disabled
- **When** static SERVICE_TOKEN presented
- **Then** 401
- **And** valid service JWT still works

#### Scenario: P14-S5 — S7 password disable safety

- **Given** educator without second factor
- **When** S7 job runs
- **Then** password remains enabled for that educator
- **And** linked educators may disable password per policy

#### Scenario: P14-S6 — Backup and restore

- **Given** backup of Identity DB + key refs
- **When** restore to empty instance
- **Then** accounts and clients present
- **And** sessions may be empty (re-login)
- **And** JWKS signing works with restored keys

#### Scenario: P14-S7 — Live Google BLOCKED without credentials

- **Given** no approved live credentials in env
- **When** live suite runs
- **Then** reports BLOCKED / exit 78
- **And** does not false-pass on loopback

#### Scenario: P14-S8 — S0–S7 stage board complete or blocked

- **Given** ops checklist
- **When** reviewed
- **Then** each stage marked done/blocked with evidence links
- **And** S7 requires support sign-off artifact

#### Scenario: P14-S9 — Rollout rollback S6/S7

- **Given** production incident
- **When** rollback flags enabled within runbook TTL
- **Then** prior authenticator works
- **And** audit records re-enablement with expiry

## Implementation Instructions

1. TV: refactor `server/internal/tv/api/auth.go` admin guard; keep `requireDevice` separate.
2. tv-web: rewrite `tv-web/src/api/auth.ts` and admin-key-gate analogous to LMS S5.
3. Config: `TV_ADMIN_AUTH_MODE=dual|jwt|legacy_key`.
4. S7 job: explicit CLI with dry-run report of accounts that would lock out — refuse if nonzero without `--force` break-glass.
5. Deploy templates with secrets from vault; no plaintext in git.
6. Live test build tag `livegoogle` separate from default CI.
7. Update LikeC4 only if edges change (should already be decided).
8. Final adversarial sweep: re-run identity-test-oauth + LMS dual + TV isolation.

## End-to-End Test Plan

| ID | Setup | Action | Assert | Command |
| --- | --- | --- | --- | --- |
| P14-E1 | TV+Identity harness | admin SSO | cookie admin ok; no localStorage key | `go test ./internal/tv/api -run AdminSSO` |
| P14-E2 | device vs admin | cross guards | isolation holds | `go test ./internal/tv/api -run DeviceNotAdmin` |
| P14-E3 | jwt_only mode | static secret | 401; JWT 200 | `go test ./internal/api -run S7JWTOnly` |
| P14-E4 | backup/restore testcontainer | restore | accounts+keys work | `go test ./internal/ops -run BackupRestore` |
| P14-E5 | live suite without creds | run | BLOCKED exit 78 | `make identity-test-live-google` |
| P14-E6 | stage checklist tool/doc | verify | S0–S7 statuses | manual+script |
| P14-E7 | rollback flags | enable legacy | prior path works | `go test ./internal/config -run RollbackFlags` |

## Anti-Cheating Audit

- Live Google not marked complete from loopback.
- Device token ≠ admin.
- S7 job cannot disable passwords for unlinked users without force+audit.
- Backup test restores real SQL not fixture JSON pretend.
- tv-web production build grepped for localStorage admin key writes.
- Legacy re-enable requires expiry timestamp.

## Completion Gate

- [ ] P14-S1–S6, S8–S9 green credential-free
- [ ] P14-S7 explicitly BLOCKED until approval (documented)
- [ ] S7 sign-off checklist attached when cutting prod
- [ ] Deploy/backup runbooks merged
- [ ] Full adversarial oauth suite still green
- [ ] Anti-cheat clean
- [ ] Plan completion rule satisfied for credential-free scope

## Dependencies

- Upstream: Phase 13
- External: Google cloud project, DNS/TLS, KMS, Nomad/deploy auth, support sign-off

## Rollback

- S6: `TV_ADMIN_AUTH_MODE=legacy_key`
- S7: temporary `dual` mode with ticket expiry ≤7d
- Identity outage: password path for still-enabled users; break-glass ops
