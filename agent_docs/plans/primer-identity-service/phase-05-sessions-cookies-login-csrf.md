# Phase 5: Sessions, cookies, and login CSRF binding

**File:** `phase-05-sessions-cookies-login-csrf.md`
**Depends on:** Phase 4
**Duration guess:** 5–7 days
**Migration stage:** S1
**Handoff wave:** W3

## Goal

Establish host-only Identity sessions and **browser-bound** OAuth login transactions so authorization codes cannot be swapped into a victim browser (login CSRF). Deliver atomic one-time consume of state+binding, correlation cookie lifecycle, session idle/absolute expiry, and cookie flags that forbid parent-domain scope. This is the critical security seam before Google RP and product BFFs.

## Scope

### In scope

- Tables: `auth_sessions` (sid, account_id, client_id, amr, ip_hash, ua_hash, created_at, expires_at, absolute_expires_at, revoked_at)
- Tables: `oauth_login_transactions` — state_hash, nonce_hash, browser_binding_hash, code_verifier_sealed (AEAD) if Identity initiates upstream, redirect_uri, return client info, expires, consumed_at
- Cookies:
  - `__Host-id_session` (or `id_session` when HTTP dev cannot use __Host-): Secure (prod), HttpOnly, SameSite=Lax, Path=/, **host-only**
  - Correlation cookie e.g. `id_oauth_corr`: HttpOnly, Path scoped to `/oauth`, short Max-Age, host-only, distinct secret from URL `state`
- Login start path sets correlation cookie + stores binding hash
- Callback/authorize completion: require cookie; `Consume...ByStateAndBinding` atomic; clear cookie on success **and** all errors
- Session issue on Identity after successful authn factors
- Narrow rotation on re-login: revoke only preexisting session id on this browser if present — **not** RevokeAll
- Fail-closed if revoke of preexisting errors (not ErrNoRows)
- Test mode session mint for harnesses (prod reject)

### Out of scope

- Google token exchange (Phase 6)
- Product BFF cookies (Phase 7 contract)
- Refresh tokens (Phase 9)

## BDD Success Criteria

#### Scenario: P5-S1 — Host-only session cookie flags

- **Given** successful Identity login test path
- **When** Set-Cookie issued
- **Then** cookie has HttpOnly and SameSite=Lax or Strict
- **And** cookie attribute string has **no** `Domain=`
- **And** Secure set when IDENTITY_ENV=production or TLS mode

#### Scenario: P5-S2 — Session absolute and idle expiry

- **Given** session with absolute_expires_at past
- **When** resolve session
- **Then** unauthorized even if idle expires_at future
- **And** revoked_at sessions fail

#### Scenario: P5-S3 — Login CSRF victim jar fails

- **Given** attacker completes authorize start and obtains callback URL with valid state/code materials as applicable
- **When** victim client with **fresh cookie jar** (never hit login start) hits callback/authorize completion
- **Then** no Identity session cookie set on victim
- **And** no account side-effect beyond attacker’s own pre-existing state
- **And** safe error only

#### Scenario: P5-S4 — Binding mismatch rejects

- **Given** valid state but wrong/missing correlation cookie
- **When** completion attempted
- **Then** reject; transaction consumed or unusable
- **And** correlation cleared if present

#### Scenario: P5-S5 — state and nonce binding

- **Given** login transaction with state_hash and nonce_hash
- **When** completion presents wrong state or later id_token nonce mismatch (Phase 6 wires nonce end-to-end)
- **Then** reject without session

#### Scenario: P5-S6 — Replay rejected

- **Given** successful completion once
- **When** identical callback replayed
- **Then** reject
- **And** user/session counts unchanged on second try

#### Scenario: P5-S7 — Concurrent callback one success

- **Given** N parallel identical completion requests with pinned binding cookie
- **When** raced
- **Then** exactly one success
- **And** one session row for that login

#### Scenario: P5-S8 — No secrets in logs or error redirects

- **Given** failed completion
- **When** inspect Location, body, logs
- **Then** no raw state, code, correlation secret, tokens, or password

## Implementation Instructions

1. CSPRNG 32-byte correlation secret; store SHA-256 only; cookie value is raw secret.
2. AEAD seal for any PKCE verifier stored server-side (`IDENTITY_OAUTH_TX_KEY` ≥32 bytes; prod required).
3. Atomic consume: single UPDATE … WHERE consumed_at IS NULL returning row count=1 matching state_hash **and** browser_binding_hash; empty binding hash always reject.
4. Cookie clear: Max-Age=0 / expires past on all error paths including disabled OAuth.
5. Middleware resolve session from cookie → account; ignore `X-User-Id` headers.
6. Harness: cookie jars mandatory; concurrent tests pin binding via AddCookie not shared jar alone.
7. Document production rejection of test session mint before listen.

## End-to-End Test Plan

| ID | Setup | Action | Assert | Command |
| --- | --- | --- | --- | --- |
| P5-E1 | HTTP login test mode | inspect Set-Cookie | host-only flags; HttpOnly | `make identity-test-oauth` |
| P5-E2 | attacker jar complete start; victim fresh jar callback | GET callback | victim no session; /me equivalent 401; counts stable | `go test ./internal/api -run LoginCSRF -race` |
| P5-E3 | replay + wrong binding + missing cookie | complete | all safe fail | `go test ./internal/api -run OAuthTxReject` |
| P5-E4 | N=20 concurrent complete | race | successes==1 | `go test ./internal/api -run ConcurrentCallback -race` |
| P5-E5 | log sink during failures | complete errors | no secret substrings | `go test ./internal/api -run NoSecretLogs` |

## Anti-Cheating Audit

- DB-only state_hash without correlation cookie is a **Critical fail** — probe victim jar mandatory.
- Do not treat double-submit of URL state as binding.
- Concurrent test must not false-pass with successes=0 due to cleared shared jar.
- Session issue must not call RevokeAllUserSessions.
- Test mint disabled in production with subprocess listen proof.

## Completion Gate

- [ ] P5-S*/E* green including Login CSRF
- [ ] Cookie flag assertions automated
- [ ] Anti-cheat clean
- [ ] Phase 4 PKCE still green

## Dependencies

- Upstream: Phase 4
- Downstream: 6, 7, 9

## Rollback

- Revert PR; no product traffic yet.
