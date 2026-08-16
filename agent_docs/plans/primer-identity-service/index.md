# Primer Identity — Stytch-backed broker delivery plan

## Outcome and current state

Primer Identity is the **sole Stytch B2B client** and the downstream Primer token broker/authorization server. Stytch owns ordinary human authentication and session validity. Identity server-validates opaque sessions, resolves exact `(project_id, organization_id, member_id)` to `accounts.id`, then issues short-lived single-audience Primer JWTs/JWKS to product BFFs and MCP. Product authorization remains local.

At `87d5c215134825edb410266a62c15534e1e9ecea`, IA library foundation exists: Stytch config, official adapter, bounded cache and normalized mapping. IA-R was reviewed at code tip `8623ee639bd64d40f819d3079c52ed567b65b27b` (`fix(identity): require namespaced configuration`): specification PASS: 0 Critical/Important; quality/security APPROVED: 0 Critical/Important. Credential-free/library foundation is complete; **not** composed production authentication; live Stytch still **BLOCKED**. Historical F0/PG1/PG2 work is complete history, not a reason to redispatch I1/I2.

**Roadmap cursor:** **IB0** is now the next dependency-ready wave; **IB1+ remain blocked on IB0**.

## Non-negotiable boundaries

1. Only Identity calls Stytch. Stytch SessionJWT/session tokens never enter Studio, LMS, TV, MCP, product APIs, or browser JavaScript.
2. Stytch tuple mapping is exact and private to Identity. Same-email or same-member cross-organization tuples remain distinct Primer personas. No email merge or automatic linking.
3. Stytch roles are eligibility hints, never token workspace roles or product authorization. Studio membership, LMS educator role, and TV device auth remain local SoT. No Stytch org auto-provisions a Studio tenant/workspace.
4. Product BFFs retain host-only cookies/CSRF and receive only Primer code/session/JWT material from the Identity broker.
5. Identity owns Primer service principals and `client_credentials`; `identity:svc:<id>` is local. Stytch M2M is deferred.
6. Revocation is two-plane: Stytch validity/cache plus Primer grant/BFF/JWT lifecycle. Webhooks do not alter product membership. Default access-token expiry is ≤15m; sub-TTL immediate revoke is deferred pending tested sid/jti machinery.

## Phase overview

**Historical filename note:** linked phase filenames remain stable for link compatibility; their old Google/JWKS/service-principal slugs are non-authoritative. The headings and labels in this table are authoritative.

| Phase | Goal | Depends on |
|---|---|---|
| [01: IA / service shell and Stytch foundation](./phase-01-service-shell-and-db.md) | Implemented foundation: configuration, adapter/cache, and only the bounded provider boundary. | IA-R config remediation and fresh review. |
| [02: IA / accounts and Stytch tuple mappings](./phase-02-accounts-and-external-identities.md) | Implemented tuple mapping foundation with exact distinct persona semantics. | IA-R index cleanup and fresh review. |
| [03: IA-R / residual remediation and review](./phase-03-keys-jwks-access-tokens.md) | Close the blocking foundation residuals before composition. **Status:** reviewed at `8623ee639bd64d40f819d3079c52ed567b65b27b`: specification PASS: 0 Critical/Important; quality/security APPROVED: 0 Critical/Important; credential-free/library foundation complete; **not** composed production authentication; live Stytch still **BLOCKED**. | Fresh quality and specification approval. |
| [04: IB0 / broker, webhook, and provisioning design freeze](./phase-04-oauth-clients-and-op.md) | Freeze the exact external contracts before endpoints are composed. | IA-R approval. |
| [05: IB1 / compose validated Stytch broker exchange](./phase-05-sessions-cookies-login-csrf.md) | Compose adapter/cache/mapping into the Identity public broker/exchange path. | IB0 frozen contract. |
| [06: IB2 / Primer ES256 JWT and JWKS bridge](./phase-06-google-rp-loopback.md) | Issue only Primer token material after the broker exchange. | IB1. |
| [07: IB3 / product BFF cookie, CSRF, and PKCE contract](./phase-07-product-bff-contract.md) | Give each product a safe broker-facing browser contract. | IB2 and IB0 contract. |
| [08: IB4 / signed webhook and two-plane revocation](./phase-08-service-principals.md) | Make provider invalidation and local grants durable and replay safe. | IB1–IB2; hard gate for production BFF/MCP. |
| [09: IB5 / Primer-owned service principals](./phase-09-refresh-revoke-logout.md) | Add bounded local machine credentials. | IB2. |
| [10: IB6 / refresh, logout, and lifecycle](./phase-10-key-rotation-and-hardening.md) | Complete Provider-plus-Primer lifecycle with no parallel human authority. | IB3–IB4. |
| [11: IB7 / key rotation and hardening](./phase-11-admin-audit-recovery.md) | Harden Primer-issued token verification and operational boundaries. | IB2/IB4. |
| [12: IB8 / Studio integration and authorization proof](./phase-12-studio-integration.md) | Cut Studio to validator-only consumption of Primer tokens. | IB2 and IB4 for production. |
| [13: IB8 / LMS transitional cutover](./phase-13-lms-dual-login-and-service-cutover.md) | Migrate LMS without weakening local educator roles. | IB3–IB6. |
| [14: IB8 / TV admin, operations, and live cutover](./phase-14-tv-admin-s7-ops-live.md) | Complete TV admin and operational readiness. | IB4–IB7 and approved live credentials. |

## Traceability and production gates

| Required observable scenario | Phase / proof |
|---|---|
| Valid Stytch tuple → Primer `sub` → local membership | IB1, IB2, IB8 Studio |
| Valid Primer token without membership denied | IB8 Studio/MCP |
| Same email/member cross-org distinct | IA mapping; IB8 integration |
| Stytch admin-like role without local role denied | IB2, IB8 Studio |
| Human/service-principal separation | IB5 |
| Raw Stytch token rejected by Studio/MCP | IB2, IB8 Studio |
| Provider outage fails closed without negative cache | IB1 |
| Signed webhook dedupe/replay/forgery/out-of-order | IB4 |
| Explicit revocation bound and local-role LMS dual run | IB4, IB6, IB8 LMS |
| Logs/audit contain no tokens/provider payload | IB4, IB7, IB8 |

Production BFF and MCP authorization are blocked on IB4, not historical Google/I3/I7 labels. Every phase requires public-boundary E2E, phase-specific anti-cheat review, link/heading validation, and no claim that a fake/loopback provider proves live Stytch behavior.
