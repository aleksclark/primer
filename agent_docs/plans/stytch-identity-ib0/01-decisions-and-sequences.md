# 01 — Candidate decisions, authority, sequences, and threats

**Status: STOP — candidate contract under independent exact-tip review.** Nothing in this package is frozen, dispatch-ready, or a substitute for a fresh zero-finding review.

## Goal

Remove redesign work from IB1–IB8 by recording candidate decisions for who authenticates, who owns each OAuth and browser boundary, what provider data may cross Identity internals, and what failures are safe; these become authoritative only after the zero-finding review gate.

## Verified external facts

The following are **verified external facts**, not Primer policy:

| Fact | Official source |
|---|---|
| `POST /v1/b2b/sessions/authenticate` accepts exactly one `session_token` or `session_jwt`; the response includes `member_session.member_session_id`, `member_id`, `organization_id`, `expires_at`, plus member/organization and fresh Stytch session material. Omitting `session_duration_minutes` avoids extending an existing session. | Stytch Authenticate Session: https://stytch.com/docs/b2b/api/authenticate-session |
| SSO completion exchanges a one-time `sso_token` server-side and can return a full member session or an intermediate session requiring MFA; `member_authenticated` is authoritative for completion. | Stytch Complete SSO Authenticate: https://stytch.com/docs/b2b/api/sso-authenticate |
| Stytch validates requested redirect URLs against environment-specific dashboard values by exact match, including path/query. | Stytch URL Validation: https://stytch.com/docs/b2b/api/url-validation |
| Stytch B2B webhooks are delivered through Svix; payloads carry `project_id`, unique `event_id`, `action`, `object_type`, `source`, entity `id`, and timestamp. Delivery is not ordered; Stytch recommends treating an event as a signal to pull current state when ordering matters. | Stytch B2B Webhooks: https://stytch.com/docs/b2b/guides/webhooks |
| Svix verification requires the unmodified raw body and `svix-id`, `svix-timestamp`, `svix-signature` (or white-labelled `webhook-*`) headers. The Go verifier is `github.com/svix/svix-webhooks/go`; `Verify` validates signatures and timestamp. | Svix verification: https://docs.svix.com/receiving/verifying-payloads/how |
| Svix signs `<id>.<unix-seconds>.<raw-body>` with HMAC-SHA256 and permits multiple space-delimited versioned signatures. | Svix manual verification: https://docs.svix.com/receiving/verifying-payloads/how-manual |
| Svix Go v1.99.1 `Verify` uses a five-minute past/future timestamp tolerance and constant-time HMAC comparison. | Exact source: https://raw.githubusercontent.com/svix/svix-webhooks/v1.99.1/go/webhook.go |
| MCP 2026-07-28 is stateless, removes the initialize/session handshake, carries protocol/client metadata per request, and requires Streamable HTTP method/name routing headers. It deprecates DCR in favor of CIMD but retains compatibility. | MCP release: https://blog.modelcontextprotocol.io/posts/2026-07-28 and transport: https://modelcontextprotocol.io/specification/2026-07-28/basic/transports |
| MCP authorization uses OAuth protected-resource metadata; audience/resource restriction and `WWW-Authenticate` metadata discovery derive from RFC 9728, while `resource` is the RFC 8707 absolute-URI request parameter. | MCP authorization-server discovery: https://modelcontextprotocol.io/specification/2026-07-28/basic/authorization/authorization-server-discovery ; RFC 9728: https://datatracker.ietf.org/doc/html/rfc9728 ; RFC 8707: https://datatracker.ietf.org/doc/html/rfc8707 |
| PKCE binds the authorization request/code to a verifier; authorization-response `iss` identifies the AS and must be compared with the expected issuer to prevent mix-up. | RFC 7636: https://datatracker.ietf.org/doc/html/rfc7636 ; RFC 9207: https://datatracker.ietf.org/doc/html/rfc9207 |
| The official Stytch B2B Get Sessions operation is `GET /v1/b2b/sessions` and accepts exact `organization_id` and `member_id` filters. The selected Primer call is exactly one unpaginated official Go SDK **v18.1.0** `client.Sessions.Get(ctx, &sessions.GetParams{OrganizationID, MemberID})` (`github.com/stytchauth/stytch-go/v18/stytch/b2b/sessions`); `GetParams` has only those two fields. The response is `*sessions.GetResponse` with `MemberSessions []sessions.MemberSession`. Returned active sessions include `member_session_id` and expiry, allowing token-free exact-session revalidation with project credentials. | Stytch Get Sessions: https://stytch.com/docs/b2b/api/get-session ; Stytch Go v18.1.0 |
| Official Stytch Go SDK v18.1.0 exists at tag `v18.1.0`. It provides the B2B API client used by IA; webhook signature verification is supplied by Svix, not a verified Stytch Go webhook helper. | Stytch Go tags: https://github.com/stytchauth/stytch-go/tags ; Svix Go: https://pkg.go.dev/github.com/svix/svix-webhooks/go |

### Assumptions that must be qualified before implementation

- The exact Stytch Dashboard event catalog can vary by enabled features and is the runtime schema source of truth. IB4 must snapshot the configured catalog and fixtures before enabling the endpoint; this candidate records accepted **families** and envelope fields, not undocumented event spellings.
- Exact production hosts, client IDs, redirect URIs, scopes, and Stytch dashboard configuration are deployment inputs. They are required registrations, never inferred from request Host headers.
- Five minutes is the candidate value because it is the verified Svix Go v1.99.1 behavior. If IB4 pins another version, it must prove the same tolerance or explicitly revise this decision before implementation.

## Primer-selected boundary

These are **Primer decisions**:

1. Identity is an OAuth AS to Primer products and MCP clients; it is not an OAuth proxy that passes Stytch tokens through.
2. Browser/BFF starts `GET /oauth/authorize`. Identity validates the complete registered tuple `(client_id, redirect_uri, resource, audience, scopes)` before any Stytch redirect. OAuth state is recoverable only through `state_hash` plus the bounded `state-seal-v1` AES-256-GCM envelope; its exact versioned envelope and length-prefixed AAD encoding are frozen in `03-data-state-and-revocation.md` and bind transaction UUID, internal client UUID, registered redirect URI, resource URI, and audience without concatenation ambiguity.
3. Identity owns `/broker/stytch/**` login/discovery/callback. Only server-side Identity code receives Stytch `token`, `sso_token`, intermediate-session token, session token, or SessionJWT.
4. Identity validates Stytch completion, verifies `member_authenticated`, and revalidates the exact project/org/member/member-session tuple through a vendor-neutral adapter. The selected provider operation is exactly one unpaginated official B2B Get Sessions `GET /v1/b2b/sessions` with exact `organization_id` and `member_id`. There is no cursor, page, or limit parameter. The official Go SDK **v18.1.0** call is `client.Sessions.Get(ctx, &sessions.GetParams{OrganizationID: exactOrganizationID, MemberID: exactMemberID})` from `github.com/stytchauth/stytch-go/v18/stytch/b2b/sessions`; `GetParams` has only those two fields and the response is `*sessions.GetResponse` with `MemberSessions []sessions.MemberSession`. IB1 must qualify that exact method, params, and response plus a bounded 1 MiB transport body; if it cannot or the pin drifts, **STOP**. The adapter uses a 2-second total deadline, a 1 MiB response-body cap, and at most 256 returned `member_sessions`. It requires exactly one byte-equal active, unexpired stored `member_session_id` with the requested organization/member tuple. A positive proof cache is at most 15 seconds. Timeout/cancellation/429/5xx is unavailable. Missing exact stored `member_session_id` or inactive/expired is definitive denial. More than 256 sessions, duplicate IDs, body over cap, malformed payload, or tuple mismatch is unavailable/fail-closed. No negative or stale success is used, and no raw token persists.
5. IB1 adds `ProviderMemberSessionID` to `StytchSessionSnapshot`: non-empty, valid UTF-8, no controls, ≤255 runes/1024 bytes. It is stored with the tuple and expiry; raw provider session material is erased after the transaction.
6. Identity creates one Primer authorization grant and returns one use Primer code to the exact registered redirect. The BFF exchanges server-to-server. Browser JavaScript never receives an access/refresh token or a Stytch artifact.
7. Product BFF owns its own `__Host-<product>-session` cookie, CSRF token, origin checks, and refresh-token custody. Identity does not set a parent-domain product cookie.
8. No implicit cross-product SSO. Each product authorization performs the broker flow. A future Identity SSO design requires a separately reviewed durable provider-session/grant lifecycle.

## Interactive sequence

```mermaid
sequenceDiagram
  actor U as Human browser
  participant B as Product BFF
  participant I as Primer Identity AS/broker
  participant S as Stytch B2B
  participant D as Identity PostgreSQL

  U->>B: GET /auth/login
  B->>B: create host-only pre-auth state + PKCE verifier
  B-->>U: 302 Identity /oauth/authorize (registered tuple)
  U->>I: GET /oauth/authorize
  I->>D: insert broker transaction (state HMAC lookup + bounded AEAD state ciphertext, embedded nonce, versioned key, PKCE and exact tuple)
  I-->>U: Stytch login/discovery UI
  U->>S: selected ordinary auth method
  S-->>U: redirect to Identity callback with one-time artifact
  U->>I: GET /broker/stytch/callback
  I->>S: server-side artifact exchange/session authenticate
  S-->>I: member_session_id + tuple + expiry (+ secret material retained only in memory)
  I->>D: decrypt state only in memory; serializable map tuple; create association/grant/code; terminalize and erase sealed state
  I-->>U: 302 exact BFF redirect?code=PrimerCode&state=...&iss=IdentityIssuer
  U->>B: exact callback
  B->>I: POST /oauth/token (code + verifier + client auth)
  I->>D: IB2 only: sign-before-commit one-use CAS consume code
  I-->>B: IB2 only: Primer access + rotating refresh material
  B-->>U: __Host-product-session; no token in response body/URL
```

## MCP sequence

```mermaid
sequenceDiagram
  participant C as MCP client
  participant M as Studio /mcp resource
  participant I as Primer Identity AS
  participant S as Stytch B2B

  C->>M: POST /mcp without token
  M-->>C: 401 WWW-Authenticate + resource_metadata
  C->>M: GET /.well-known/oauth-protected-resource[/mcp]
  M-->>C: resource=https://studio.example/mcp, authorization_servers=[Identity]
  C->>I: RFC 8414 AS metadata
  C->>I: authorization code + S256 PKCE + resource
  I->>S: Identity-hosted broker only
  S-->>I: validated provider session/tuple
  I-->>C: Primer one-use code then single-audience access token
  C->>M: POST /mcp Bearer Primer JWT
  M->>I: cached JWKS fetch only
  M-->>C: tool result; Studio local membership authorizes
```

Static/pre-registered clients are a Primer interoperability decision for IB1–IB8. Identity does not expose `registration_endpoint`; unknown DCR/CIMD clients fail `unauthorized_client`. Service actors skip Stytch and use distinct registered confidential clients with `client_credentials` in IB5.

## Provisioning and authorization precedence

A valid tuple proves provider authentication only. Mapping it to `accounts.id` does not grant product access. Studio `workspace_memberships`, LMS educator permissions, and TV admin/device rules remain local. Initial provisioning is one of:

- an existing explicit product invite bound to `accounts.id` after the tuple is mapped; or
- an administrator-created local membership referencing that account.

Provider organization/member suspension or deletion blocks new broker exchange and revokes provider-associated Primer grants/refresh families. It does not delete or downgrade product memberships. Optional deprovision reconciliation is a later, separately reviewed operation with dry-run, approval, and audit.

## Failure matrix

| Condition | OAuth/public result | Cache/persistence behavior |
|---|---|---|
| Stytch timeout, cancellation, transport, 429, or 5xx | `temporarily_unavailable`; browser gets safe retry page, no redirect error details | no negative cache; transaction remains non-success and expires |
| definitive invalid/revoked/expired provider session | `access_denied` | short bounded negative cache allowed by IA; no grant/code |
| incomplete MFA / `member_authenticated=false` | continue broker at Identity only | no grant/code; bounded transaction TTL |
| unknown/unmapped tuple without eligible provisioning | `access_denied` | optional account mapping may exist; no product membership created |
| client/redirect/resource/audience/scope mismatch | `invalid_request` or `unauthorized_client` before provider redirect | no provider call |
| broker transaction/code replay | `invalid_grant` / safe browser restart | failed CAS; audit replay outcome |
| refresh token reuse | `invalid_grant` | atomically revoke entire family and associated grant |
| webhook verification/timestamp/content-type/body failure | HTTP 400/415/413 | no ledger/effect for unverified request; sanitized metric only |
| exact duplicate webhook (both IDs resolve to one row and body hash matches) | HTTP 204 | existing ledger row; no duplicate effect |
| verified event-ID, Svix-ID, or cross-ID collision | HTTP 204 | insert/reuse one fingerprinted immutable security event and one leased alert item; never overwrite a receipt or apply authority effect |

No HTTP client used for Stytch, discovery metadata, or callbacks follows redirects automatically. Outbound URLs are configured/pinned HTTPS endpoints; loopback is test-only. Request and response limits are in the endpoint and data contracts.

## Threat model

| Threat | Required control/evidence |
|---|---|
| OAuth mix-up/code injection | state hash, exact redirect/client/resource tuple, S256 PKCE, authorization response `iss`, one-use code CAS |
| Open redirect | exact pre-registered redirect comparison; no wildcard, prefix, Host-derived, or query mutation |
| Stytch artifact leak | callback only on Identity; no query/body logging; generic errors; never persist raw token/session/JWT |
| Cross-product token replay | one `aud`; resource fixed at authorize and token exchange; validators reject any other audience |
| Cross-tenant/persona merge | exact unique tuple, no email lookup/link, product membership checked locally |
| Refresh theft/replay | high-entropy opaque token, HMAC-SHA256 hash at rest with versioned pepper, family rotation and reuse revocation |
| Webhook forgery/replay | raw body, Svix `Verify`, five-minute skew, dual-ID/hash comparison, versioned collision fingerprint, durable receipt/security evidence before effects |
| Out-of-order webhook | event is a signal; conservative revoke/`InvalidateAll`; no webhook grants or role mutation |
| Provider outage bypass | no stale success after provider expiry; no negative cache for transient failure; fail closed |
| Offline JWT overclaim | ≤15m expiry; honest statement that revocation is not immediate; no product/provider roles by default |
| Secret/PII leakage | no request/response/provider payload logs; only bounded IDs/correlation hashes and outcome codes |
