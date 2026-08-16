# 04 — Candidate Stytch methods, webhook receipt, provisioning, and effects

**Status: STOP — candidate contract under independent exact-tip review.** The handler is not authorized to apply any provider or product effect until a fresh exact-tip zero-finding review.

## Initial ordinary-human methods

Identity exposes only these Stytch B2B methods initially:

| Method | Identity ownership | Completion proof |
|---|---|---|
| Email Magic Link | Identity starts Stytch B2B discovery/organization flow; exact callback is `/broker/stytch/callback` | server-side consume one-time callback token, complete any discovery/org selection/MFA, then fresh session authenticate |
| Email OTP | Identity starts discovery/organization OTP and posts the code server-side; no product callback | `member_authenticated=true`, then fresh session authenticate |
| SAML SSO | Identity chooses a registered Stytch connection/org and owns the exact callback | server-side `sso_token` authenticate, complete MFA, then fresh session authenticate |
| OIDC SSO | same boundary as SAML SSO | same proof |

Passwords are disabled for ordinary users. A Stytch or local break-glass/legacy lane is explicitly configured, off by default, separately audited, rate-limited, and cannot be silently enabled in production. No email-based merge, cross-org linking, or “same Stytch member” heuristic is allowed.

Provider roles, `is_admin`, organization metadata, SSO group roles, and similar fields may be consulted only by a bounded eligibility policy. They are absent from Primer JWTs and never create product membership.

## Verified webhook envelope and catalog limitation

Stytch documents an envelope with `project_id`, unique `event_id`, `action`, `object_type`, `source`, entity `id`, and ISO-8601 `timestamp`. Event type is `source.object_type.action`; the guide explicitly demonstrates `direct.organization.create` and `scim.member.update`. CREATE/UPDATE include a full object under a key such as `organization` or `member`; DELETE may carry only IDs. Delivery has no ordering guarantee. Source: https://stytch.com/docs/b2b/guides/webhooks

The public guide explicitly lists organization/member/connection objects but does **not** verify a member-session webhook object or a dedicated session-revoked event. Therefore IB4 must not invent or depend on `member_session.revoked`. Provider session expiry/revocation is also enforced by fresh validation on broker/refresh boundaries. A later Dashboard catalog snapshot may add a session event only through an explicit contract revision with fixture evidence.

## Qualification-required initial event families

The following are Primer's intended **family categories**, not authoritative exact event identifiers. IB4 is **STOP / qualification-required** until a committed snapshot from the official B2B event catalog or configured Stytch Dashboard proves each exact `source.object_type.action` identifier, source, payload schema, and field/value eligibility mapping. Configure only the proven identifiers to Identity; do not synthesize wildcard names or assume that every `DIRECT`, `DASHBOARD`, or `SCIM` combination exists.

| Event family | Required IDs | Safe effect |
|---|---|---|
| member `UPDATE` category | envelope project ID + member entity ID; organization ID from verified member payload or authoritative follow-up fetch | always `InvalidateAll`; revoke exact tuple-associated provider sessions/grants/families only when the committed fixture-backed current-state field/value mapping says terminal/ineligible; otherwise no grant or membership change |
| member `DELETE` category | project ID + member ID; organization ID from ledger association/local association lookup | revoke all exact project/org/member associations; no product membership deletion |
| organization `UPDATE` category | project ID + organization entity ID | always `InvalidateAll`; revoke project/org associations only when the committed fixture-backed current-state field/value mapping says terminal/ineligible; otherwise no authorization change |
| organization `DELETE` category | project ID + organization ID | revoke all project/org associations |

Creation events and connection/configuration events may be durably receipted as `ignored` for audit, but they never provision accounts, grants, roles, tenants, or workspaces. Unknown object/action/source is durably `ignored` only after signature and envelope validation; alert on catalog drift. The allowlist is configured from an IB4 committed fixture captured from the official Dashboard catalog, not a hand-extended prefix matcher.

## HTTP receipt contract

`POST /webhooks/stytch` exists only on Primer Identity.

| Control | Candidate exact value (pending review) |
|---|---|
| content type | exactly `application/json` with optional UTF-8 charset; otherwise 415 |
| raw body limit | 256 KiB hard cap before allocation/read completion; oversize 413 |
| headers | require one complete canonical set: `svix-id`, `svix-timestamp`, `svix-signature`; white-labelled `webhook-*` may be accepted only if the configured endpoint actually uses it, never mixed |
| signature library | pin reviewed `github.com/svix/svix-webhooks/go`; use `NewWebhook` + `Verify(rawBody, headers)`, never `VerifyIgnoringTimestamp` |
| timestamp skew | five minutes past/future (verified v1.99.1 behavior); NTP/clock health required |
| signature input | unmodified raw bytes; JSON parsing happens only after verification |
| request time | 5 seconds total receipt budget; no provider call before durable receipt |
| response | 204 for a new event, an exact verified duplicate, or a verified ID/hash collision after durable quarantine/security evidence; 400 invalid signature/header/timestamp/JSON/envelope; collisions never apply effects and do not use 409/retry as an oracle |
| CORS/CSRF | no CORS; browser CSRF middleware not used; signature is authentication |

Svix headers and algorithm are documented at https://docs.svix.com/receiving/verifying-payloads/how-manual. The verified Go source uses HMAC-SHA256, constant-time compare, supports `svix-*`/`webhook-*`, and enforces ±5 minutes: https://raw.githubusercontent.com/svix/svix-webhooks/v1.99.1/go/webhook.go.

## Receipt algorithm

1. Reject method/content type/body/header limit failures without reading/logging payload beyond cap.
2. Read raw bytes once under cap.
3. Reject incomplete, duplicated, or mixed `svix-*`/`webhook-*` header sets, then verify signature and timestamp against the configured webhook secret using Svix `Verify` over the byte-identical raw body.
4. Parse a strict bounded envelope; require configured project ID, bounded IDs/type fields, valid action/source/object, and timestamp.
5. Compute SHA-256 of the raw body. In one transaction, acquire deterministic transaction-scoped advisory locks for canonical length-prefixed hashes of both unique identities, ordered by lock key, then read/lock rows found independently by `(provider,project,event_id)` and `(provider,svix_message_id)`. A serialization/unique conflict retries the bounded transaction and re-reads/reclassifies; the conflict itself is never called a duplicate.
6. An exact duplicate requires both lookups to resolve to the same receipt and that receipt's hash to equal the presented hash. These are distinct quarantines even when one hash matches: `event_id_reused_new_svix_id` (provider event ID reused with a new Svix ID), `svix_id_reused_new_event_id` (Svix ID reused with a new provider event ID), `cross_id_collision` (the two IDs resolve to different rows), and `body_hash_mismatch`. Never treat a unique violation or `INSERT ... ON CONFLICT DO NOTHING` as proof of duplication.
7. For a collision, compute the exact versioned length-prefixed semantic and observation fingerprints from `03-data-state-and-revocation.md`, insert-or-reuse one immutable semantic security event, one immutable distinct-observation row, and one alert-delivery row, preserve both original receipts unchanged, perform no authority effect, and return 204. The security event—not an ordinary incoming receipt—is the quarantine evidence.
8. Commit receipt/security evidence before effects and return 204. The handler does nothing further. Both the effect worker and collision-alert worker use CAS claim/reclaim with owner/token/expiry and version, retry exactly at 1m, 5m, 15m, 1h, 4h, 8h, and 12h, dead-letter failed attempt eight, recover expired leases after crash, and reject stale-token/version finalization.

Never log raw body, signature, webhook secret, member email/profile, or provider payload. Metrics use event family/outcome only after allowlist normalization; IDs are not labels.

## Out-of-order and provider lookup behavior

Stytch states webhook ordering is not guaranteed and recommends a webhook as a signal to fetch current state. IB4 uses this ordering:

1. Receipt is durable and idempotent independent of event timestamp.
2. Deletion or an already-known terminal association can be conservatively revoked without a provider fetch.
3. Updates query current member/organization state only through the pinned, qualified adapter where an exact org/member can be identified. Provider timeout/429/5xx marks effect `failed_retryable`; it never becomes an eligibility success or negative cache entry. Definitive not-found only terminally revokes matching provider association/grant/family; it never changes product membership.
4. A verified update indicating terminal state can only reduce authority. An older “active” update never reactivates a revoked grant/family.
5. Reactivation requires a fresh interactive broker flow and provider validation; product membership remains untouched.

## Cache and grant effects

Every verified relevant event invokes the safe initial global barrier `stytchcache.InvalidateAll`, because IA has no bounded member-session-ID-to-token cache index and raw tokens are not stored. This may reduce availability briefly but cannot preserve stale success. Targeted invalidation is deferred until a no-raw-token, bounded, race-tested index separately proves correctness.

Relevant terminal events atomically:

- mark matching `provider_session_associations` suspended/revoked;
- revoke matching human `oauth_grants`;
- revoke all corresponding `oauth_refresh_families` and live refresh-token rows;
- append `oauth_revocations` and `identity_audit_events` linked to the webhook ledger;
- leave existing access JWTs valid only until their ≤15-minute `exp`.

No webhook creates/links an account, issues a grant/code/token, adds or changes product membership, copies provider roles into local roles, or mutates Studio/LMS/TV databases.

## Provisioning contract

- Exact validated tuple may create/resolve an Identity account mapping under existing IA transaction rules.
- Product access still requires an explicit invite or admin-created local product membership referencing `accounts.id`.
- “Invite acceptance” may bind a pending local invite to the newly mapped account only through a product-owned authenticated flow and one-use invite proof; it is not inferred from provider email/domain/role.
- Provider org/member suspension blocks new Primer exchange and revokes Primer grants/families. It does not remove local roles. Optional deprovision reconciliation is deferred and must be dry-run/approved/audited.

## IB4 required fixtures

Before implementation PROCEED, commit sanitized fixtures for:

- the exact configured Dashboard/catalog event identifiers, sources, schemas, and update field/value eligibility mappings; absent evidence keeps every unproven family disabled;
- one valid signed event for each configured family/source combination;
- bad/missing/mixed headers; old/future timestamp; rotated multiple signatures;
- malformed/oversized/non-UTF-8 JSON;
- exact retry (both event/Svix IDs resolve to the same row and hash);
- unique/idempotent observation of `event_id_reused_new_svix_id`, `svix_id_reused_new_event_id`, `cross_id_collision`, and `body_hash_mismatch`, including concurrent identical repeats proving one fingerprint/event/alert, no original-row overwrite, and no authority effect;
- update→delete and delete→older update reorderings;
- provider follow-up 429/5xx/timeout and definitive not-found;
- member/org active update proving it cannot grant/reactivate;
- log/trace capture proving no raw body/signature/token/PII.
- main-worker and alert-worker lease CAS/reclaim, every retry delay, exhaustion/dead-letter, crash recovery, stale-token/version finalize rejection, and the separate append-only collision-security record.

The receiver gate must additionally prove that the exact bytes read under the 256 KiB cap are the bytes passed to Svix `Verify`, and that JSON parsing begins only after successful verification. A framework-decoded or reserialized body is a blocker even if semantic JSON is unchanged.

Live Stytch event delivery remains BLOCKED until approved credentials and Dashboard event configuration exist; locally signed Svix fixtures prove only the receiver boundary.
