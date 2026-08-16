# 06: IB2 / Primer ES256 JWT and JWKS bridge

**Status: STOP — candidate dependency under independent exact-tip review; no dispatch.**

## Goal

Add Primer-owned ES256 access-token issuance and public JWKS/AS metadata after IB1 code/grant composition. Use [`../stytch-identity-ib0/02-http-oauth-bff-mcp-contract.md`](../stytch-identity-ib0/02-http-oauth-bff-mcp-contract.md) as the token profile.

## BDD Success Criteria

#### Scenario: IB2-S1 — Strict Primer JWT
- **Given** a consumed valid Primer authorization code
- **When** `/oauth/token` issues an access token
- **Then** it is ES256, `typ=at+jwt`, `sub=accounts.id`, exactly one audience, required signed public `client_id` from `oauth_clients.client_id`, and expires in ≤15m/provider bound
- **And** `azp`, internal OAuth-client UUIDs, provider tuple/role/org/session, and product roles are absent.

#### Scenario: IB2-S2 — JWKS consumer validation
- **Given** public `/.well-known/jwks.json` and AS metadata
- **When** Studio-like validators fetch keys
- **Then** valid Primer tokens pass and wrong/multiple audience, raw Stytch, alg substitution, unknown kid, expiry, and missing/wrong/overlong/control-bearing/non-registered `client_id` fail closed
- **And** `azp` or an internal UUID substituted as `client_id` is rejected.

## Implementation Instructions

- Intake the read-only donor only by the safe sequence in [`../stytch-identity-ib0/jwks-donor-evidence.md`](../stytch-identity-ib0/jwks-donor-evidence.md); do not cherry-pick wholesale or overwrite IA-R/IB1.
- Implement missing JWT issuer, `/oauth/token`, JWKS and metadata handlers; donor custody alone is insufficient.
- Emit and validate the stable public `oauth_clients.client_id` string as the required signed `client_id` claim for human and service tokens. Do not emit `azp` and never serialize the internal `oauth_clients.id` UUID into a public claim.
- Reserve migration numbers after IB1 and create the signing-key custody needed for initial issuance, including at-most-one `next` and at-most-one `active`. IB7 owns the complete next/active/retired/destroyed rotation, timestamp ordering, and private-material destruction gate.
- Add the initial hashed refresh-family/token rows required by the IB0 atomic authorization-code issuance response and satisfy the exact active-family/one-current-token creation constraints immediately. Refresh redemption, rotation, terminal lifecycle, reuse detection, revoke, logout, and long-lived refresh retention remain IB6.
- No human refresh behavior beyond that initial atomic issuance contract until IB6; no Stytch token exchange endpoint.

## End-to-End Test Plan

Run IB2-E00..E11: selective donor review; public token→JWKS validator path; negative algorithm/audience/kid/time matrix; required public `client_id` claim and absent-`azp` allowlist; missing/wrong/overlong/control-bearing/non-registered/internal-UUID client IDs; TTL cap; private_key_jwt form/replay/challenge; RFC8414 metadata; RFC7009 revoke/PKCE negatives; concurrent public code consume/replay; signer/commit/lost-response atomicity; Studio MRTR handle/confirm binding to the validated public `client_id`; and fresh/upgrade/down migration proof for IB2-created signing-key, assertion-replay, initial refresh-family/current-token, and token-issuance-audit tables. Prove at most one initial active/next key without claiming retired/destroyed rotation, exact initial refresh/code-exchange constraints, and 24-hour code purge preserving copied 400-day issuance evidence through nullable `ON DELETE SET NULL`. Use real key custody/PostgreSQL and process HTTP.

## Anti-Cheating Audit

No symmetric fallback, multi-audience token, missing or internal-UUID `client_id`, `azp` alias, provider SessionJWT passthrough, roles/org IDs in claims, private key in JWKS/logs/plaintext, hard-coded signer, or custody-only test labelled token/JWKS proof.

## Completion Gate

- [ ] IB2-S* and IB2-E00..E11 green.
- [ ] IB2-owned migration constraints and copied issuance-evidence retention pass; refresh rotation/reuse remains IB6 and full key retirement/destruction remains IB7.
- [ ] JWKS/AS metadata/OpenAPI parity and signer-aware readiness pass.
- [ ] Fresh exact-tip specification and security review approves donor-derived and new code.
