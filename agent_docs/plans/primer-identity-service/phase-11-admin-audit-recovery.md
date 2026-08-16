# 11: IB7 / key rotation and hardening

## Goal

Harden Primer-issued token verification and operational boundaries. This plan is Stytch-backed: Stytch is upstream human-session authority; Primer Identity is the only Stytch client and mints only downstream Primer material where this phase authorizes it.

## BDD Success Criteria

### Scenario: IB7 / key rotation and hardening

- **Given** the preceding phase gates and a bounded, sanitized test environment
- **When** key rotation preserves only valid overlap and rejects token confusion; logs/audit include correlation only, never tokens/provider payload.
- **Then** the behavior is observable through the named public boundary and durable local evidence
- **And** unique partial indexes permit at most one `active` and at most one `next` signing key
- **And** lifecycle constraints enforce ordered `next → active → retired → destroyed` timestamps and require sealed private material until destruction, then require it to be absent
- **And** no raw Stytch session, SessionJWT, provider payload, or provider-derived product role crosses the Identity boundary

## Implementation Instructions

- Preserve exact tuple mapping and no-email-merge semantics; never use Stytch organization/member roles as product authorization.
- Keep product authorization and host-only cookie/CSRF ownership in the product BFF; Identity is neither a product membership store nor an independent human session authority.
- Record the durable source of truth, failure semantics, migration/rollout constraints, audit fields, and focused test command before implementation.
- Preserve `signing_keys_one_active_uq` and `signing_keys_one_next_uq`. Enforce status/timestamp/material checks: `next` has no lifecycle timestamps; `active` has `activated_at`; `retired` has ordered `retired_at`; `destroyed` has ordered `destroyed_at` and NULL private ciphertext. Rotation transactions must not bypass these constraints.
- **Dependency gate:** IB2/IB4.

## End-to-End Test Plan

Run IB7-E01..E04 rotation/JWKS cache and negative token-type E2Es. IB7-E01 is the owning full-lifecycle migration gate: on fresh/upgrade/down real PostgreSQL, reject a second active key, reject a second next key, reject every out-of-order `next → active → retired → destroyed` timestamp/material transition, reject missing ciphertext before destruction, and reject retained ciphertext after destruction. Prove old unexpired tokens still validate during overlap while retired/destroyed keys cannot sign. Use public endpoints/processes and the real durable store; permitted provider fakes prove only the bounded Identity adapter boundary and cannot substitute for the explicit live-provider gate.

## Anti-Cheating Audit

Stytch keys/session claims are never treated as local signing authority. Review handlers, caches, persistence and audit logs for hard-coded success, test-only bypasses, raw-token persistence, swallowed provider errors, role/tenant derivation, or direct product-to-Stytch paths.

## Completion Gate

- [ ] The scenario and IB7-E01..E04 negative/cross-boundary cases pass at a public boundary.
- [ ] Full next/active/retired/destroyed migration and rotation constraints pass on fresh/upgrade/down real PostgreSQL.
- [ ] Durable state, replay/retry behavior, and sanitized audit evidence are verified where applicable.
- [ ] No Stytch material or provider authorization leaks to products.
- [ ] `git diff --check`, documentation links/headings, and applicable build/test gates pass.
- [ ] The next dependency is not unblocked merely by a library-only or mocked proof.
