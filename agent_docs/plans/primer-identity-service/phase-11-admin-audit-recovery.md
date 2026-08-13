# Phase 11: Admin, audit, recovery, privacy

**File:** `phase-11-admin-audit-recovery.md`
**Depends on:** Phases 2, 9
**Duration guess:** 5–7 days
**Migration stage:** S1+
**Handoff wave:** W6

## Goal

Ship restricted admin operations (clients, service principals, account lock, session revoke-all, key rotate trigger), immutable-ish auth audit log, recovery codes / break-glass path, and privacy controls (PII minimization, export/delete hooks). Ensure admin is not reachable with ordinary product user JWTs.

## Scope

### In scope

- Admin authn: mTLS **or** break-glass session **or** `IDENTITY_ADMIN_TOKEN` hashed (dev) — production prefer mTLS/break-glass; document
- Admin routes under `/admin/v1/...` separate security scheme
- Ops: create/update oauth client redirects; mint/rotate service credentials; lock/unlock account; revoke-all sessions; list sessions; trigger key rotation
- `audit_events` append-only style: actor, action, target, ip_hash, ua_hash, outcome, request_id; no raw secrets
- Recovery codes: generate hashed set; one-time verify; re-issue invalidates
- Break-glass provider principals audited + rate limited
- Privacy: userinfo minimal; access tokens still omit email by default; data export/delete job emits `account.pending_deletion` event stub
- Explicit link APIs: `GET/POST /v1/account/links` step-up (password prove or reauth)

### Out of scope

- Polished admin SPA
- SIEM vendor export format
- SCIM

## BDD Success Criteria

#### Scenario: P11-S1 — Admin requires privileged auth

- **Given** ordinary human access JWT aud=primer-lms
- **When** call admin lock account
- **Then** 401/403
- **And** no lock

#### Scenario: P11-S2 — Admin lock revokes sessions

- **Given** admin auth + user with sessions
- **When** lock account
- **Then** status locked; sessions revoked
- **And** audit row written

#### Scenario: P11-S3 — Audit excludes secrets

- **Given** client secret rotation
- **When** audit event written
- **Then** event payload has no plaintext secret
- **And** action `client.secret_rotated` visible

#### Scenario: P11-S4 — Audit on login outcomes

- **Given** failed and successful logins
- **When** query audit
- **Then** outcomes recorded with hashed IP/UA
- **And** no password fields

#### Scenario: P11-S5 — Recovery code one-time

- **Given** issued recovery codes
- **When** one code used
- **Then** login/recovery succeeds once
- **And** reuse fails

#### Scenario: P11-S6 — Break-glass use audited

- **Given** break-glass principal
- **When** used
- **Then** audit + metric/alert hook
- **And** cannot silently mint product admin roles

#### Scenario: P11-S7 — Delete/export privacy path

- **Given** account delete requested
- **When** process runs
- **Then** account pending_deletion/anonymized per policy
- **And** event emitted for products to unlink subject_ref

## Implementation Instructions

1. Separate chi mount `/admin/v1` with strict auth middleware.
2. Hash admin token at startup compare if used.
3. Recovery codes: 10 codes, bcrypt/argon2 hash, single table.
4. Link flow: require authenticated session; step-up password or fresh Google reauth marker.
5. Rate limit recovery and admin lock.
6. Tests for admin negative with product JWT.

## End-to-End Test Plan

| ID | Setup | Action | Assert | Command |
| --- | --- | --- | --- | --- |
| P11-E1 | admin vs user JWT | lock | only admin | `go test ./internal/admin -run Authz` |
| P11-E2 | rotate secret | read audit | no secret | `go test ./internal/audit -run NoSecret` |
| P11-E3 | recovery codes | use twice | second fail | `go test ./internal/recovery -run OneTime` |
| P11-E4 | delete account | export/delete | event+status | `go test ./internal/admin -run PrivacyDelete` |

## Anti-Cheating Audit

- Admin routes not accidentally registered with product session guard only.
- Audit test must fail if secret substring present.
- Recovery codes not stored plaintext.
- Link API cannot link by email without proof.

## Completion Gate

- [ ] P11-S*/E* green
- [ ] Admin threat model notes in PR
- [ ] Anti-cheat clean

## Dependencies

- Upstream: 2, 9
- Downstream: 13–14

## Rollback

- Disable admin mount via config; keep audit table.
