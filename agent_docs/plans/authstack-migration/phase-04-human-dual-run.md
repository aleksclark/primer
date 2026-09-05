# Phase 4: Human dual-run and session migration

## Goal

Introduce the selected provider for human/admin authentication while existing Primer Identity/Stytch and product-local sessions remain available as a measured rollback path. Build explicit canonical links, durable BFF/session custody, tenant-context transitions, and live browser proof before any product makes authstack authoritative.

This phase proves equivalence and migration safety across products; Phase 5 performs separate cutovers.

## BDD Success Criteria

#### Scenario: Existing user links to one canonical provider identity

- **Given** an existing legacy account/local membership and a signed-in selected-provider user
- **When** the user completes the approved linking/provisioning flow
- **Then** an audited additive link stores exact old and new issuer+subject values
- **And** the existing local person/member ID and product data remain stable
- **And** no email or provider role grants the link or membership.

#### Scenario: Ambiguous account is quarantined

- **Given** multiple legacy accounts share an email or a provider subject conflicts with an existing link
- **When** migration/backfill encounters the identity
- **Then** it grants no product access
- **And** emits a redacted actionable review record
- **And** requires explicit administrative resolution that is idempotent and audited.

#### Scenario: Browser session remains server-side

- **Given** a user completes selected-provider login through an LMS, TV, Tasks, or Studio BFF
- **When** the callback succeeds
- **Then** provider/access/refresh material remains in server-side session custody
- **And** the browser receives only an opaque `HttpOnly`, `Secure`, appropriately `SameSite` host-scoped cookie
- **And** state, nonce, PKCE where applicable, exact redirect/host/origin, and CSRF checks are enforced
- **And** browser local/session storage contains no bearer token or admin key.

#### Scenario: Tenant switch reloads authorization context

- **Given** a user belongs to tenants A and B and has data open under A
- **When** the active verified tenant changes to B
- **Then** server-side session context and product memberships are reloaded for B
- **And** A's cached/list/detail data is not visible or mutable
- **And** a provider tenant claim without local B membership still denies access.

#### Scenario: Logout and revocation terminate the right sessions

- **Given** active legacy and authstack sessions during dual run
- **When** the user logs out or the selected provider revokes the session
- **Then** the relevant BFF session is deleted/invalidated and cookie cleared
- **And** stale requests deny within the documented bound
- **And** unrelated users/tenants and product-local device tokens remain unaffected.

#### Scenario: Per-product rollback restores the old browser path

- **Given** a product is in human dual-run and canonical links are additive
- **When** its authstack path is disabled using the rehearsed rollback control
- **Then** existing legacy sessions/login continue according to policy
- **And** no canonical mapping or local membership data is lost
- **And** other products and service M2M migrations remain on authstack.

## Implementation Instructions

1. Use only the selected authstack provider adapter and guide. Construct provider clients/verifiers at each BFF/API composition root; expose only `auth.Authenticator`/`auth.Principal` downstream.
2. Define provider-side browser applications, exact callbacks/origins/authorized parties, tenant/organization behavior, and session lifetimes per product. Separate production and development registrations; no wildcard redirects/origins.
3. Implement or adapt durable BFF session stores for LMS, TV, Tasks, and Studio. Store provider material encrypted or as provider-managed session references according to the selected guide; cookies hold opaque session IDs only. Remove browser `localStorage` bearer/admin-key use from the new path.
4. Preserve legacy account/local IDs and attach canonical links. Build a migration command/report that reads product/Identity data through explicit service/database boundaries—not cross-database joins in production. Report unresolved/colliding links without sensitive claims.
5. For LMS, keep educator/password/opaque sessions only as legacy rollback during dual run; authstack login maps to local educator and enforces local role. Do not grant access solely from provider organization/role.
6. For TV, add explicit product-local human admin membership before accepting provider sessions. “Any valid JWT is admin” must not survive cutover.
7. For Studio, preserve workspace memberships and human-only publish confirmation. Replace or adapt the existing BFF, ensuring its store is durable in production and authstack is the API auth boundary.
8. For Tasks, migrate only parent/BFF identity and tenant context. `tasks-test-issuer` remains test-only and must be rejected by production config. Student browser/Android pairing credentials remain local and unchanged.
9. Implement dual-run evaluation at login/callback/session validation and protected requests. Define authoritative result and mismatch classes. Never call the legacy provider with selected-provider raw credentials or vice versa.
10. Implement logout, session expiry/rotation, provider revocation/outage behavior, and rollback. Retain Primer Identity/Stytch registrations, grants, keys, and BFF compatibility until Phase 7.
11. Add browser-safe `/auth/me` projections containing only needed local display/membership/session state; do not expose tokens or raw provider claims.
12. Add metrics and audits for login start/callback/session/tenant switch/logout/link outcomes using request IDs and canonical actor references, with redaction tests.

## End-to-End Test Plan

- Use the real development selected provider and managed headless browser for each product. Exercise login start, provider sign-in, callback, authenticated navigation/API load, refresh/session continuity, CSRF-protected mutation, logout, and post-logout denial.
- For a user with two provider tenants and local memberships, switch A→B and verify UI/API reload plus cross-tenant read/update/delete denial. Also test provider tenant B without local membership.
- Inspect browser storage/cookies through the browser tooling: no bearer/admin key in localStorage/sessionStorage/URL; session cookie has expected host/path/HttpOnly/Secure/SameSite attributes. Do not print cookie values.
- Seed explicit same-email distinct users and a link collision in disposable databases. Run migration twice; assert no auto-link, access denial, quarantine record, and idempotent explicit resolution.
- Exercise wrong state, nonce, PKCE verifier, issuer, authorized party, redirect host, duplicate cookie, Origin, CSRF, expired session, provider outage, and revoked session. Assert sanitized responses and no provider body/token leakage.
- Keep one legacy session active while creating an authstack session; compare product-local actor/membership/data results. Force a mismatch and confirm the configured authority denies or quarantines.
- Roll back each product independently and prove old login/session still works while M2M service flows remain authstack.
- Confirm device tokens remain valid across human login/logout/provider outage and cannot call the new BFF/human API.
- Run backend suites plus browser E2E. Terra review must load `browser-use-terminal` and use managed headless browser by default. Browser proof supplements, not replaces, policy/DB tests.

Credential-free provider fixtures cover adversarial callback branches; they cannot satisfy successful live development-provider login/session/logout or tenant-switch evidence.

## Anti-Cheating Audit

- Inspect browser bundle/source maps/storage to ensure tokens/admin keys are not merely hidden from UI.
- Verify cookies contain opaque references, not serialized provider/access/refresh tokens, and production BFF stores are durable—not default in-memory stores.
- Search callback/session handlers for unverified claims, email links, provider-role membership grants, wildcard redirects/origins, or Host-derived callbacks.
- Confirm selected-provider construction stays at composition roots and raw provider sessions do not enter product handlers/domain.
- Verify dual-run mismatch handling cannot choose “allow if either succeeds” without explicit safe mapping and authorization.
- Check tenant switch invalidates server/client caches and repository queries remain tenant-scoped.
- Inspect logout/revocation for global device-token invalidation or accidental cross-user session deletion.
- Reject browser-only assertions that do not test backend wrong-issuer/audience/party/kind/tenant and DB ownership.
- Confirm rollback evidence uses real old wiring/state and does not recreate deleted keys/registrations on the fly.
- Audit logs and error captures for credential/provider-payload leakage.

## Completion Gate

- [ ] Selected provider is live in development with exact product registrations and no wildcard callbacks/origins.
- [ ] Additive canonical links/backfills are explicit, idempotent, collision-safe, and email-independent.
- [ ] Durable BFF sessions, cookie attributes, state/nonce/PKCE, CSRF, logout, expiry, revocation, and outage behavior pass.
- [ ] Tenant switch/isolation and no-local-membership denial pass through browser and backend boundaries.
- [ ] LMS/TV/Studio/Tasks local authorization remains authoritative; TV has explicit human admin membership.
- [ ] Device credentials remain unaffected and cannot authenticate human BFF/API routes.
- [ ] Independent per-product rollback is rehearsed without undoing service auth.
- [ ] Managed browser, real-provider, real-Postgres, focused/full/race/coverage/build and redaction tests pass.
- [ ] Anti-cheating audit finds no browser token storage, in-memory production sessions, auto-linking, or client-only authorization.
