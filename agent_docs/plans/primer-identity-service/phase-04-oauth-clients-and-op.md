# 04: IB0 / reviewed broker, webhook, and provisioning design freeze

**Status: PASS — independently reviewed design freeze at `4bfd6d5c03412d134a635c32279ef37b1c4e0d9e`; 0 Critical, 0 Important, and 0 Minor findings.** Credential-free IB1 may PROCEED only after this review-status commit is merged to `master`; live Stytch remains **BLOCKED**.

## Goal

Produce one reviewed design authority for IB1–IB8 before any endpoint/schema work. The package is [`../stytch-identity-ib0/`](../stytch-identity-ib0/) and is frozen by the independent zero-finding review recorded above. Its post-merge PROCEED applies only to credential-free IB1; later waves retain their own gates. It owns exact OAuth/BFF/MCP endpoints, tables, state machines, Stytch callback/webhook rules, failure semantics, rollout, traceability, and JWKS donor intake. IA-R at this branch base remains credential-free foundation only; live Stytch remains **BLOCKED**.

## BDD Success Criteria

#### Scenario: IB0-P04-S1 — Complete broker contract

- **Given** the IA-R adapter/cache/tuple foundation
- **When** an IB1 implementer follows the IB0 package
- **Then** every authorize/callback/token transition, field, lifetime, error and transaction boundary is specified without redesign
- **And** Identity alone receives Stytch one-time/session artifacts
- **And** products receive only Primer material.

#### Scenario: IB0-P04-S2 — Complete revocation and provisioning contract

- **Given** Stytch/Svix verified source facts and unordered provider events
- **When** an IB4 implementer follows the package
- **Then** signature headers, raw-body/skew/idempotency/receipt/effect behavior are exact
- **And** dual-ID/hash collisions are fingerprint-idempotent immutable evidence with a separately leased/retryable alert item and no authority effect
- **And** intended member/organization family categories remain disabled until committed official Dashboard/catalog fixtures qualify exact identifiers, schemas, and update eligibility mappings
- **And** `InvalidateAll` plus provider-associated grant/family revocation is the initial safe reaction
- **And** no webhook or provider role mutates product membership.

#### Scenario: IB0-P04-S3 — Traceable downstream contract

- **Given** IB1–IB8, Studio BFF/MCP, and the JWKS donor
- **When** the plans/architecture are reviewed
- **Then** each requirement maps to BDD/E2E evidence and a dependency wave
- **And** the donor is marked read-only/not merged/not reviewed
- **And** live provider proof remains blocked.

## Implementation Instructions

1. Treat [`../stytch-identity-ib0/index.md`](../stytch-identity-ib0/index.md) as the IB0 decision record.
2. IB1 migration gates cover only broker transactions/sealed state, provider-session association tuple/account integrity, human grants and composite account/association FKs, authorization-code issuance, and their CAS/unique/index/down/upgrade behavior; add bounded `provider_member_session_id` to the internal Stytch snapshot and never persist raw provider material.
3. IB2 owns strict ES256 JWT/JWKS after IB1, the initial signing-key and refresh-family/current-token/code-exchange schema, and copied issuance evidence with nullable `ON DELETE SET NULL`; it may selectively reimplement/rebase only approved donor pieces from [`../stytch-identity-ib0/jwks-donor-evidence.md`](../stytch-identity-ib0/jwks-donor-evidence.md).
4. IB3 owns product host-only cookie/CSRF/state/PKCE; IB4 owns signed webhook/two-plane revocation plus receipt/collision/security-alert worker schema, indexes, leases, and retention, and is the production BFF/MCP hard gate.
5. IB5 service principals remain Primer-owned; IB6 owns refresh rotation/reuse, terminal lifecycle, and long-lived retention; IB7 owns full signing-key next/active/retired/destroyed rotation; IB8 owns product/MCP migration/live proof.
6. OpenAPI and generated-client parity begin with implementation waves; IB0 creates no code/schema/generated artifacts.

## End-to-End Test Plan

IB0 is docs/architecture-only. Run the link/heading/requirement/E2E traceability audit, LikeC4 1.46 validate/build/export with positive/negative edge assertions, docs/architecture allowlist, and `git diff --check`. Runtime E2Es are assigned in [`../stytch-identity-ib0/05-verification-rollout-traceability.md`](../stytch-identity-ib0/05-verification-rollout-traceability.md); fake provider evidence never satisfies live Stytch.

## Anti-Cheating Audit

Reject TBDs, invented Stytch session-event names, direct product→Stytch edges, provider roles as product roles, browser token storage, in-memory persistence substitutes, immediate-JWT-revocation claims, generated/code artifacts, wholesale donor intake, or any status text calling IB1–IB8/live Stytch implemented.

## Completion Gate

- [x] All files in the IB0 package resolve and distinguish verified facts from Primer policy.
- [x] `REQ-IB0-*` → `IB0-S*` → future `IB*-E*` traceability is complete.
- [x] Phase 05–09 and Studio/MCP/delivery/crosswalk dependencies agree.
- [x] LikeC4 required positive edges render and prohibited Stytch edges are absent.
- [x] Docs/architecture-only allowlist and `git diff --check` pass.
- [x] Credential-free IB1 remains blocked until this reviewed status is merged to `master`; later/live gates remain blocked.
