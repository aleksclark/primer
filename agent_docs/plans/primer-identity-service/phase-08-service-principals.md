# Phase 8: Service principals and client_credentials

**File:** `phase-08-service-principals.md`
**Depends on:** Phases 3–4
**Duration guess:** 4–6 days
**Migration stage:** S1 (issuance); product dual-accept Phase 13 (S4)
**Handoff wave:** W4

## Goal

Replace long-term static shared secrets with OAuth2 **client_credentials** service principals: hashed client secrets, audience+scope grants, short-lived service JWTs, and rotation with dual-valid grace. Emit metrics-friendly client_id claims for LMS/Studio validators.

## Scope

### In scope

- Tables: `service_principals`, `service_credentials` (secret_hash, label, expires_at, revoked_at), `service_grants` (audience, scopes[])
- `POST /oauth/token` grant_type=client_credentials
- Mint JWT: sub=service principal UUID, client_id claim, aud single, scope, amr=`client_secret`, jti, exp≤10m
- Admin mint/rotate/revoke credential (Phase 11 hardens authz; Phase 8 may use break-glass env bootstrap + tests)
- Secret shown once; argon2id/bcrypt hash at rest
- Rotation: new secret valid immediately; old valid until grace ≤7d or revoke
- Negative: wrong secret, revoked, expired, scope not granted, wrong aud grant

### Out of scope

- LMS SharedSecretGuard dual-accept (Phase 13)
- Human auth code changes

## BDD Success Criteria

#### Scenario: P8-S1 — client_credentials success

- **Given** principal with grant aud=primer-lms scope `ingest:instruction_logs`
- **When** POST token with client_id/secret
- **Then** access_token JWT verifies
- **And** aud and scope exact

#### Scenario: P8-S2 — Confidential auth required

- **Given** principal
- **When** token request without secret
- **Then** 401 invalid_client
- **And** no token

#### Scenario: P8-S3 — Secret hashed at rest

- **Given** newly minted secret
- **When** inspect DB
- **Then** plaintext secret absent
- **And** hash verifies only via check API

#### Scenario: P8-S4 — Rotation dual-valid grace

- **Given** rotated credential
- **When** old secret used within grace
- **Then** token still issued
- **And** after revoke/grace end old secret fails while new works

#### Scenario: P8-S5 — Scope and audience enforcement

- **Given** grant only primer-lms ingest scope
- **When** request scope studio:materialize or mint would imply other aud
- **Then** reject or issue only granted subset per policy (**decided: reject unknown requested scope**)
- **And** verifier side documented for products

## Implementation Instructions

1. Reuse token minter from Phase 3 with service claim profile.
2. Constant-time secret verify.
3. Do not put service roles in JWT beyond scopes.
4. Bootstrap: `IDENTITY_BOOTSTRAP_SERVICE_CLIENTS` JSON for dev only; prod uses admin API.
5. Metrics: token issues by client_id (not secret).
6. Document mapping to Studio `identity:svc:<id>` subject_ref.

## End-to-End Test Plan

| ID | Setup | Action | Assert | Command |
| --- | --- | --- | --- | --- |
| P8-E1 | seed principal | client_credentials | JWT verify aud/scope | `go test ./internal/serviceauth -run Happy` |
| P8-E2 | mint secret | DB inspect | hashed only | `go test ./internal/serviceauth -run Hash` |
| P8-E3 | rotate + grace + revoke | token | matrix allow/deny | `go test ./internal/serviceauth -run Rotation` |

## Anti-Cheating Audit

- Secrets not logged on mint beyond one-time response test capture.
- client_credentials must not issue human amr or sid.
- No static SERVICE_TOKEN equivalence inside Identity.

## Completion Gate

- [ ] P8-S*/E* green
- [ ] Grant model documented for LMS/Studio
- [ ] Anti-cheat clean

## Dependencies

- Upstream: 3–4
- Downstream: 10, 12–14
- Parallel with 5–7 OK

## Rollback

- Revert PR; products still on static secrets until 13.
