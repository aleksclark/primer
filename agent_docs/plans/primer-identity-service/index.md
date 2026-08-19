# Primer Identity — Stytch-backed broker delivery plan

**IB2/I6 status: COMPLETE — dual-reviewed credential-free tip `f5d5b5b372dc6988a65d082f948b96e7de9f432b` (`fix(identity): close IB2 grant metadata and assertion retention`), plus opt-in live test-project harness at `e8ecd183b9354bcf9b200f2c1ace81ee1bdb47b5`.** Spec PASS 0C/0I (1 minor); quality/security APPROVED 0C/0I. Integrated via `impl/IB2-hardening-remed` (supersedes master tip `f31559db92445826aabd98bbc0903a22d80d6e80` from PR #24). **IB3/I7 is complete on master via PR #26 (`81b3302`):** the credential-free product BFF package has exact static registration, server-side PKCE code exchange, host-only product cookies, CSRF/Origin checks, and no cross-product cookie issuance. The residual real-browser IB3-E01..E06 process/artifact proof remains open and must not be inferred from package tests. The reviewed IB0 design freeze remains the contract authority. Full IB8-E10 browser + webhook live proof remains **BLOCKED**.

## Outcome and current state

Primer Identity is the **sole Stytch B2B client** and the downstream Primer token broker/authorization server. Stytch owns ordinary human authentication and session validity. Identity server-validates opaque sessions, resolves exact `(project_id, organization_id, member_id)` to `accounts.id`, then issues short-lived single-audience Primer JWTs/JWKS with a required signed public OAuth `client_id` to product BFFs and MCP. Product authorization remains local.

At `87d5c215134825edb410266a62c15534e1e9ecea`, IA library foundation exists: Stytch config, official adapter, bounded cache and normalized mapping. IA-R was reviewed at code tip `8623ee639bd64d40f819d3079c52ed567b65b27b`. IB1 is complete and reviewed at `59a3998208ba9ef87dfe0bf4a913eedab3753ef8`. IB2 is complete on master at `f31559d` and **superseded for integration** by the dual-reviewed hardening tip above (migrations `00006`–`00009`, host-rooted token/revoke audiences, assertion retention `exp+60s`, exact RFC8414 grant inventory, fail-closed human `ClientLookup`, exact `invalid_client` wire, auth-code field sets, OpenAPI/client parity). Coverage 82.4% (≥80%). Parent gates green on the hardening tip. Opt-in `make identity-live-stytch` qualifies the production adapter against the Stytch **test** project only; it is **not** IB8-E10.

**Cursor:** **IB5 / I9 Primer-owned service principals and `client_credentials`**. IB4 implementation is merged as PR #37 (`b2f4ae4`), but its fresh exact-tip review remains pending; no production BFF/MCP claim. The IB0 exact-tip review passed at `4bfd6d5c03412d134a635c32279ef37b1c4e0d9e`; live Stytch and IB8-E10 remain BLOCKED.

**Post-merge roadmap cursor:** **IB5 / I9 service principals and `client_credentials`**. I7's credential-free BFF contract and the IB4 implementation are present, but production BFF/MCP authorization remains blocked until the fresh IB4 exact-tip specification/security gate passes. Live Stytch browser/webhook proof remains **BLOCKED**.

## Non-negotiable boundaries

1. Only Identity calls Stytch. Stytch SessionJWT/session tokens never enter Studio, LMS, TV, MCP, product APIs, or browser JavaScript.
2. Stytch tuple mapping is exact and private to Identity. Same-email or same-member cross-organization tuples remain distinct Primer personas. No email merge or automatic linking.
3. Stytch roles are eligibility hints, never token workspace roles or product authorization. Studio membership, LMS educator role, and TV device auth remain local SoT. No Stytch org auto-provisions a Studio tenant/workspace.
4. Product BFFs retain host-only cookies/CSRF and receive only Primer code/session/JWT material from the Identity broker.
5. Identity owns Primer service principals and `client_credentials`; `identity:svc:<id>` is local. Stytch M2M is deferred.
6. Revocation is two-plane: Stytch validity/cache plus Primer grant/BFF/JWT lifecycle. Webhooks do not alter product membership. Default access-token expiry is ≤15m; sub-TTL immediate revoke is deferred pending tested sid/jti machinery.
7. Provider-session associations are account-bound by composite database constraints. Signing custody permits at most one `active` and one `next` key and enforces ordered material destruction; neither invariant is application-only.

## Phase overview

**Historical filename note:** linked phase filenames remain stable for link compatibility; their old Google/JWKS/service-principal slugs are non-authoritative. The headings and labels in this table are authoritative.

| Phase | Goal | Depends on |
|---|---|---|
| [01: IA / service shell and Stytch foundation](./phase-01-service-shell-and-db.md) | Implemented foundation: configuration, adapter/cache, and only the bounded provider boundary. | IA-R config remediation and fresh review. |
| [02: IA / accounts and Stytch tuple mappings](./phase-02-accounts-and-external-identities.md) | Implemented tuple mapping foundation with exact distinct persona semantics. | IA-R index cleanup and fresh review. |
| [03: IA-R / residual remediation and review](./phase-03-keys-jwks-access-tokens.md) | Close the blocking foundation residuals before composition. **Status:** reviewed at `8623ee639bd64d40f819d3079c52ed567b65b27b`: specification PASS: 0 Critical/Important; quality/security APPROVED: 0 Critical/Important; credential-free/library foundation complete; **not** composed production authentication; live Stytch still **BLOCKED**. | Fresh quality and specification approval. |
| [04: IB0 / reviewed broker, webhook, and provisioning design freeze](./phase-04-oauth-clients-and-op.md) | Independently reviewed docs/architecture **PASS** at the recorded design tip; no runtime implementation. | Credential-free IB1 may start only after the review-status commit reaches `master`; [`../stytch-identity-ib0/`](../stytch-identity-ib0/). |
| [05: IB1 / compose validated Stytch broker exchange](./phase-05-sessions-cookies-login-csrf.md) | **Complete and reviewed** at code tip `59a3998208ba9ef87dfe0bf4a913eedab3753ef8`: credential-free callback/state/exact unpaginated `Sessions.Get` adapter/mapping/code-issuance composition with 1 MiB/256 bounds and account-bound associations; public token consume/JWT/JWKS waits for IB2. | Reviewed IB0 design freeze; post-merge status gate. |
| [06: IB2 / Primer ES256 JWT and JWKS bridge](./phase-06-google-rp-loopback.md) | **Complete and dual-reviewed** at hardening tip `f5d5b5b` (supersedes master `f31559d`). | IB1. |
| [07: IB3 / product BFF cookie, CSRF, and PKCE contract](./phase-07-product-bff-contract.md) | **Complete on master via PR #26 (`81b3302`)** for the credential-free BFF package; real-browser IB3-E01..E06 evidence remains residual. | IB2 and IB0 contract. |
| [08: IB4 / signed webhook and two-plane revocation](./phase-08-service-principals.md) | **Merged as PR #37 (`b2f4ae4`); fresh exact-tip review remains pending.** Make provider invalidation and local grants durable and replay safe, with four reason-bound collision classes and one restart-safe alert. | IB1–IB3; hard gate for production BFF/MCP. |
| [09: IB5 / Primer-owned service principals](./phase-09-refresh-revoke-logout.md) | **Implementation present at `ce3c17c`; fresh exact-tip security review pending.** Add bounded local machine credentials and `client_credentials`. | IB2. |
| [10: IB6 / refresh, logout, and lifecycle](./phase-10-key-rotation-and-hardening.md) | Complete Provider-plus-Primer lifecycle with no parallel human authority. | IB3–IB4. |
| [11: IB7 / key rotation and hardening](./phase-11-admin-audit-recovery.md) | Harden Primer-issued token verification and enforce unique next/active plus ordered retirement/destruction custody. | IB2/IB4. |
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
| Exact unpaginated `Sessions.Get`; 1 MiB/256; exact/duplicate behavior | IB1 |
| Public `client_id` required; `azp`/internal UUID denied; MRTR bound to claim | IB2, IB8 Studio/MCP |
| Signed webhook dedupe/replay/forgery/out-of-order + four collision classes | IB4 |
| Composite provider-association/account FKs reject cross-account writes | IB1 |
| Initial signing-key/refresh/code-exchange schema and copied issuance evidence | IB2 |
| Full next/active/retired/destroyed signing-key rotation and material destruction | IB7 |
| Webhook receipt/collision/security-alert worker schema, leases, and retention | IB4 |
| Refresh rotation/reuse terminal lifecycle and long-lived retention | IB6 |
| Explicit revocation bound and local-role LMS dual run | IB4, IB6, IB8 LMS |
| Logs/audit contain no tokens/provider payload | IB4, IB7, IB8 |

Production BFF and MCP authorization are blocked on IB4, not historical Google/I3/I7 labels. Every phase requires public-boundary E2E, phase-specific anti-cheat review, link/heading validation, and no claim that a fake/loopback provider proves live Stytch behavior.
