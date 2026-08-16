# 10: IB6 / refresh, logout, and lifecycle

## Goal

Complete Provider-plus-Primer lifecycle with no parallel human authority. This plan is Stytch-backed: Stytch is upstream human-session authority; Primer Identity is the only Stytch client and mints only downstream Primer material where this phase authorizes it.

## BDD Success Criteria

### Scenario: IB6 / refresh, logout, and lifecycle

- **Given** the preceding phase gates and a bounded, sanitized test environment
- **When** refresh rotates, a consumed token is reused, or logout/revoke/expiry clears product material and invalidates local cache/grant as applicable without extending beyond Stytch validity
- **Then** the behavior is observable through the named public boundary and durable local evidence
- **And** active families have exactly one current token; terminal families have none; consumed tokens have same-family sequence `n+1` successors; reuse atomically records timestamps and revokes the family, grant, and every live token
- **And** no raw Stytch session, SessionJWT, provider payload, or provider-derived product role crosses the Identity boundary

## Implementation Instructions

- Preserve exact tuple mapping and no-email-merge semantics; never use Stytch organization/member roles as product authorization.
- Keep product authorization and host-only cookie/CSRF ownership in the product BFF; Identity is neither a product membership store nor an independent human session authority.
- Record the durable source of truth, failure semantics, migration/rollout constraints, audit fields, and focused test command before implementation.
- Enforce the exact NULL-safe family/token status/timestamp/successor constraints and named lifecycle trigger from IB0; do not rely on the partial unique index for existence or transition correctness.
- Retain refresh/grant/revocation/audit evidence for 400 days; do not introduce a shorter purge that conflicts with retained `RESTRICT` evidence.
- **Dependency gate:** IB3–IB4.

## End-to-End Test Plan

IB6-E01 covers first/current token, rotation successor linkage and sequence, concurrent single winner, consumed-token reuse, exact active/terminal family and token constraints, family+grant+all-live-token terminalization, no post-terminal issuance, idle≤absolute expiry, restart and migration-upgrade durability, logout/revoke, provider unavailable, audit without secrets, and 400-day refresh/grant/revocation/audit retention with FK-safe purge behavior. Use public endpoints/processes and real PostgreSQL; permitted provider fakes prove only the bounded Identity adapter boundary and cannot substitute for the explicit live-provider gate.

## Anti-Cheating Audit

No local human refresh/session becomes a new source of human authority. Review handlers, caches, persistence and audit logs for hard-coded success, test-only bypasses, raw-token persistence, swallowed provider errors, role/tenant derivation, or direct product-to-Stytch paths.

## Completion Gate

- [ ] The scenario and IB6-E01 negative/cross-boundary cases pass at a public boundary.
- [ ] Refresh rotation/reuse terminal constraints and 400-day retained evidence pass on real PostgreSQL across restart and migration upgrade.
- [ ] Durable state, replay/retry behavior, and sanitized audit evidence are verified where applicable.
- [ ] No Stytch material or provider authorization leaks to products.
- [ ] `git diff --check`, documentation links/headings, and applicable build/test gates pass.
- [ ] The next dependency is not unblocked merely by a library-only or mocked proof.
