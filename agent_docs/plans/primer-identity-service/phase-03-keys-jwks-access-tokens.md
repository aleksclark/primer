# Phase 3: Keys, JWKS, and access tokens

**File:** `phase-03-keys-jwks-access-tokens.md`
**Depends on:** Phase 2
**Duration guess:** 4–6 days
**Migration stage:** S1
**Handoff wave:** W2

## Goal

Introduce signing key lifecycle storage, public JWKS publication, and access-token mint/verify with **short TTL**, **single audience**, minimal claims, and strict alg/iss/exp enforcement. Products will validate locally via JWKS without introspection on the hot path.

## Scope

### In scope

- Tables: `signing_keys(kid, alg, public_jwk, private_key_sealed_or_ref, status active|next|retired, created_at, not_before, retire_after)`
- Dev/test: generate ES256 P-256 in process; seal private material with `IDENTITY_KEY_SEAL_SECRET` (≥32 bytes) or file/KMS interface stub
- `GET /oauth/jwks` — public keys only for active+next
- `GET /.well-known/openid-configuration` stub fields needed (issuer, jwks_uri, response_types, etc.) — full OP fields completed Phase 4
- Token service: MintAccessToken(account|service, aud, client_id, scope, sid?, amr, auth_time?)
- Claims per design §8; **omit** email, roles, workspace_ids by default
- Verify helper used by tests and later product validators package `internal/token` (exportable patterns documented for LMS/Studio copy)
- TTL config: human default 10m (≤15m); service default 5m (≤10m)
- Reject `alg=none`; pin ES256 (+ RS256 if enabled)
- readyz fails if no active signer

### Out of scope

- Full authorize/token grants (Phase 4)
- Key rotation ceremony UX (Phase 10 deepens)
- Product wiring

## BDD Success Criteria

#### Scenario: P3-S1 — JWKS serves public keys only

- **Given** an active signing key
- **When** client GET `/oauth/jwks`
- **Then** response contains JWK with kid and public components
- **And** response body has no private key material (`d` absent for EC)

#### Scenario: P3-S2 — Active signer present in JWKS

- **Given** server ready
- **When** readyz checked
- **Then** 200 only if active kid ∈ JWKS
- **And** missing signer → ready non-200

#### Scenario: P3-S3 — Mint human access JWT

- **Given** account sub A and client studio-bff
- **When** mint aud=`curriculum-studio` scope `openid studio:api`
- **Then** JWT verifies with JWKS
- **And** claims include iss, sub, aud, exp, iat, nbf, jti, client_id, scope, amr, auth_time, sid when session provided

#### Scenario: P3-S4 — Single audience only

- **Given** mint API
- **When** caller attempts multi-aud or empty aud
- **Then** mint rejected
- **And** no token issued

#### Scenario: P3-S5 — Short TTL enforced

- **Given** config max human TTL 15m
- **When** mint requested with TTL 1h
- **Then** rejected or clamped per fail-closed policy (**decided: reject**)
- **And** exp-iat ≤ 15m

#### Scenario: P3-S6 — Verify rejects bad iss/aud/sig/exp

- **Given** verifier for aud=primer-lms iss=IDENTITY_ISSUER
- **When** token has wrong aud, wrong iss, bad sig, or exp past skew
- **Then** verify error
- **And** no principal returned

#### Scenario: P3-S7 — Unknown kid fails closed until refetch path

- **Given** token kid not in cache
- **When** verify runs
- **Then** one JWKS refetch attempted (interface)
- **And** still-unknown kid fails (rotation success path Phase 10)

#### Scenario: P3-S8 — Access token omits email and roles

- **Given** account with email and no product roles concept
- **When** mint human token
- **Then** payload JSON has no `email`, `email_verified`, `roles`, `workspace_ids`

## Implementation Instructions

1. Use `lestrrat-go/jwx` or `golang-jwt` + manual JWK — pick one, pin version, document.
2. Prefer ES256; store public JWK JSON; private key sealed at rest (AES-GCM) for dev; interface `KeyBackend` for KMS later.
3. Normalize issuer string once (decide trailing slash policy; discovery and tokens must match).
4. jti = ULID/UUID; store optional denylist table stub empty until Phase 9.
5. OpenAPI register JWKS/discovery as public security:[].
6. Provide `internal/token.Verifier` with exact aud match (string or single-element array only — reject multi).
7. Tests: golden claim set; fuzz header alg confusion (`alg` none/HS256 with public key).

## End-to-End Test Plan

| ID | Setup | Action | Assert | Command |
| --- | --- | --- | --- | --- |
| P3-E1 | server + keys | GET jwks | 200; no private fields; kid present | `make identity-test` |
| P3-E2 | mint+verify round trip | HTTP or service | valid token; exp bound | `go test ./internal/token -run MintVerify` |
| P3-E3 | matrix wrong iss/aud/sig/exp/nbf | verify | all reject | `go test ./internal/token -run ClaimMatrix -race` |
| P3-E4 | decode payload | mint | no email/roles keys | `go test ./internal/token -run MinimizeClaims` |

## Anti-Cheating Audit

- JWKS handler must read from DB/key backend, not hard-coded JWK constant only in production path (tests may inject).
- Verifier must check signature — tests with alg=none and flipped signature required.
- Confirm single-aud by attempting array aud `["curriculum-studio","primer-lms"]` reject.
- Private key bytes never logged; seal secret not in JWKS.

## Completion Gate

- [ ] P3-S*/E* green
- [ ] readyz tied to signer
- [ ] Anti-cheat claim matrix green
- [ ] Discovery at least lists issuer + jwks_uri

## Dependencies

- Upstream: Phase 2
- Downstream: 4–10, 12–14

## Rollback

- Revert PR; clients not yet depending on JWKS in prod (S1 dark).
