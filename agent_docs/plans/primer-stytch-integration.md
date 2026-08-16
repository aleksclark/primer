# Primer Stytch Integration — reconciled implementation record

**IB0 status: PASS — independently reviewed design freeze at `4bfd6d5c03412d134a635c32279ef37b1c4e0d9e`; 0 Critical, 0 Important, and 0 Minor findings.** Credential-free IB1 may PROCEED only after this review-status commit is merged to `master`; live Stytch remains **BLOCKED**.

**Status:** **IA-R reviewed at code tip `8623ee639bd64d40f819d3079c52ed567b65b27b` (`fix(identity): require namespaced configuration`): specification PASS: 0 Critical/Important; quality/security APPROVED: 0 Critical/Important. Credential-free/library foundation complete; **not** composed production authentication; live Stytch still **BLOCKED**.** The original IA foundation remains at `87d5c215134825edb410266a62c15534e1e9ecea`.

## Selected production architecture

**Stytch B2B is the upstream authority for human authentication and session validity.** Primer Identity is the sole Stytch SDK/API client and the downstream Primer OAuth/token broker and authorization server for product BFFs and MCP. Identity validates an opaque Stytch session server-side, resolves the exact immutable `(project_id, organization_id, member_id)` tuple to `accounts.id`, and mints short-lived, single-audience Primer ES256 JWTs discoverable through Primer JWKS.

Stytch session tokens and SessionJWTs never reach Studio, LMS, TV, MCP, product APIs, or browser JavaScript. Products validate only Primer JWTs/JWKS. Product host-only cookies and CSRF remain product-owned; a product BFF uses the Identity-hosted Stytch broker/callback and receives only Primer authorization-code/session/JWT material.

- Stytch provides ordinary human auth. Local password is disabled by default and permitted only as explicit break-glass or legacy-migration policy; it is not a parallel human identity authority.
- Stytch organization/member roles are bounded eligibility hints only. They never become product authorization, JWT workspace roles, or automatic Studio tenancy. Studio workspace membership, LMS educator roles, and TV device authentication remain their respective local systems of record.
- Provisioning is explicit invite/admin only. A Stytch organization is not a Studio workspace. Distinct Stytch tuples remain distinct Primer accounts/personas: no email merge or automatic cross-organization linking.
- Primer Identity owns service principals, `client_credentials`, and JWKS. `identity:svc:<primer-service-principal-id>` is local service identity; Stytch M2M is deferred and must not be assumed.

## IA implementation evidence and residuals

Implemented foundation: fail-closed optional configuration, the official Stytch B2B SDK adapter, bounded HMAC-keyed validation cache, normalized `stytch_mappings` tuple mapping, and focused tests. It has **no** application/API wiring, exchange, Primer JWT/JWKS issuance, BFF, webhook, or live-provider proof.

The official SDK authenticate request must **omit** `session_duration_minutes`; it must not claim an explicit zero value as the selected contract.

### Required IA-R remediation before IB work

1. **Production config policy:** selected policy is Stytch mandatory in production. Code must explicitly require `Enabled=true`, live environment, configured credentials, and no override, with tests. Indirect rejection of disabled Stytch is insufficient.
2. **Cancellation isolation:** per-token `Invalidate(token)` must not globally fence unrelated in-flight validations. Implement isolated per-token cancellation/epoch; reserve global fencing for `InvalidateAll`, with a concurrency regression test.
3. **Cleanup:** correct SDK documentation to state `session_duration_minutes` is omitted; remove the duplicate project/org tuple index before deploy; make the `golang.org/x/sync` direct/indirect tidy classification correct.
4. **Boundary gates:** use cache-only invocation or cap direct adapter tokens; use a CSPRNG per-process HMAC key with defined lifecycle; classify transient provider failure as unavailable and do not cache it; retain live-provider testing as an explicit blocked gate.

## Two-plane revocation and non-goals

Revocation has two planes: (1) Stytch session validity and bounded validation cache; (2) Primer grants plus product BFF refresh/access-JWT lifecycle. IB4 must accept signed provider webhooks with signature, timestamp, replay and idempotency checks; retain a durable provider-session/grant association without storing a raw Stytch token; invalidate cache and revoke Primer grants. Access JWT expiry defaults to **≤15 minutes**. Webhooks do not grant or change Studio/LMS membership. Immediate revocation shorter than JWT TTL is deferred until a tested local/replicated `sid`/`jti` mechanism exists.

Not in this work: direct product-Stytch integration, roles as product authorization, automatic organization-to-workspace provisioning, Stytch M2M, raw token persistence/logging, or generated API/schema changes.

## Reconciled roadmap

| Wave | Status | Required outcome / dependency |
|---|---|---|
| IA | implemented; credential-free/library foundation complete, not production auth | I1/I2 Stytch config, adapter/cache and exact tuple mapping foundation |
| IA-R | reviewed at `8623ee639bd64d40f819d3079c52ed567b65b27b`; specification PASS: 0 Critical/Important; quality/security APPROVED: 0 Critical/Important | close config, invalidation, dependency/index residuals; fresh spec + quality review |
| IB0 | **independently reviewed design freeze; PASS with no runtime implementation** | exact Identity-hosted interactive broker/redirect, webhook event contract, durable grant association, local break-glass policy, organization provisioning policy, Primer OAuth AS; see [`stytch-identity-ib0/`](./stytch-identity-ib0/) |
| IB1 | post-merge credential-free implementation cursor | compose Stytch client/cache/mapping into Identity broker/exchange; Primer-only grant association |
| IB2 | planned | ES256/JWKS single-audience Primer token bridge bound to validated mapped Stytch session |
| IB3 | planned | product BFF contract, host-only cookie/CSRF/PKCE, Primer-only material |
| IB4 | planned hard gate | signed webhook, cache/grant revoke, replay/idempotency; production BFF/MCP dependency |
| IB5 | planned | Primer-owned service principals and `client_credentials` |
| IB6 | planned | refresh/logout/provider-plus-local lifecycle |
| IB7 | planned | Primer signing-key rotation and hardening |
| IB8 | planned | admin/audit/recovery, LMS/TV and live Stytch cutover |

**Post-merge roadmap cursor:** **IB1 credential-free implementation**, only after this review-status commit is merged to `master`. No IB1–IB8 runtime is implemented. Live Stytch remains **BLOCKED**.

## IB0 reviewed design contract

[`stytch-identity-ib0/`](./stytch-identity-ib0/) records the reviewed `/oauth/authorize` → Identity-owned Stytch callback → one-use Primer code flow, no implicit cross-product SSO, additive OAuth/grant/refresh/webhook tables, bounded provider `member_session_id`, ES256 token profile, product BFF and MCP protected-resource contract, Svix verification/receipt rules, rollout/rollback, traceability, and the read-only JWKS donor assessment. Official facts and Primer-selected policy are distinguished there with exact source URLs. The independent review covered 40 docs/architecture files; links, 58 planned E2Es, 24 requirements, and LikeC4 topology passed. This design freeze does not claim IB1–IB8 implemented or change the live-provider status.

## Required evidence gates

The plan may claim production human auth only after: IA-R review is green; live Stytch credentials/project validation succeeds without overrides; IB1–IB4 public-path E2Es pass; signed webhook revocation evidence exists; and Studio/MCP accept only Primer tokens. Required anti-cheat cases include raw Stytch token rejection, provider outage fail-closed without negative caching, token/payload-free audit logs, and no direct Stytch-to-product/database topology.
