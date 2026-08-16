# 12: IB8 / Studio integration and authorization proof

## Goal

Cut Studio to validator-only consumption of Primer tokens. This plan is Stytch-backed: Stytch is upstream human-session authority; Primer Identity is the only Stytch client and mints only downstream Primer material where this phase authorizes it.

## BDD Success Criteria

### Scenario: IB8 / Studio integration and authorization proof

- **Given** the preceding phase gates and a bounded, sanitized test environment
- **When** valid mapped user token with local membership succeeds; valid token without membership, Stytch admin-like role without local role, and raw Stytch token are denied.
- **Then** the behavior is observable through the named public boundary and durable local evidence
- **And** no raw Stytch session, SessionJWT, provider payload, or provider-derived product role crosses the Identity boundary

## Implementation Instructions

- Preserve exact tuple mapping and no-email-merge semantics; never use Stytch organization/member roles as product authorization.
- Keep product authorization and host-only cookie/CSRF ownership in the product BFF; Identity is neither a product membership store nor an independent human session authority.
- Record the durable source of truth, failure semantics, migration/rollout constraints, audit fields, and focused test command before implementation.
- **Dependency gate:** IB2 and IB4 for production.

## End-to-End Test Plan

Studio public API and MCP E2E prove local workspace membership remains SoT. Use public endpoints/processes and a real local durable store where applicable; permitted provider fakes prove only the bounded Identity adapter boundary and cannot substitute for the explicit live-provider gate.

## Anti-Cheating Audit

Stytch org is not a Studio tenant/workspace; no direct Studio-Stytch edge. Review handlers, caches, persistence and audit logs for hard-coded success, test-only bypasses, raw-token persistence, swallowed provider errors, role/tenant derivation, or direct product-to-Stytch paths.

## Completion Gate

- [ ] The scenario and its negative/cross-boundary cases pass at a public boundary.
- [ ] Durable state, replay/retry behavior, and sanitized audit evidence are verified where applicable.
- [ ] No Stytch material or provider authorization leaks to products.
- [ ] `git diff --check`, documentation links/headings, and applicable build/test gates pass.
- [ ] The next dependency is not unblocked merely by a library-only or mocked proof.
