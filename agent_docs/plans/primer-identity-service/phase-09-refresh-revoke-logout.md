# Phase 9: Refresh, revoke, and logout

**File:** `phase-09-refresh-revoke-logout.md`
**Depends on:** Phases 5, 7
**Duration guess:** 4–6 days
**Migration stage:** S1–S2
**Handoff wave:** W5

## Goal

Complete session lifecycle: refresh token **rotation** (server-side / BFF-held only), RFC 7009 revocation, logout fail-closed, denylist of jti/sid on security events, and proof that re-login does not globally logout other devices. Close theft/replay windows for refresh handles.

## Scope

### In scope

- `refresh_sessions` or session-bound refresh handles: hashed token, family id, rotated_from, expires, revoked
- `POST /oauth/token` grant_type=refresh_token with rotation (new refresh, invalidate old); reuse of old → revoke family (theft detection)
- `POST /oauth/revoke` RFC 7009 — access jti and/or refresh; auth client; idempotent
- `POST /v1/logout` and BFF logout path: revoke session server-side; clear cookies only after successful revoke **or** already-gone; hard store errors → 5xx without false ok
- `jti_denylist` / sid revoke checks in verifier optional path + required on account lock
- Unrelated sessions survive login (narrow rotation from Phase 5 remains)
- Idle sliding capped by absolute expiry

### Out of scope

- Google RP-initiated logout (deferred)
- Product SPA UI polish

## BDD Success Criteria

#### Scenario: P9-S1 — Refresh rotation success

- **Given** valid refresh handle at BFF/Identity
- **When** refresh grant called
- **Then** new access token issued
- **And** new refresh issued; old refresh rejected thereafter

#### Scenario: P9-S2 — Refresh theft / reuse detection

- **Given** rotated-away refresh reused
- **When** token endpoint sees reuse
- **Then** refresh family revoked
- **And** subsequent refreshes fail

#### Scenario: P9-S3 — Revoke access/refresh

- **Given** active tokens
- **When** POST `/oauth/revoke`
- **Then** refresh unusable
- **And** access jti denylisted or session revoked so verifier fails where implemented

#### Scenario: P9-S4 — Revoke fail-closed on store error

- **Given** revoke path with injected store failure
- **When** client calls revoke/logout
- **Then** 5xx
- **And** client not told success while token still valid (except already-gone)

#### Scenario: P9-S5 — Logout fail-closed

- **Given** authenticated session
- **When** logout hits real revoke error (inject hook; do not Close entire DB)
- **Then** not `ok:true` with cleared cookies while row live
- **And** 503-class error

#### Scenario: P9-S6 — Unrelated sessions survive login

- **Given** user has session S2 on device B
- **When** device A re-logins (narrow rotate A)
- **Then** S2 still resolves
- **And** only A’s prior session revoked

#### Scenario: P9-S7 — Logout idempotent when already gone

- **Given** session already revoked
- **When** logout again
- **Then** success clear cookies allowed
- **And** no 5xx solely due to ErrNoRows

#### Scenario: P9-S8 — Account lock denylists sessions

- **Given** active sessions and access JWT
- **When** account locked
- **Then** all sessions revoked
- **And** verifier rejects sid/jti as designed

#### Scenario: P9-S9 — No tokens in logout logs

- **Given** logout/revoke
- **When** logs captured
- **Then** raw tokens absent (jti/sid ok)

## Implementation Instructions

1. Hash refresh tokens at rest; return raw once.
2. Rotation in one transaction: insert new, revoke old, detect reuse race.
3. Logout BFF: CSRF+Origin; call Identity revoke; clear local session.
4. Test hook `RevokeSessionFunc` for fail-closed without store.Close.
5. Extend verifier with denylist lookup bounded TTL cache.
6. Metrics: refresh, revoke, logout failures.

## End-to-End Test Plan

| ID | Setup | Action | Assert | Command |
| --- | --- | --- | --- | --- |
| P9-E1 | login+refresh | rotate twice | old invalid; new valid | `go test ./internal/session -run RefreshRotate -race` |
| P9-E2 | reuse old refresh | token | family dead | `go test ./internal/session -run RefreshTheft` |
| P9-E3 | logout inject err + happy + already-gone | POST logout | fail-closed matrix | `go test ./internal/api -run Logout -race` |
| P9-E4 | two sessions re-login one | login A | B alive | `go test ./internal/api -run UnrelatedSessions` |

## Anti-Cheating Audit

- No `_ = RevokeSession` then always ok.
- Refresh not returned to browser JS in bffref.
- Theft detection not disabled in test builds used for gate.
- Cookie clear encoding Max-Age=0 accepted as clear.

## Completion Gate

- [ ] P9-S*/E* green
- [ ] Fail-closed logout/revoke proven
- [ ] Unrelated session proof green
- [ ] Anti-cheat clean

## Dependencies

- Upstream: 5, 7
- Downstream: 11–14

## Rollback

- Feature flag disable refresh; force short access re-login.
