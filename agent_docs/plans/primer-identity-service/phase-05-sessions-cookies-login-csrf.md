# 05: IB1 / compose validated Stytch broker exchange

**Status: STOP — candidate dependency under independent exact-tip review; no dispatch.**

## Goal

Candidate IB1 scope, pending review of [`../stytch-identity-ib0/`](../stytch-identity-ib0/): `/oauth/authorize`, Identity-owned Stytch login/discovery/callback, recoverable state, bounded provider revalidation, durable provider-session/grant association, and one-use Primer code issuance. IB1 has no public token endpoint, code consume/replay, or access JWT; those start in IB2.

## BDD Success Criteria

#### Scenario: IB1-S1 — Exact tuple creates one Primer grant/code
- **Given** an exact registered client tuple and freshly validated Stytch session
- **When** Identity maps project/org/member/member-session IDs
- **Then** one serializable transaction creates the provider association, grant and 60-second hashed code
- **And** only the exact BFF redirect receives it.

#### Scenario: IB1-S2 — Exact unpaginated revalidation and replay/outage fail closed
- **Given** concurrent callback/code replay and the pinned Stytch Go v18.1.0 `Sessions.Get` provider boundary
- **When** the flow executes
- **Then** callback/code CAS yields at most one success
- **And** revalidation makes exactly one unpaginated `client.Sessions.Get(ctx, &sessions.GetParams{OrganizationID, MemberID})` call and accepts exactly one byte-equal active, unexpired stored `member_session_id` for the exact tuple
- **And** the response body is capped at 1 MiB and `MemberSessions` at 256 entries
- **And** transient failures are unavailable/not negative-cached/no stale success
- **And** missing exact session or inactive/expired is denial, while body/count overflow, duplicate IDs, malformed payload, or tuple mismatch is unavailable/fail-closed.

#### Scenario: IB1-S3 — Persona/provisioning isolation
- **Given** same-email cross-org tuples and provider admin roles
- **When** authentication succeeds
- **Then** tuples remain distinct accounts
- **And** no Studio/LMS/TV membership or role is created.

## Implementation Instructions

- Add only the IB1-owned broker-transaction, provider-session-association, human-grant, and authorization-code migrations; exact columns/keys/lifetimes are in `03-data-state-and-revocation.md`. Do not create or gate signing-key, refresh, token-issuance-audit, webhook, collision, alert, revocation, or later audit tables.
- Extend `StytchSessionSnapshot` with bounded `ProviderMemberSessionID`; map `member_session.member_session_id`; preserve no-token/no-PII snapshot boundary.
- Compose only the bounded cache/adapter path. Qualify exactly one unpaginated official Go SDK v18.1.0 `client.Sessions.Get(ctx, &sessions.GetParams{OrganizationID: exactOrganizationID, MemberID: exactMemberID})`; `GetParams` has no cursor/page/limit and the response is `*sessions.GetResponse` with `MemberSessions []sessions.MemberSession`. Enforce a two-second total deadline, 1 MiB transport-body cap, and at most 256 returned sessions. Missing/inactive/expired exact stored session is denial; duplicate IDs, overflow, malformed payload, tuple mismatch, timeout/429/5xx are unavailable. Identity consumes Stytch one-time/intermediate/session artifacts in memory and erases them after completion.
- Add `UNIQUE (id, account_id)` on provider-session associations and composite deferrable FKs from broker transactions and human grants so a provider association cannot be attached to the wrong account; enforce the NULL-safe human-grant account/association branch now, while IB5 owns service-principal grant enablement and its FK.
- Implement allowed Magic Link, Email OTP and SAML/OIDC SSO fixture paths with Identity callback ownership and incomplete-MFA continuation.
- Add Identity-owned OpenAPI handlers/schema and fail generated-client parity; expose no provider payload type.

## End-to-End Test Plan

Run IB1-E01..E10 from the IB0 verification matrix using real HTTP processes and disposable PostgreSQL plus a scripted provider boundary. Include fresh/upgrade/down for IB1-owned tables only; sealed-state/status/CAS and authorization-code unique/index behavior; tuple/account consistency; wrong-account broker/grant inserts rejected by the composite association/account FKs; callback-artifact/code-issuance CAS races (not code redemption); same-email isolation; member-session association; outage/cache classification; handler/OpenAPI/generated-client parity; exact `state-seal-v1` vectors/terminal zeroing; and an adapter qualification proving the exact unpaginated v18.1.0 method/params/response, 1 MiB/256 bounds, exact-session denial, duplicate/fail-closed behavior, and no stale success. Include raw-provider-token/payload schema and secret/log scans. Do not require any later-wave table. Live Stytch remains BLOCKED.

## Anti-Cheating Audit

No in-memory broker/code store, fake mapping success, raw token/session persistence, code returned before commit, email lookup, provider-role membership, direct adapter bypass, redirect wildcard, or test-only production auth path.

## Completion Gate

- [ ] IB1-S* and IB1-E01..E10 green at exact tip.
- [ ] Public OpenAPI parity, race suite, IB1-owned real-DB migrations/constraints, full build/vet/coverage and diff hygiene pass without a future-wave table.
- [ ] No Primer access token is issued before IB2.
- [ ] Fresh specification and quality/security review approves the exact tip.
