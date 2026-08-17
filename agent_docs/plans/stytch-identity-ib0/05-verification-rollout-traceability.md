# 05 — Reviewed verification, rollout, rollback, and traceability

**Status: PASS — independent IB0 design review at `4bfd6d5c03412d134a635c32279ef37b1c4e0d9e`; 0 Critical, 0 Important, and 0 Minor findings.** Review evidence covers 40 docs/architecture files, links, 58 planned E2Es, 24 requirements, and passing LikeC4 topology. This remains the design freeze. Credential-free IB1 subsequently completed and passed review at code tip `59a3998208ba9ef87dfe0bf4a913eedab3753ef8`; IB2/I6 is next only after reviewed IB1 reaches `master`, and live Stytch credentials/browser proof remains **BLOCKED**.

## BDD success criteria

### IB0-S01 — Product broker isolation

- **Given** two statically registered product clients with distinct redirect/resource/audience tuples
- **When** a human completes each authorization
- **Then** each starts a distinct Stytch-backed broker transaction and returns a one-use Primer code only to the exact client redirect
- **And** no Identity or cross-product SSO cookie is reused
- **And** browser JS, URLs, Studio, LMS, TV, and MCP contain no Stytch session material.

### IB0-S02 — Tuple and product authorization separation

- **Given** two valid provider tuples sharing an email-like value, one with an admin-like provider role, and only one local Studio membership
- **When** both complete provider authentication and call Studio
- **Then** they map to distinct `accounts.id` subjects
- **And** only local membership authorizes
- **And** no workspace/role is auto-created or placed in a JWT.

### IB0-S03 — One-use code and rotating refresh

- **Given** a valid broker grant/code and refresh family
- **When** concurrent callers redeem the code or same refresh token
- **Then** exactly one succeeds
- **And** code replay fails `invalid_grant`
- **And** the winning rotation links same-family sequence `n` to `n+1`, leaving exactly one current token
- **And** refresh reuse records terminal timestamps and atomically revokes the entire family, grant, and every live token; no terminal family can issue again.

### IB0-S04 — Provider failures fail closed

- **Given** expired cached proof and scripted timeout, 429, 5xx, or definitive revoked responses
- **When** broker/refresh needs provider validation
- **Then** transient failures return unavailable without negative cache or stale success
- **And** definitive invalid/revoked/expired returns denial without grant/token issuance.

### IB0-S05 — Signed webhook durable revocation

- **Given** valid, forged, replayed, exact-duplicate, every ID/hash collision class, repeated identical collision, and out-of-order Stytch/Svix fixtures
- **When** Identity receives them at `/webhooks/stytch`
- **Then** only verified in-skew envelopes are durably receipted
- **And** receipt commits before idempotent effects
- **And** exact duplicate is only both-IDs-same-row-same-hash
- **And** every one-ID re-pair, cross-ID, and body-mismatch class, including concurrent identical repeats, is one fingerprinted immutable event plus one restart-safe leased alert item, returns 204, and has no authority effect
- **And** relevant terminal events call `InvalidateAll` and revoke provider-associated grants/families
- **And** no event grants or mutates product membership.

### IB0-S06 — Primer JWT and MCP boundary

- **Given** a delegated human, a Primer service principal, a raw Stytch token, and tokens for wrong audiences
- **When** they call Studio `/mcp`
- **Then** only valid ES256 `aud=curriculum-studio` Primer tokens pass authentication
- **And** Studio local membership/scope authorizes each tool
- **And** human write tools require delegated-human class
- **And** service actors have no publish-confirmation authority.

### IB0-S07 — Honest revocation bound

- **Given** an issued 15-minute offline Primer access JWT and subsequent provider suspension
- **When** the webhook is applied
- **Then** new exchange/refresh fails and grants/families are revoked
- **And** the existing JWT may remain valid until `exp`
- **And** documentation/metrics do not claim immediate access-token revocation.

### IB0-S08 — Rollout and rollback safety

- **Given** legacy LMS/TV auth and a credential-free fake boundary
- **When** IB8 dual-run is enabled
- **Then** legacy and Primer paths are measured separately while local roles remain authoritative
- **And** production cannot enable Stytch mode without live/pinned credentials, callback registrations, webhook secret/events, keys, and IB4
- **And** rollback disables new entry/refresh without destroying mappings/audit or extending tokens.

## Requirement traceability

| Requirement | Decision/data/API | BDD | E2E evidence |
|---|---|---|---|
| REQ-IB0-01 Identity-hosted broker, one-use code | D01; HTTP authorize/callback/code issuance | S01,S03 | IB1-E01, IB1-E02 |
| REQ-IB0-02 no implicit cross-product SSO | D02; broker cookie | S01 | IB3-E01 |
| REQ-IB0-03 durable model/hashed secrets/lifetimes | data tables/state machines | S03,S05 | IB1-E03, IB2-E11, IB4-E09..E11, IB6-E01, IB7-E01 |
| REQ-IB0-04 member-session ID in snapshot | provider association | S03,S05 | IB1-E04 |
| REQ-IB0-05 allowed methods/callback ownership | webhook/method doc | S01 | IB1-E05 |
| REQ-IB0-06 no email merge/provider role auth | D03–D05 | S02 | IB1-E06, IB8-E01 |
| REQ-IB0-07 explicit product provisioning | provisioning contract | S02,S05 | IB8-E02 |
| REQ-IB0-08 ES256 single-audience ≤15m plus required signed public `client_id` | JWT table | S06,S07 | IB2-E01..E04, IB2-E10,E11 |
| REQ-IB0-09 Primer-owned client_credentials | token endpoint/IB5 | S06 | IB5-E01,E02 |
| REQ-IB0-10 MCP static clients/code+PKCE/metadata | MCP contract | S06 | IB8-E03,E04 |
| REQ-IB0-11 signed webhook/event ledger | webhook + ledger | S05 | IB4-E01..E11 |
| REQ-IB0-12 failure/cache semantics | failure matrix | S04 | IB1-E07 |
| REQ-IB0-13 exact endpoints/errors/cookies/CSRF | HTTP contract | S01,S03,S06 | IB3-E01..E06 |
| REQ-IB0-14 JWKS donor assessment | donor evidence | S06 | IB2-E00 intake review |
| REQ-IB0-15 rollout/rollback/live block | rollout below | S08 | IB8-E05..E09 |
| REQ-IB0-16 no token/PII/provider payload logs | threat model/audit tables | S01,S05 | IB4-E08, IB7-E04 |
| REQ-IB0-17 OpenAPI/generated client policy | HTTP contract | S01,S06 | IB1-E08 |
| REQ-IB0-18 LikeC4 positive/negative topology | architecture model | S01,S05,S06 | IB0-E-C4 |
| REQ-IB0-19 recoverable sealed state | state DDL + callback contract | S01,S03 | IB1-E09 |
| REQ-IB0-20 exact provider revalidation | bounded adapter contract | S04 | IB1-E10 |
| REQ-IB0-21 private_key_jwt/metadata/revoke exactness | HTTP contract | S03,S06 | IB2-E05..E08 |
| REQ-IB0-22 sign-before-commit response rule | issuance transaction | S03 | IB2-E09 |
| REQ-IB0-23 webhook lease/security collision isolation | data + webhook worker | S05 | IB4-E09..E11 |
| REQ-IB0-24 MCP authority/MRTR | MCP contract and Studio refs | S06 | IB8-E05,E06 |

No requirement is considered implemented by IB0; rows name the future wave that must produce runtime evidence.

## Planned E2E matrix

| ID | Real public boundary and assertion |
|---|---|
| IB1-E01 | Browser→BFF→Identity authorize→fake Stytch callback→exact BFF redirect with one-use code/state/iss against real Postgres; no token exchange or JWT in IB1 and no Stytch material outside Identity |
| IB1-E02 | Identity authorize/broker/callback handler contract and OpenAPI/generated-client parity; no token endpoint/client/JWT path or provider payload schema in IB1 |
| IB1-E03 | migration fresh/upgrade/down in disposable real Postgres for IB1-owned broker transactions, provider-session associations, human grants, and authorization codes only; exact sealed-state/status/CAS, unique/index, FK/trigger, delete-action, and rollback/upgrade behavior; association tuple/account consistency and unique `(id, account_id)`; composite `DEFERRABLE INITIALLY DEFERRED` account/association FKs from human grants and broker transactions; wrong-account transaction/grant inserts fail; authorization-code hash/lifetime/broker uniqueness holds; 24h code-before-broker purge has no dependency on future retained tables; schema/log scans contain no raw provider token or payload |
| IB1-E04 | bounded `provider_member_session_id` survives snapshot→association; raw token scan empty |
| IB1-E05 | magic link, OTP and SAML/OIDC fixtures; incomplete MFA remains Identity-only |
| IB1-E06 | same-email cross-org tuples produce distinct accounts; no local membership creation |
| IB1-E07 | timeout/429/5xx not cached; revoked denial cached only within bound; no stale success |
| IB1-E08 | concurrent callback-artifact/code-issuance CAS; exactly one committed/returned Primer code, with no `/oauth/token` request, code redemption, or code consumption |
| IB1-E09 | `state-seal-v1` vectors for exact 1..1024-byte recovery and envelope/AAD bytes: cross-field concatenation collision separation, unknown envelope/key version, nonce/ciphertext/tag/AAD tamper, rotation overlap/retirement, expiry/replay, no plaintext/log, and every terminal path nulling sealed state and zeroing the in-memory state buffer |
| IB1-E10 | adapter qualification proves exactly one unpaginated official Go SDK v18.1.0 `Sessions.Get` with only OrganizationID/MemberID, `*sessions.GetResponse.MemberSessions`, 1 MiB body cap, max 256 member_sessions; missing exact stored member_session_id or inactive/expired is denial; >256/duplicate/malformed/tuple-mismatch/timeout/429/5xx is unavailable; no stale success; inability or pin drift is STOP |
| IB2-E00 | donor rebase/selective intake review at exact post-IB1 tip; no wholesale cherry-pick |
| IB2-E01 | token endpoint emits ES256; product validates through fetched JWKS |
| IB2-E02 | wrong/multiple audience, alg substitution, unknown `kid`, expired token rejected |
| IB2-E03 | claim allowlist proves required public `client_id`, absent `azp`, and no provider tuple/role/product role/org ID/internal UUID |
| IB2-E04 | configured TTL >900s fails; provider constraint caps actual expiry |
| IB2-E10 | wrong/missing/overlong/control-bearing `client_id` and internal-UUID-as-client_id fail closed; Studio MRTR handle/confirm tests bind public `client_id` string to the validated JWT claim and audit the field |
| IB2-E05 | private_key_jwt exact form/type/iss=sub/aud/ES256/local-key/durable-jti replay matrix; invalid_client is exact 401 JSON with no Bearer challenge, while failed Authorization-header Basic is exact `Basic realm="token", charset="UTF-8"` |
| IB2-E06 | RFC8414 path-aware issuer metadata and RFC9207 `iss` on success/error redirect; metadata field inventory and HTTP headers/media/cache matrix; exact authorization-code/refresh/client-credentials success bodies and omission of refresh/id_token/provider fields where forbidden |
| IB2-E07 | RFC7009 owning-client idempotent 200, optional hint, no token oracle; duplicate forms and PKCE 43–128/S256 base64url negatives |
| IB2-E08 | public token/code-consume/replay path occurs only after IB1; concurrent consume gives one success |
| IB2-E09 | signer failure rolls back, commit failure discards signed token, response-after-commit, and lost response consumes code/requires restart |
| IB2-E11 | migration fresh/upgrade/down for IB2-created signing-key, client-assertion replay, initial refresh-family/current-token, and token-issuance-audit tables; initial issuance rejects missing/hash/pepper/lifetime/current-token violations; signing-key indexes permit at most one `active` and one `next` row needed by IB2 without claiming retirement/destruction rotation; code purge sets nullable `token_issuance_audit.authorization_code_id` to NULL while copied `authorization_code_hash` preserves 400-day issuance evidence; exact FK/index/delete actions pass |
| IB3-E01 | browser E2E exact redirect/state/iss/PKCE and separate product host-only cookies |
| IB3-E02 | JS/localStorage/sessionStorage/network URL scan contains no access/refresh/Stytch token |
| IB3-E03 | CSRF/Origin negatives reject logout/mutations |
| IB3-E04 | open redirect/prefix/wildcard/Host poisoning rejected before provider call |
| IB3-E05 | BFF restart retains server-side session/refresh custody; cookie alone reveals no token |
| IB3-E06 | no CORS access to Identity token/revoke routes |
| IB4-E01 | valid Svix signed fixture receipts then revokes |
| IB4-E02 | forged/missing/mixed headers and past/future >5m rejected |
| IB4-E03 | exact dual-ID/same-row/same-hash duplicate is 204 idempotent; each of `event_id_reused_new_svix_id`, `svix_id_reused_new_event_id`, `cross_id_collision`, and `body_hash_mismatch` returns 204/no effect without overwriting receipts; concurrent identical observation yields one fingerprint/security event/alert |
| IB4-E04 | update/delete reorder never reactivates authority |
| IB4-E05 | crash after receipt and after effect replays safely after restart |
| IB4-E06 | verified relevant event invokes global `InvalidateAll`; no fake targeted index |
| IB4-E07 | product memberships unchanged; old JWT honest ≤15m bound |
| IB4-E08 | logs/traces/audit contain no raw body/signature/email/token/payload |
| IB4-E09 | migration fresh/upgrade/down creates IB4-owned webhook receipt and security-alert worker tables plus claim/reclaim indexes; main and alert workers use owner/token/expiry+version CAS claim and expired-lease reclaim and reject stale worker completion after restart |
| IB4-E10 | both workers use exact 1m/5m/15m/1h/4h/8h/12h retry schedule, then dead-letter on failed attempt eight without stranded work |
| IB4-E11 | IB4-owned collision event/observation/alert constraints, unique/lookup indexes, FK/delete actions, append-only evidence, and 400-day receipt/security/alert/revocation/audit retention pass; all collision classes use versioned length-prefixed fingerprinting and one leased alert item; duplicate observation is idempotent, 204, and has no authority effect |
| IB5-E01 | service `client_credentials` JWT has service subject and no refresh/provider association |
| IB5-E02 | service token cannot acquire human/write/publish-confirmation authority |
| IB6-E01 | refresh rotation, parallel race, same-family successor/sequence, active/terminal current-token constraints, and consumed-token reuse revoke family/grant/all-live-tokens durably; restart and migration-upgrade behavior preserve terminal state; refresh/grant/revocation/audit evidence and FK-safe purge obey 400-day retention |
| IB7-E01 | full signing-key rotation lifecycle: `next → active → retired → destroyed` timestamp/material constraints, overlap where old unexpired tokens validate while retired/destroyed keys cannot sign, at-most-one active/next partial indexes, and no private ciphertext after destroy, all on fresh/upgrade/down real-Postgres paths |
| IB7-E02 | restart/key custody and JWKS cache/ETag behavior |
| IB7-E03 | private keys absent from logs/JWKS/DB plaintext/backups fixture |
| IB7-E04 | failure/log redaction and request/response size/redirect denial probes |
| IB8-E01 | Studio valid mapped subject with membership allowed; no membership/provider role denied |
| IB8-E02 | invite/admin provisioning only; provider events never add/remove product roles |
| IB8-E03 | MCP RFC 9728 metadata→Identity code+PKCE→Studio `/mcp` interoperability |
| IB8-E04 | raw Stytch/wrong-aud rejected; human write vs service publish matrix |
| IB8-E05 | tool authority matrix: human/service list/read, explicit active service draft scope, human-only propose/confirm, no bypass |
| IB8-E06 | `studio.publish.propose` returns proposal only/no `requestState`; MRTR-capable initial `studio.publish.confirm` must return `InputRequiredResult` without publishing, then the same confirm tool/method is retried with exact state+responses and a new canonical string JSON-RPC id; confirmation binds public `client_id` string to the validated JWT claim; wrong/missing `client_id` handle tests fail closed with audit; atomic one-use consume, expiry terminalization/reissue, concurrent reissue, and every replay/binding/wrong-method mismatch fail closed; non-MRTR UI fallback carries no MCP state |
| IB8-E07 | LMS dual-run parity/role enforcement metrics and kill switch |
| IB8-E08 | TV human-admin migration leaves device tokens TV-owned |
| IB8-E09 | production missing/mismatched config fails before listen |
| IB8-E10 | approved live Stytch browser + webhook test (currently BLOCKED) |
| IB8-E11 | rollback drill within stated windows without data deletion/token lifetime extension |
| IB0-E-C4 | LikeC4 validate/build/export and rendered-edge/negative-pair assertions |

## Anti-cheating audit

Reviewers must inspect that:

- no handler returns a code/JWT without a real transaction and exact mapping;
- no in-memory broker/code/refresh/webhook state substitutes for Postgres;
- no fixture token is accepted in production branches;
- no Stytch SessionJWT is re-signed/passed through as Primer authorization;
- no tests call private handlers while claiming browser/OAuth/MCP public-path proof;
- no provider role/email/domain creates a product membership or account merge;
- no access token carries multiple audiences or product/provider roles by convenience;
- no refresh token/code/client secret is plaintext in DB, fixtures, logs, or generated clients;
- no webhook parses/reformats body before signature verification, ignores timestamp, applies effects before receipt, or grants access;
- no generic upsert/unique-violation is called webhook dedupe; no append-only collision row is mutated for alert delivery; no worker/alert timer is in memory; and no finalization by row ID alone omits lease token+version;
- no product or MCP makes a direct Stytch call or validates a Stytch token;
- no Identity-local revocation table is claimed to invalidate already-issued offline JWTs immediately;
- no fake/loopback result is labelled live Stytch evidence;
- no skipped external-client/live-provider gate is silently treated as pass;
- no donor branch is merged/cherry-picked wholesale while overwriting IA-R.

## Rollout waves

1. **IB0 reviewed design freeze:** independently PASS at the reviewed design tip above; no runtime or live-provider completion. Keep live provider/deploy blocked.
2. **IB1 credential-free callback/code issuance:** real Postgres + scripted local provider boundary; broker disabled in production. Add migrations, snapshot member-session ID, recoverable-state callback, provider revalidation qualification, and one-use code issuance. Public token endpoint/code consumption/replay/JWT are explicitly out of IB1.
3. **IB2 public token bridge:** selective donor reimplementation/intake after IB1, public code consume/replay, private-key client auth, initial signing-key and refresh-family/current-token schema, 400-day copied issuance evidence, sign-before-commit ES256 JWT/JWKS, and validator conformance. Signing-key retirement/destruction waits for IB7; refresh rotation/reuse waits for IB6.
4. **IB3 one product canary:** Studio BFF test/staging; separate cookies; no production traffic before IB4.
5. **IB4 revocation hard gate:** create and migration-test webhook receipt/collision/security-alert/revocation/audit tables, indexes, leases, retention, signed local fixtures, and restart/replay before an approved Stytch test-environment webhook. Production BFF/MCP remains blocked until green.
6. **IB5–IB7 lifecycle/hardening:** service principals in IB5; refresh rotation/reuse, terminal lifecycle, and long-lived retention in IB6; full signing-key rotation/retirement/destruction and recovery in IB7.
7. **IB8 dual run:** LMS old/new login measured separately; TV human admin only; Studio/MCP Primer-token-only canary. Local authorization remains unchanged.
8. **Live provider:** security-approved live credentials/callback/event configuration, browser/MFA/webhook drill, support runbook, then gradual traffic.

### Dual-run metrics (bounded labels)

`identity_broker_attempts_total{client,outcome}`, `identity_token_exchange_total{grant_type,outcome}`, `identity_provider_validate_total{outcome}`, `identity_webhook_total{family,outcome}`, `identity_refresh_reuse_total{client}`, `product_auth_path_total{product,legacy|primer,outcome}`, and JWT validation failures by stable reason. No account/member/session/event IDs, email, token fragments, or raw errors as labels.

## Rollback contract

- **Minutes (entry rollback):** disable product route to new `/auth/login` and MCP client registration; retain Identity mappings/grants/audit. Existing access JWTs naturally expire ≤15m.
- **≤15 minutes (authority convergence):** revoke affected refresh families/clients, stop new code exchange, keep JWKS available for already-issued tokens. Never remove current verification key before all tokens expire.
- **Hours (application rollback):** restore legacy LMS/TV human path under explicit feature flag while preserving local role checks; Studio/MCP can be disabled. Do not re-enable local ordinary passwords.
- **Schema rollback:** forward-fix in production. Down migrations only in disposable/pre-live environments after backup evidence.
- **Provider outage:** no bypass/stale extension. Show unavailable; legacy dual-run may remain available only while its separately approved migration window is active.

Rollback never deletes tuple mappings/webhook/audit evidence, never broadens redirect/scope/audience, and never extends token expiry.

## IB0 reviewed PROCEED gate

**Independent design-review criteria completed at the reviewed design tip:**

- [x] exact endpoints, tables, states, lifetimes, failure codes and ownership are reviewable in this package;
- [x] `provider_member_session_id` addition is explicitly scoped to IB1, while public token/code-consume/replay/JWT is scoped to IB2;
- [x] official facts/Primer assumptions have exact URLs and MCP/delivery references agree with the reviewed design freeze;
- [x] LikeC4 required positive edges render and forbidden direct Stytch pairs are absent;
- [x] mechanical link/heading/traceability/allowlist/diff checks passed.

**The IB0 PROCEED gate was consumed by reviewed credential-free IB1 at code tip `59a3998208ba9ef87dfe0bf4a913eedab3753ef8`.** IB2/I6 is next only after reviewed IB1 reaches `master`; IB1 makes no token/JWT/JWKS implementation claim and does not unblock live Stytch.

**STOP** if any value remains “TBD,” live credentials/provider calls are required, event spellings are invented beyond verified catalog evidence, generated/code/SQL artifacts appear, donor intake would overwrite IA-R, or runtime completion is claimed from docs/fakes.
