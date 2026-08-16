# Primer Identity service design — Stytch-backed human-auth broker

## 1. Decision and scope

The selected production architecture is **Stytch B2B upstream of Primer Identity**. Stytch is the authoritative ordinary-human authentication and session-validity service. Primer Identity is the only Stytch SDK/API client and is Primer’s downstream OAuth/token broker and authorization server for product BFFs and MCP.

Identity accepts an opaque Stytch session only at its server-side broker boundary, validates it with Stytch, resolves exact immutable `(project_id, organization_id, member_id)` to `accounts.id`, creates/retrieves a Primer grant, and mints a short-lived ES256 Primer JWT for exactly one audience. Its JWKS is the only token-verification dependency exposed to products. Stytch SessionJWTs/session tokens never reach browser JS, Studio, LMS, TV, MCP, or product APIs.

No alternate production human RP is selected, and this design does not create a parallel self-hosted password authority. Local password is disabled by default and is allowed only as explicitly gated break-glass/legacy migration. Stytch M2M is deferred; Identity owns local service principals and `client_credentials`.

## 2. Ownership and authorization precedence

| Concern | Authority | Explicit non-authority |
|---|---|---|
| Human authentication/session validity | Stytch B2B | Identity, products, browser JS |
| Stytch session validation, tuple mapping, Primer grant/JWT/JWKS | Primer Identity | Studio/LMS/TV/MCP |
| Studio tenant/workspace membership and RBAC | Curriculum Studio | Stytch organization/member roles, JWT roles |
| LMS educator authorization | LMS | Stytch roles |
| TV device authentication | TV | Stytch/Identity human login |
| Machine principals and `identity:svc:<id>` | Primer Identity | Stytch M2M |

Stytch roles are bounded eligibility hints only. A Stytch organization never auto-creates or maps to a Studio workspace, and roles never enter JWT workspace claims. Provisioning is explicit invite/admin only. Distinct Stytch tuples remain distinct accounts/personas—even if an email or member-looking field is similar—without email merge or automatic cross-org linking.

## 3. Broker and BFF protocol boundary

1. A product BFF begins the **Identity-hosted Stytch broker/callback** flow. Redirect URI, state, PKCE and CSRF/cookie binders are frozen in IB0.
2. Identity alone exchanges/validates the opaque Stytch session, invokes bounded cache/adapter behavior, and maps the tuple locally.
3. Identity returns only Primer authorization-code/session/JWT material to the BFF. Product-owned host-only cookies and CSRF remain on the product host. No product API or browser script receives a Stytch credential.
4. The BFF supplies a single-audience Primer JWT to its API. Studio/LMS/TV/MCP validate Primer signature, issuer, expiry, audience and scope through Primer JWKS, then independently authorize local memberships/roles.

A Primer human JWT contains local subject identity (for example `identity:<account-uuid>` at product boundaries), issuer/audience/expiry and approved scopes; it contains no raw Stytch token, SessionJWT, tuple, provider role, or workspace role. The default access-token expiry is **≤15 minutes** and cannot exceed validated provider-session constraints.

## 4. Revocation, outage and audit

Revocation is two-plane. Plane one is Stytch-session validity plus bounded cache behavior: transient 5xx/429/timeout/transport errors are **unavailable** and are not negative cached. Plane two is local Primer grant plus BFF refresh/access-JWT lifecycle. IB4 adds a signed Stytch webhook receiver with signature and timestamp validation, replay protection, idempotency and out-of-order behavior. It stores a durable provider-session/grant association **without a raw Stytch token**, invalidates validation cache and revokes related Primer grants.

Webhooks cannot grant or change Studio/LMS membership. The guaranteed offline bound is the ≤15-minute access-token expiry; immediate revocation shorter than that is explicitly deferred until a tested local/replicated `sid`/`jti` mechanism exists. Logs and audit may contain sanitized correlation and decision metadata, never raw tokens or provider payloads.

## 5. Current status and remediation

IA at `87d5c215134825edb410266a62c15534e1e9ecea` implemented config, official adapter, cache and mapping foundation. It is neither composed nor production-approved: no exchange, JWT/JWKS, BFF, webhook, app wiring or live proof exists.

Before IB work, IA-R must explicitly enforce Stytch enabled + live environment + credentials + no production override; isolate per-token invalidation/cancellation (global fence only for `InvalidateAll`); document omitted `session_duration_minutes`; remove duplicate project/org tuple index; correct `golang.org/x/sync` tidy classification; and enforce cache-only or capped direct-adapter tokens plus CSPRNG per-process HMAC lifecycle. Fresh specification and quality review are required.

## 6. Delivery sequence and acceptance matrix

| Wave | Outcome | Production gate |
|---|---|---|
| IA / IA-R | foundation then residual closure/review | not production auth |
| IB0–IB1 | frozen broker contract and composed validated exchange | no product token yet |
| IB2–IB3 | Primer ES256/JWKS bridge and BFF contract | no production until webhook |
| IB4 | signed webhook/revocation | hard dependency for production BFF/MCP |
| IB5–IB7 | service principals, lifecycle, hardening | bounded machine and security proof |
| IB8 | Studio/LMS/TV cutover, admin/audit/live proof | all local authorization E2Es green |

Required E2Es: valid mapped tuple reaches local membership; valid token with no membership is denied; cross-org same-email personas remain distinct; Stytch admin-like role cannot authorize locally; human/service classes are separate; Studio/MCP reject raw Stytch token; transient outage fails closed/no negative cache; webhook forgery/replay/dedupe/out-of-order is safe; LMS dual-run still enforces local roles; audit/log output contains no token/provider payload.
