# Primer Identity service design — Stytch-backed human-auth broker

**IB0 status: STOP — candidate remediation under independent exact-tip review; no broker/token implementation is authorized by this reference.**

## 1. Decision and scope

The selected production architecture is **Stytch B2B upstream of Primer Identity**. Stytch is the authoritative ordinary-human authentication and session-validity service. Primer Identity is the only Stytch SDK/API client and is Primer’s downstream OAuth/token broker and authorization server for product BFFs and MCP.

Identity accepts an opaque Stytch session only at its server-side broker boundary, validates it with Stytch, resolves exact immutable `(project_id, organization_id, member_id)` to `accounts.id`, creates/retrieves a Primer grant, and mints a short-lived ES256 Primer JWT for exactly one audience with the required signed public OAuth `client_id`. Its JWKS is the only token-verification dependency exposed to products. Stytch SessionJWTs/session tokens never reach browser JS, Studio, LMS, TV, MCP, or product APIs.

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

1. A product BFF begins `GET /oauth/authorize` at Primer Identity with an exact registered client/redirect/resource/audience/scope tuple, state, and S256 PKCE. The candidate contract is [`stytch-identity-ib0/`](./stytch-identity-ib0/) and becomes implementation authority only after its zero-finding exact-tip review.
2. Identity alone exchanges/validates the opaque Stytch session, then revalidates the exact member session with exactly one unpaginated Stytch Go v18.1.0 `Sessions.Get(OrganizationID, MemberID)` call. The adapter caps the body at 1 MiB and returned sessions at 256, requires exactly one byte-equal active/unexpired stored session ID, denies missing/inactive/expired, and treats duplicate/overflow/malformed/tuple mismatch or transient provider failure as unavailable/fail-closed.
3. Identity creates a durable Primer grant and one-use 60-second Primer authorization code. The BFF exchanges it server-to-server and receives only Primer access/rotating refresh material. Product-owned host-only cookies and CSRF remain on the product host. No product API or browser script receives a Stytch credential.
4. The BFF supplies a single-audience Primer JWT to its API. Studio/LMS/TV/MCP validate Primer signature, issuer, expiry, audience, scope, and required public `client_id` through Primer JWKS, reject `azp` and internal-UUID client identifiers, then independently authorize local memberships/roles.

A Primer human JWT uses the canonical string form of `accounts.id` as `sub`; products map that UUID to their local opaque `subject_ref` convention (for example `identity:<uuid>`). It contains issuer/audience/expiry, approved scopes, and required signed `client_id=oauth_clients.client_id`; `azp` is absent and the internal OAuth-client UUID is never public. It contains no raw Stytch token, SessionJWT, tuple, provider role, or workspace role. The default access-token expiry is **≤15 minutes** and cannot exceed validated provider-session constraints.

Each product authorization starts a new broker transaction; implicit Identity/cross-product SSO is deferred. IB1 adds bounded provider `member_session_id` to the internal snapshot and persists it with tuple/expiry solely for grant and webhook correlation, never the raw provider token. Exact endpoint, data, state, webhook, MCP, and rollback contracts live in the IB0 package.

## 4. Revocation, outage and audit

Revocation is two-plane. Plane one is Stytch-session validity plus bounded cache behavior: transient 5xx/429/timeout/transport errors are **unavailable** and are not negative cached. Plane two is local Primer grant plus BFF refresh/access-JWT lifecycle. IB4 adds a signed Stytch webhook receiver with signature and timestamp validation, replay protection, idempotency and out-of-order behavior. Exact duplicate means both provider-event and Svix IDs resolve to one receipt with the same body hash; event-ID re-pair, Svix-ID re-pair, cross-row ID collision, and body mismatch are separate reason-bound fingerprint classes that produce one immutable security event/observation/alert and no authority effect. It stores a durable provider-session/grant association **without a raw Stytch token**, constrained by composite association/account FKs, invalidates validation cache and revokes related Primer grants.

Webhooks cannot grant or change Studio/LMS membership. The guaranteed offline bound is the ≤15-minute access-token expiry; immediate revocation shorter than that is explicitly deferred until a tested local/replicated `sid`/`jti` mechanism exists. Logs and audit may contain sanitized correlation and decision metadata, never raw tokens or provider payloads.

## 5. Current status and remediation

IA at `87d5c215134825edb410266a62c15534e1e9ecea` implemented config, official adapter, cache and mapping foundation. It is neither composed nor production-approved: no exchange, JWT/JWKS, BFF, webhook, app wiring or live proof exists.

Before IB work, IA-R must explicitly enforce Stytch enabled + live environment + credentials + no production override; isolate per-token invalidation/cancellation (global fence only for `InvalidateAll`); document omitted `session_duration_minutes`; remove duplicate project/org tuple index; correct `golang.org/x/sync` tidy classification; and enforce cache-only or capped direct-adapter tokens plus CSPRNG per-process HMAC lifecycle. Fresh specification and quality review are required.

## 6. Delivery sequence and acceptance matrix

| Wave | Outcome | Production gate |
|---|---|---|
| IA / IA-R | foundation then residual closure/review | not production auth |
| IB0–IB1 | candidate broker contract review and later composed validated exchange | no product token yet |
| IB2–IB3 | Primer ES256/JWKS bridge and BFF contract | no production until webhook |
| IB4 | signed webhook/revocation | hard dependency for production BFF/MCP |
| IB5–IB7 | service principals, lifecycle, hardening | bounded machine and security proof |
| IB8 | Studio/LMS/TV cutover, admin/audit/live proof | all local authorization E2Es green |

Required E2Es: exact unpaginated Stytch session revalidation obeys 1 MiB/256 and duplicate/exact-ID semantics; wrong-account association/grant writes fail; valid mapped tuple reaches local membership; valid token with no membership is denied; JWTs require public `client_id` and reject `azp`/internal UUIDs; cross-org same-email personas remain distinct; Stytch admin-like role cannot authorize locally; human/service classes are separate; Studio/MCP reject raw Stytch token and bind MRTR confirmation to the validated public `client_id`; transient outage fails closed/no negative cache; all four webhook collision classes are fingerprinted/alerted without authority effect; signing keys allow at most one active and one next and erase private ciphertext on destruction; LMS dual-run still enforces local roles; audit/log output contains no token/provider payload.
