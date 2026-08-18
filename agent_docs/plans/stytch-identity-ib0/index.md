# Stytch Identity IB0 — reviewed IB1–IB8 contract freeze

**Status: PASS — independently reviewed design freeze at `4bfd6d5c03412d134a635c32279ef37b1c4e0d9e` with 0 Critical, 0 Important, and 0 Minor findings.** This package remains the **contract authority** for IB1–IB8. Runtime status lives in [`../primer-identity-service/index.md`](../primer-identity-service/index.md) and [`../identity-ib2-resume.md`](../identity-ib2-resume.md): IB1+IB2 credential-free complete (hardening tip `0dd5faa`); next is IB3 after hardening lands on master. Full IB8-E10 browser/webhook remains **BLOCKED**.

## Outcome

This package is the reviewed design authority for the Stytch-backed Primer Identity broker. Its credential-free IB1 implementation authority takes effect only after this review-status commit is merged to `master`; it does not authorize later waves or live-provider work. Stytch B2B authenticates ordinary humans. Primer Identity is the sole Stytch client and the Primer OAuth authorization server (AS). Products and MCP receive only Primer authorization codes, refresh tokens, access JWTs, and JWKS; they never receive or validate a Stytch session token, SessionJWT, callback token, intermediate-session token, provider role, or provider payload.

## Authority and scope

Authoritative repository inputs:

- [`../primer-stytch-integration.md`](../primer-stytch-integration.md)
- [`../primer-identity-service-design.md`](../primer-identity-service-design.md)
- [`../primer-identity-service/index.md`](../primer-identity-service/index.md) and Phase 04
- [`../curriculum-studio-mcp-design.md`](../curriculum-studio-mcp-design.md)
- [`../curriculum-studio-foundation-crosswalk.md`](../curriculum-studio-foundation-crosswalk.md)
- merged IA-R code at this branch base

Official external facts are cited by exact URL in [the source ledger](./01-decisions-and-sequences.md#verified-external-facts). Values labelled **Primer decision** are local policy, not claims about Stytch or MCP.

In scope: interactive broker, callback ownership, durable OAuth/grant/session model, provisioning posture, ES256 JWT/JWKS, BFF contract, MCP authorization, signed webhook receipt/revocation, service actors, rollout/rollback, requirements and verification gates.

Out of scope: Go/SQL implementation, generated OpenAPI/client artifacts, credentials, live Stytch calls, production deployment, automatic product provisioning, email account merge, Stytch M2M, and sub-15-minute distributed JWT revocation.

## Reviewed design decisions

| ID | Decision |
|---|---|
| IB0-D01 | Product BFFs and MCP clients use Primer Identity as the OAuth AS. Identity hosts the Stytch login/discovery/callback boundary and is the only receiver of one-time Stytch artifacts and session material. |
| IB0-D02 | Every product authorization starts a new broker transaction. No implicit cross-product or Identity SSO initially; durable provider-session/grant lifecycle must be proven first. |
| IB0-D03 | Exact immutable `(provider_project_id, provider_organization_id, provider_member_id)` resolves `accounts.id`; email never links identities. Add bounded `provider_member_session_id` to the internal session snapshot in IB1 and persist only that ID plus tuple/expiry, never the provider token. |
| IB0-D04 | Initial human methods are Stytch B2B Email Magic Link, Email OTP, and SAML/OIDC SSO. Identity owns all callback/exchange endpoints. Local passwords are disabled except audited break-glass/legacy policy. |
| IB0-D05 | Provider roles and organization status are eligibility inputs only. Studio/LMS/TV membership and roles are never auto-created or silently changed. Explicit invite/admin provisioning is the default. |
| IB0-D06 | Primer access tokens are ES256, one audience, `sub=accounts.id`, required signed public `client_id` (`varchar(128)` OAuth client identifier; never an internal UUID; `azp` unused initially), no provider/product roles or organization IDs by default, and expire in at most 15 minutes. Validators bind `iss`/`aud`/`sub`/`scope`/`client_id`. Identity owns issuer and JWKS. |
| IB0-D07 | MCP starts with static/pre-registered clients (no DCR/CIMD intake in IB1–IB8), authorization-code + S256 PKCE for humans, distinct `client_credentials` for services, and Studio protected-resource metadata pointing to Identity. Propose returns no state; only initial and retried calls to the same confirm method participate in MRTR, while non-MRTR Studio UI fallback carries no MCP state. Raw Stytch tokens are rejected. |
| IB0-D08 | Identity alone accepts Stytch/Svix webhooks. It verifies the raw body and signed headers and durably receipts before effects. Exact duplicate is only when `provider_event_id` and `svix_message_id` both resolve to the same receipt and the body hash matches. One-ID re-pair (event ID reused with a new Svix ID, or Svix ID reused with a new event ID, even when the hash matches), IDs that resolve to different rows, and body mismatch are distinct quarantines with deterministic fingerprints; they never apply effects. Unordered valid events apply `InvalidateAll` and revoke associated local grants/families. |
| IB0-D09 | Transient provider timeout/429/5xx is unavailable and not negative-cached. Definitive invalid/revoked/expired is denial. Broker/code/refresh replay fails closed. No stale success after provider expiry. |
| IB0-D10 | OpenAPI is owned by Identity HTTP implementation and generated from/checked against handlers after IB0. This candidate records prose/wire contracts only and deliberately emits no implementation or generated artifact. |
| IB0-D11 | Clean JWKS donor `53b693cc93c8cccb15109d90bae90811029af893` is evidence only. It is not merged or reviewed for IB2 and cannot be cherry-picked wholesale because it branched before IA-R. |
| IB0-D12 | Credential-free loopback/fake boundaries may prove local contracts; they never satisfy the live Stytch gate. Production remains fail-closed and BLOCKED until IB1–IB8 and live-provider evidence pass. |

## Files

| File | Purpose |
|---|---|
| [01 — decisions and sequences](./01-decisions-and-sequences.md) | authority, verified facts, sequence diagrams, trust and threat model |
| [02 — HTTP/OAuth/BFF/MCP](./02-http-oauth-bff-mcp-contract.md) | exact endpoints, requests, responses, errors, cookies, redirect and MCP rules |
| [03 — data and state](./03-data-state-and-revocation.md) | additive tables, fields, keys, lifetimes, CAS/idempotency/replay and retention |
| [04 — Stytch webhook/provisioning](./04-stytch-webhook-and-provisioning.md) | human methods, provider callback, event envelope, signature and revocation effects |
| [05 — verification and rollout](./05-verification-rollout-traceability.md) | BDD/E2E traceability, anti-cheat, waves, migration, rollback, STOP/PROCEED gate |
| [JWKS donor evidence](./jwks-donor-evidence.md) | read-only keep/reshape/drop assessment at exact donor tip |

## IB1–IB8 dependency order

| Wave | Contracted result | Hard dependency |
|---|---|---|
| IB1 | callback ownership, exact tuple mapping, recoverable state terminalization, provider-session association, and one-use code **issuance only** | IB0 |
| IB2 | public token endpoint, code consume/replay, ES256 JWT/JWKS and single-audience token response | IB1; selective donor intake only after rebase/reimplementation review |
| IB3 | product BFF host-only cookie, CSRF, state and PKCE | IB2 |
| IB4 | signed webhook ledger, `InvalidateAll`, provider-associated grant/family revocation | IB1–IB3; hard production BFF/MCP gate |
| IB5 | Primer service principals and `client_credentials` | IB2 |
| IB6 | rotating refresh families, reuse detection, logout/revoke lifecycle | IB3–IB4 |
| IB7 | signing-key rotation, hardening and recovery | IB2 and IB4 |
| IB8 | Studio/MCP/LMS/TV adapters, dual-run migration and live-provider gate | IB3–IB7 |

## Completion rule

The independent design review confirmed that linked files are internally consistent, `REQ-IB0-*` mappings and LikeC4 mechanical checks are present, and the 40-file docs/architecture diff is clean. This PASS freezes design only and does **not** claim IB1–IB8 implemented or live Stytch verified.
