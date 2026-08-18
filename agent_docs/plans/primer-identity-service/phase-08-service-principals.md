# 08: IB4 / signed webhook and two-plane revocation

**Status: READY — IB0 exact-tip dependency passed at `4bfd6d5c03412d134a635c32279ef37b1c4e0d9e` with 0 Critical/Important/Minor findings.** The prior STOP is cleared; dispatch IB4/I8 from current master. The fresh IB4 exact-tip specification and quality/security approval remains required before any production BFF/MCP claim.

## Goal

Implement Identity-only Stytch/Svix receipt, durable event ledger, conservative cache invalidation, and provider-associated Primer grant/refresh revocation only after the candidate contract in [`../stytch-identity-ib0/04-stytch-webhook-and-provisioning.md`](../stytch-identity-ib0/04-stytch-webhook-and-provisioning.md) passes the IB0 zero-finding exact-tip review. IB4 remains the hard production BFF/MCP gate.

## BDD Success Criteria

#### Scenario: IB4-S1 — Verify and receipt before effects
- **Given** raw signed/malformed/oversized/old/future/forged events
- **When** `/webhooks/stytch` receives them
- **Then** only valid application/json ≤256KiB with complete Svix headers and ±5m timestamp is accepted
- **And** the unique event/message ledger commits before any effect.

#### Scenario: IB4-S2 — Replay/reorder idempotency
- **Given** exact duplicate, `event_id_reused_new_svix_id`, `svix_id_reused_new_event_id`, `cross_id_collision`, `body_hash_mismatch`, update→delete, and delete→older-update sequences
- **When** the worker retries/restarts
- **Then** exact duplicate is only both IDs resolving to the same receipt with the same body hash
- **And** each collision class is quarantined under the versioned semantic fingerprint that includes `reason_code`, never overwrites a receipt, and has no authority effect
- **And** concurrent identical observations yield one semantic event, one observation, and exactly one restart-safe alert item
- **And** older active updates never reactivate authority.

#### Scenario: IB4-S3 — Two-plane revoke without product role mutation
- **Given** relevant terminal member/org state
- **When** the verified event applies
- **Then** `InvalidateAll` executes and exact provider-associated grants/refresh families revoke
- **And** product memberships remain unchanged
- **And** old access JWTs remain honestly bounded by ≤15m expiry.

## Implementation Instructions

Pin reviewed Svix Go and use raw-body `Verify`, never timestamp-ignore. Accept only committed Dashboard-catalog fixtures for documented member/org event families; do not invent member-session webhook events. Add receipt/revocation/audit tables, immutable fingerprint-unique collision evidence for all four named classes, and the separate mutable alert-delivery ledger. The semantic fingerprint includes the class `reason_code`; the observation fingerprint preserves transport and both lookup matches. Main and alert workers both use owner/token/expiry+version fencing, the same exact retry/dead-letter schedule, and restart-safe reclaim. Do not store raw body after extraction/hash. Provider follow-up timeout/429/5xx is retryable, never eligibility success.

## End-to-End Test Plan

Run IB4-E01..E11 with signed fixtures, real Postgres, process restart/crash boundaries, cache global-barrier assertion, product-membership query, old-JWT bound, main/alert worker lease fencing and exact backoff/dead-letter schedule, and all four collision classes. Fresh/upgrade/down migration proof covers IB4-owned receipt, collision event/observation, security-alert, revocation, and audit tables; exact FK/delete actions, unique/lookup/claim/reclaim indexes, append-only evidence, lease constraints, and 400-day retention. Prove exact-duplicate classification, reason-bound semantic fingerprints, immutable lookup evidence, concurrent identical observation collapsing to one event/observation/alert, no receipt overwrite, and no authority effect. Include the log/trace secret/PII scan. A live Stytch webhook test remains a later BLOCKED IB8 gate.

## Anti-Cheating Audit

No JSON reserialization before verify, `VerifyIgnoringTimestamp`, effect-before-receipt, in-memory dedupe, raw payload storage/logging, fake targeted cache index, webhook provisioning, immediate offline-JWT revoke claim, or silently accepted unknown catalog event.

## Completion Gate

- [ ] IB4-S* and IB4-E01..E11 green.
- [ ] IB4-owned receipt/collision/alert/revocation/audit migration, index, lease, delete-action, and retention gates pass.
- [ ] Event fixtures match configured official catalog evidence.
- [ ] Production BFF/MCP hard gate may advance only after fresh exact-tip spec/security approval.
