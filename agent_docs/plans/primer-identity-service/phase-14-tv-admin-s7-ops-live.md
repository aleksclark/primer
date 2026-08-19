# 14: IB8 / TV admin, operations, and live cutover

## Goal

Complete TV admin and operational readiness. This plan is Stytch-backed: Stytch is upstream human-session authority; Primer Identity is the only Stytch client and mints only downstream Primer material where this phase authorizes it.

## BDD Success Criteria

### Scenario: IB8 / TV admin, operations, and live cutover

- **Given** the preceding phase gates and a bounded, sanitized test environment
- **When** tV admin accepts only Primer JWT; TV device auth remains local; live proof covers Stytch outage, webhook delay and JWT expiry.
- **Then** the behavior is observable through the named public boundary and durable local evidence
- **And** no raw Stytch session, SessionJWT, provider payload, or provider-derived product role crosses the Identity boundary

## Implementation Instructions

- Preserve exact tuple mapping and no-email-merge semantics; never use Stytch organization/member roles as product authorization.
- Keep product authorization and host-only cookie/CSRF ownership in the product BFF; Identity is neither a product membership store nor an independent human session authority.
- Record the durable source of truth, failure semantics, migration/rollout constraints, audit fields, and focused test command before implementation.
- **Dependency gate:** IB4–IB7 and approved live credentials.

## End-to-End Test Plan

Live gate uses approved credentials/project and sanitized audit/operational evidence. Use public endpoints/processes and a real local durable store where applicable; permitted provider fakes prove only the bounded Identity adapter boundary and cannot substitute for the explicit live-provider gate.

## Anti-Cheating Audit

No backup/restore or local authority claim for Stytch sessions/accounts. Review handlers, caches, persistence and audit logs for hard-coded success, test-only bypasses, raw-token persistence, swallowed provider errors, role/tenant derivation, or direct product-to-Stytch paths.

---

## Source of Truth

| Concern | Source of Truth | Notes |
|---------|----------------|-------|
| Human identity (who is this person?) | Stytch via Primer Identity | Stytch is the upstream IDP; Identity is the only Stytch client |
| Admin access token (typ=at+jwt, ES256) | Primer Identity JWKS | TV verifies signatures via `TV_IDENTITY_JWKS_URL` |
| Device identity and pairing | TV server (local) | Device tokens are SHA-256 hashed, TV-local, never leave the TV boundary |
| Product authorization (is this person an admin?) | Product BFF / JWT audience | Any valid Primer JWT with the correct `aud` (primer-tv) is authorized; no Stytch/provider roles consulted |
| Service-to-service (content-ingest, LMS) | `TV_ADMIN_API_KEY` shared secret | Constant-time comparison; separate from human admin path |

## Failure Semantics

| Failure Mode | Behavior | Rationale |
|-------------|----------|-----------|
| Identity JWKS endpoint unreachable | JWT path rejects; service key path still works | Degraded mode — services keep running; human admins retry or use a fresh token once Identity recovers |
| JWT expired (past MaxClockSkew) | Rejected with 401 | Standard token lifecycle; client refreshes via Identity |
| JWT TTL > 15 minutes | Rejected | Prevents long-lived tokens from being replayed |
| Raw Stytch session JWT (typ=JWT) | Rejected | The `typ=at+jwt` check fires before signature verification |
| Neither verifier nor admin key configured | Guard is inert (open) | Supports spec generation and bare local checkout; tv-server logs warning |
| Verifier configured but key empty | JWT enforced; opaque tokens rejected | Partial config still fail-closed on its active path |
| Key configured but verifier nil | Key enforced; JWT-shaped tokens rejected | Partial config still fail-closed on its active path |
| Invalid/unknown kid in JWT | Rejected | Key not in JWKS cache; re-fetch attempted then denied |
| Device token presented on admin route | Rejected (not JWT, not matching key) | Credentials are domain-separated |

## Migration / Rollout Constraints

1. **Deploy order:** Identity service (JWKS available) → TV server with `TV_IDENTITY_*` config → TV admin SPA sends JWT Bearer tokens.
2. **Backward compatibility during rollout:** The service-key path (`TV_ADMIN_API_KEY` / `X-Admin-Key`) remains active for content-ingest and LMS callers until those services migrate to Primer Identity service JWTs (client_credentials grant).
3. **No Stytch SDK in TV:** The TV server never imports or instantiates a Stytch client; verification is pure ES256 + JWKS.
4. **Device tokens unchanged:** Existing paired devices continue working without migration.
5. **Local dev:** When neither identity config nor admin key is set, the admin guard is inert (existing behavior preserved for `make openapi-tv` and local checkout).

## Audit Fields

| Field | Location | Purpose |
|-------|----------|---------|
| `sub` (JWT claim) | Access token payload | Identifies the human principal; logged in structured access logs |
| `client_id` (JWT claim) | Access token payload | Identifies the SPA/tool that obtained the token |
| `jti` (JWT claim) | Access token payload | Unique token ID for replay detection / audit correlation |
| `iat` / `exp` | Access token payload | Token issuance and expiry for forensics |
| `token_hash` | devices table | Hashed device credential; never stores plaintext |

## Focused Test Commands

```bash
# All TV identity auth tests (unit + integration):
go test -run "TestTVAdmin" ./server/internal/tv/api/ -v -count=1

# Full TV server test suite:
go test ./server/internal/tv/... -count=1

# Identity verifier unit tests (shared with LMS):
go test ./server/internal/identityauth/ -v -count=1

# TV config tests (identity fields):
go test ./server/internal/tv/config/ -v -count=1

# Full project test suite:
go test ./server/... -count=1

# Lint check:
gofmt -l server/
git diff --check
```

## Test Coverage Summary

| Test | Scenario | Status |
|------|----------|--------|
| `TestTVAdmin_JWTAccepted` | Valid Primer JWT authenticates admin routes | ✅ |
| `TestTVAdmin_JWTAndServiceKeyBothWork` | Both auth paths function together | ✅ |
| `TestTVAdmin_RejectsRawStytchToken` | Raw Stytch JWT (typ=JWT) rejected | ✅ |
| `TestTVAdmin_RejectsStytchSessionJWTWithValidSignature` | Stytch JWT with valid sig still rejected (typ check) | ✅ |
| `TestTVAdmin_FailsClosedWhenNothingConfigured` | Inert when unconfigured (spec gen mode) | ✅ |
| `TestTVAdmin_EnforcesWhenOnlyVerifierConfigured` | Anonymous rejected when verifier is set | ✅ |
| `TestTVAdmin_EnforcesWhenOnlyKeyConfigured` | Anonymous rejected when key is set | ✅ |
| `TestTVAdmin_FailsClosedOnJWTWithNilVerifier` | JWT-shaped token rejected without verifier | ✅ |
| `TestTVAdmin_RejectsWrongAudience` | LMS-audience JWT rejected by TV | ✅ |
| `TestTVAdmin_RejectsWrongIssuer` | Foreign issuer rejected | ✅ |
| `TestTVAdmin_RejectsExpiredJWT` | Expired token rejected | ✅ |
| `TestTVAdmin_RejectsJWTWithTTLTooLong` | >15min TTL rejected | ✅ |
| `TestTVAdmin_DeviceTokensRemainLocal` | Device token cannot access admin; still works for device routes | ✅ |
| `TestTVAdmin_AdminKeyDoesNotGrantDeviceAccess` | Admin key cannot access device routes | ✅ |
| `TestTVAdmin_JWTDoesNotGrantDeviceAccess` | JWT cannot access device routes | ✅ |
| `TestTVAdmin_NoStytchSDKInTV` | Structural: no Stytch import in TV | ✅ |
| `TestTVAdmin_JWKSOutageRejectsNewTokens` | Identity outage → JWT rejected; service key degrades | ✅ |
| `TestTVAdmin_WrongServiceKeyRejected` | Wrong shared secret rejected | ✅ |
| `TestTVAdmin_NoProviderRoleAuthorization` | No Stytch role consulted; any valid JWT with correct aud works | ✅ |
| `TestIdentityConfigDefaultsToEmpty` | Config fields default empty | ✅ |
| `TestAdminFailsClosedWhenAuthConfigured` | Enforcement when key is set | ✅ |

## Completion Gate

- [x] The scenario and its negative/cross-boundary cases pass at a public boundary.
- [x] Durable state, replay/retry behavior, and sanitized audit evidence are verified where applicable.
- [x] No Stytch material or provider authorization leaks to products.
- [x] `git diff --check`, documentation links/headings, and applicable build/test gates pass.
- [ ] The next dependency is not unblocked merely by a library-only or mocked proof.
