# 03: IA-R / residual remediation and review

## Goal

Close the blocking foundation residuals before composition. This plan is Stytch-backed: Stytch is upstream human-session authority; Primer Identity is the only Stytch client and mints only downstream Primer material where this phase authorizes it.

## BDD Success Criteria

### Scenario: IA-R / residual remediation and review

- **Given** the preceding phase gates and a bounded, sanitized test environment
- **When** per-token invalidation only cancels/epochs the matching in-flight validation while InvalidateAll fences all; production disabled/non-live/override configurations fail.
- **Then** the behavior is observable through the named public boundary and durable local evidence
- **And** no raw Stytch session, SessionJWT, provider payload, or provider-derived product role crosses the Identity boundary

## Implementation Instructions

- Preserve exact tuple mapping and no-email-merge semantics; never use Stytch organization/member roles as product authorization.
- Keep product authorization and host-only cookie/CSRF ownership in the product BFF; Identity is neither a product membership store nor an independent human session authority.
- Record the durable source of truth, failure semantics, migration/rollout constraints, audit fields, and focused test command before implementation.
- **Dependency gate:** Fresh quality and specification approval.

## End-to-End Test Plan

Race/concurrency regression plus dependency/index migration verification. Use public endpoints/processes and a real local durable store where applicable; permitted provider fakes prove only the bounded Identity adapter boundary and cannot substitute for the explicit live-provider gate.

## Anti-Cheating Audit

Global cancellation must not be reused for a single token; avoid duplicate tuple index. Review handlers, caches, persistence and audit logs for hard-coded success, test-only bypasses, raw-token persistence, swallowed provider errors, role/tenant derivation, or direct product-to-Stytch paths.

## Completion Gate

- [ ] The scenario and its negative/cross-boundary cases pass at a public boundary.
- [ ] Durable state, replay/retry behavior, and sanitized audit evidence are verified where applicable.
- [ ] No Stytch material or provider authorization leaks to products.
- [ ] `git diff --check`, documentation links/headings, and applicable build/test gates pass.
- [ ] The next dependency is not unblocked merely by a library-only or mocked proof.
