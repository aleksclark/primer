# Phase 4: OAuth clients and OP authorization-code + PKCE

**File:** `phase-04-oauth-clients-and-op.md`
**Depends on:** Phase 3
**Duration guess:** 5–7 days
**Migration stage:** S1
**Handoff wave:** W2

## Goal

Make Identity an OAuth 2.1 / OIDC **authorization server (OP)** for confidential product BFFs: client registry with exact redirect URI allowlists, discovery metadata, authorization endpoint issuing one-time codes bound to PKCE S256, and token endpoint exchanging codes for access tokens (refresh comes Phase 9). Browser binding cookies land fully in Phase 5; this phase establishes grant plumbing and PKCE verification.

## Scope

### In scope

- Tables: `oauth_clients`, `oauth_client_redirect_uris`, `oauth_authorization_codes` (hashed code, hashed code_challenge, method S256, client_id, account_id nullable until authn, redirect_uri, nonce_hash, expires, consumed_at, scope)
- Admin/seed API or bootstrap seed for dev clients: `studio-bff`, `lms-bff`, `tv-admin-bff` with secrets hashed
- `GET /.well-known/openid-configuration` complete enough: authorization_endpoint, token_endpoint, jwks_uri, response_types_supported=`code`, code_challenge_methods=`S256`, grant_types, subject_types, id_token signing algs
- `GET /oauth/authorize` — validate client_id, redirect_uri exact match, response_type=code, PKCE challenge present; if unauthenticated → hand off to login (Phase 5/6); if authenticated test path → issue code redirect
- `POST /oauth/token` — grant_type=authorization_code with code_verifier; authenticate confidential client (basic or body); PKCE verify; single-use code; mint access (+ id_token if openid)
- ID token: nonce, aud=client_id, iss, sub, exp short
- Exact redirect deny extras (query mismatch, trailing slash mismatch)
- Rate-limit stubs/hooks on authorize/token

### Out of scope

- Google federation (Phase 6)
- Full login CSRF cookie (Phase 5 completes binding)
- client_credentials (Phase 8)
- refresh_token grant (Phase 9)

## BDD Success Criteria

#### Scenario: P4-S1 — Register confidential client with redirect allowlist

- **Given** admin/bootstrap seed
- **When** client `lms-bff` registered with redirect `https://lms.example.com/auth/callback`
- **Then** secret stored hashed
- **And** plaintext secret shown once only at mint

#### Scenario: P4-S2 — Authorize rejects non-allowlisted redirect

- **Given** registered client
- **When** authorize with different redirect_uri
- **Then** error to safe display (not redirect to attacker) or RFC error without open redirect
- **And** no code issued

#### Scenario: P4-S3 — Discovery document consistent

- **Given** server issuer config
- **When** GET discovery
- **Then** issuer matches token iss
- **And** jwks_uri and authorization/token endpoints correct

#### Scenario: P4-S4 — PKCE S256 success

- **Given** authenticated test account session (Phase 5 seam or test inject documented)
- **When** authorize with challenge then token with correct verifier
- **Then** access_token returned
- **And** code consumed

#### Scenario: P4-S5 — PKCE failure

- **Given** issued code
- **When** token with wrong verifier
- **Then** 400 invalid_grant
- **And** code burned or remains unusable per decided burn-on-failure (**decided: burn**)

#### Scenario: P4-S6 — Auth code single-use

- **Given** successful token exchange
- **When** second token request same code
- **Then** reject
- **And** no second access token

#### Scenario: P4-S7 — Auth code TTL

- **Given** code older than ≤2 minutes
- **When** token exchange
- **Then** reject
- **And** no token

## Implementation Instructions

1. Hash authorization codes at rest (SHA-256); never store raw code.
2. Store `code_challenge` and method; verify S256(verifier)==challenge.
3. Confidential client auth: HTTP Basic or body client_id/client_secret; constant-time secret check.
4. redirect_uri comparison: exact byte match against registered URIs.
5. For pre-Phase-5 testing, allow `IDENTITY_AUTH_MODE=test` authorize completion via test login session mint (fail-closed in production) — same mode flag used later.
6. id_token optional but required when scope has openid; include nonce from authorize.
7. OpenAPI document token errors without leaking secrets.
8. Seed script for local clients used by Studio/LMS integration tests.

## End-to-End Test Plan

| ID | Setup | Action | Assert | Command |
| --- | --- | --- | --- | --- |
| P4-E1 | seed client | authorize bad redirect | no Location to evil; no code row | `go test ./internal/oauth -run RedirectAllowlist` |
| P4-E2 | discovery | GET well-known | fields match config | `go test ./internal/api -run Discovery` |
| P4-E3 | full code+PKCE happy | authorize→token | access verifies; id_token nonce | `go test ./internal/oauth -run PKCEHappy -race` |
| P4-E4 | replay code + bad PKCE + expired | token | all reject; single-use | `go test ./internal/oauth -run CodeLifecycle` |

## Anti-Cheating Audit

- Token endpoint must verify PKCE server-side — not skip when `ENV=test` broadly.
- redirect_uri allowlist not prefix match.
- Client secret not logged on failure.
- Code raw value not in DB; test SELECT proves hash only.
- Production rejects test authorize completion path.

## Completion Gate

- [ ] P4-S*/E* green
- [ ] Discovery + authorize + token paths real HTTP
- [ ] Seed clients documented
- [ ] Anti-cheat clean

## Dependencies

- Upstream: Phase 3
- Downstream: 5–9, 12–14
- Parallel: Phase 8 after clients table exists

## Rollback

- Revert PR; S1 dark — no product traffic.
