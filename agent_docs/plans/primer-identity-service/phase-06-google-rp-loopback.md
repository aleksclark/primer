# Phase 6: Google RP federation (loopback crypto)

**File:** `phase-06-google-rp-loopback.md`
**Depends on:** Phase 5
**Duration guess:** 6–8 days
**Migration stage:** S1 (protocol complete; live Google later)
**Handoff wave:** W3

## Goal

Implement Identity as OIDC **relying party** to Google (and test provider): authorization-code + PKCE to upstream, ID token verification with real crypto, upsert on `(google, sub)` only, strict claim bounds, bounded HTTP client with **CheckRedirect deny**, and production Google endpoint pins. Prove the full matrix on a **loopback IdP**; mark live Google **BLOCKED**.

## Scope

### In scope

- `internal/oauthtest` loopback IdP: ephemeral RSA, discovery, auth, token (JSON content-type), JWKS, PKCE S256, one-time codes, Force* knobs, HangDiscovery/Token/JWKS with absolute caps
- `IDENTITY_OIDC_PROVIDER=google|test` — production allows only google; test/dev allow test
- Google pin constants when provider=google (prod **and** development):
  - issuer `https://accounts.google.com`
  - auth `https://accounts.google.com/o/oauth2/v2/auth`
  - token `https://oauth2.googleapis.com/token`
  - jwks `https://www.googleapis.com/oauth2/v3/certs`
- Upstream login transaction fields; Identity session after upsert
- Claim validation: sub≤255; email required+verified at RP boundary ≤320; name≤200; UTF-8; no controls; **never truncate sub**
- HTTP client: Timeout default 10s; dial/TLS/header timeouts; body LimitReader; CheckRedirect → typed `ErrRedirectDenied` for 301–308 (incl 307 body re-POST)
- Injected client shallow-copy + force deny
- No access/refresh token persistence from Google for login-only scopes
- Packaging: oauthtest not imported by production cmd without build tag

### Out of scope

- Live Google credentials in CI
- Account linking UX (Phase 11/13)
- Product BFF (Phase 7)

## BDD Success Criteria

#### Scenario: P6-S1 — Loopback happy path real crypto

- **Given** provider=test loopback IdP with RSA JWKS
- **When** browser-like jar completes Identity login via Google RP simulation
- **Then** external_identities row `(google|test, sub)` exists
- **And** Identity session cookie set
- **And** signature actually verified (bad key fails sibling test)

#### Scenario: P6-S2 — Bad signature / wrong aud / exp rejected

- **Given** loopback Force* overrides
- **When** callback completes
- **Then** no user side effect for bad sig, wrong aud, exp past, nonce mismatch
- **And** safe error redirect/code only

#### Scenario: P6-S3 — Stable provider+sub upsert

- **Given** existing `(test, sub-A)` account
- **When** login again same sub
- **Then** same account id
- **And** no second account

#### Scenario: P6-S4 — Same email different sub no merge

- **Given** account A email E sub-A
- **When** login sub-B email E
- **Then** account B created distinct
- **And** A untouched

#### Scenario: P6-S5 — Claim bounds max accepted

- **Given** sub 255 bytes, email 320 valid shape, name 200
- **When** login
- **Then** success and stored equal

#### Scenario: P6-S6 — Claim bounds max+1 / missing email rejected

- **Given** sub 256 or email missing/unverified or controls
- **When** login
- **Then** reject; no user/session side effect
- **And** no claim content in error URL

#### Scenario: P6-S7 — HTTP timeout on discovery/token

- **Given** HangDiscovery or HangToken
- **When** New or callback
- **Then** errors within ~2s bound
- **And** no hang; no secret in error

#### Scenario: P6-S8 — CheckRedirect deny token/JWKS/discovery 301–308

- **Given** upstream token/JWKS/discovery returns redirect to sink (cross-host, same-host, relative, loop, metadata-like)
- **When** exchange/verify
- **Then** sink hits==0; no client_secret/code_verifier in sink body
- **And** typed deny; safe error surface without Location leak

#### Scenario: P6-S9 — Google endpoint pins

- **Given** provider=google with evil issuer or trailing-slash variant
- **When** Validate config in development or production
- **Then** fail closed
- **And** exact constant match required

#### Scenario: P6-S10 — Production rejects test provider

- **Given** IDENTITY_ENV=production and provider=test
- **When** process starts
- **Then** non-zero exit before listen

#### Scenario: P6-S11 — No Google tokens persisted or logged

- **Given** successful RP login
- **When** inspect DB and logs
- **Then** no Google access_token/refresh_token columns filled
- **And** logs lack token strings

## Implementation Instructions

1. Libraries: `golang.org/x/oauth2`, `github.com/coreos/go-oidc/v3` (re-pin if go get drifts Huma).
2. `oauth.New` builds dedicated client; never DefaultClient; never Background without timeout for discovery.
3. Scopes: `openid email profile` only for login.
4. UpsertExternalIdentity after claim validate only.
5. Map all protocol errors to safe codes (`auth_exchange_failed`, `auth_token_invalid`, …).
6. Build tag `//go:build oauthtest` or keep package only referenced from `_test.go` / e2e main with tag.
7. Document live Google as Phase 14 BLOCKED; no .env secrets committed.

## End-to-End Test Plan

| ID | Setup | Action | Assert | Command |
| --- | --- | --- | --- | --- |
| P6-E1 | loopback + cookie jar | full login | session+identity row; CSRF binding still holds | `make identity-test-oauth` |
| P6-E2 | ForceSub second + same email | two logins | two accounts | `go test ./internal/oidc -run NoEmailMerge` |
| P6-E3 | claim bounds matrix | login | max ok; max+1 reject | `go test ./internal/oidc -run ClaimBounds` |
| P6-E4 | redirector sinks 301–308 | exchange | zero sink hits | `go test ./internal/oidc -run RedirectDeny -race` |
| P6-E5 | evil google endpoints | Validate | error | `go test ./internal/config -run GooglePin` |
| P6-E6 | production+test provider | start | fail before listen | `go test ./internal/config -run ProdRejectTestIdP` |
| P6-E7 | log capture | login | no tokens | `go test ./internal/oidc -run NoTokenLogs` |

## Anti-Cheating Audit

- Direct mock of ValidateIDToken without crypto is insufficient for P6-E1.
- Google pin must be exact match not EqualFold/TrimSuffix.
- CheckRedirect deny required even when Google pin present.
- Production cmd `go list -deps` must not include oauthtest.
- Live Google not claimed complete.

## Completion Gate

- [ ] P6-S*/E* green on loopback
- [ ] Live Google checklist remains BLOCKED
- [ ] Anti-cheat redirect matrix green
- [ ] Packaging isolation proven

## Dependencies

- Upstream: Phase 5
- Downstream: 7, 12–14

## Rollback

- Revert PR; leave Phase 5 sessions intact if split commits.
