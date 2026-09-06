# Phase 2: Clerk authstack foundation

## Goal

Add authstack to the relevant Go modules and establish Clerk adapter construction at composition roots, provider-neutral authentication/authorization contracts below that seam, explicit policy registries, canonical identity mappings, and compatibility harnesses. No public route cuts over in this phase; legacy remains authoritative while Clerk/authstack runs only in shadow/compatibility modes that cannot grant access.

This phase makes later service and human migrations mechanical: handlers and domain services consume normalized `auth.Principal` and `auth.Authorizer`, while Clerk construction/JWKS refresh and temporary legacy adapters remain at startup boundaries.

## BDD Success Criteria

#### Scenario: Every protected boundary has an explicit policy

- **Given** a protected HTTP route group, MCP endpoint, or gRPC full method in the ratified matrix
- **When** startup builds its policy registry
- **Then** `AcceptedCredentials` is non-empty and audience, authorized party, and tenant requirements match the boundary
- **And** a missing route/full-method authorization entry fails closed
- **And** authentication executes before authorization.

#### Scenario: Shadow auth cannot grant access

- **Given** a request accepted by authstack but denied by the legacy path during shadow mode
- **When** it reaches a protected route
- **Then** the request is denied according to the authoritative legacy path
- **And** a redacted mismatch metric/audit record is emitted
- **And** no test or rollout flag turns a shadow success into authorization.

#### Scenario: Principal is normalized at the boundary

- **Given** a valid Clerk session/M2M test JWT or a conforming authstack test authenticator
- **When** the request passes authentication
- **Then** handlers/domain services receive `auth.Principal` from context
- **And** they do not parse JWTs, provider claims, headers, or cookies
- **And** canonical identity uses issuer plus subject.

#### Scenario: Canonical mapping is additive and collision-safe

- **Given** existing product-local person/member rows and legacy identity references
- **When** the mapping migration/backfill runs repeatedly
- **Then** it creates the same issuer+subject→local-ID links without duplication
- **And** uniqueness prevents one canonical identity from silently attaching to conflicting local identities in the same product/tenant
- **And** ambiguous links remain unresolved and deny access.

#### Scenario: Tenant mismatch is rejected before data access

- **Given** a valid principal for tenant A and a resource owned by tenant B
- **When** the caller requests the resource through a public API
- **Then** authentication may succeed but product authorization denies/not-founds the request
- **And** the repository query includes verified tenant/local membership bounds
- **And** no caller-supplied tenant substitutes for the principal context.

#### Scenario: Wrong credential class cannot be reclassified

- **Given** valid human, M2M, and product-local device credentials
- **When** each is presented to a boundary that disallows its class
- **Then** the boundary returns sanitized unauthenticated/forbidden semantics as specified
- **And** `KindHint`, header choice, or token shape does not reclassify it.

## Implementation Instructions

1. Add `git.clark.team/aleksclark/authstack` through each module's normal Go dependency workflow. Pin a reviewed commit/version compatible with repository Go versions; do not add a local `replace` in committed production modules.
2. At each composition root, load the exact configured Clerk JWKS/public key and construct `clerk.NewAuthenticator`, plus authstack RBAC or a product authorizer adapter. Add bounded unknown-`kid`/scheduled JWKS refresh at that composition seam. Keep `authstack/clerk` imports out of handlers/domain/repositories. Fail production startup on incomplete issuer/JWKS/audience/party/tenant configuration; resource servers receive no Clerk secret key.
3. Define immutable named HTTP policies and gRPC full-method registries from the Phase 1 matrix. Use `authhttp.AuthenticateWithPolicy`; use policy-aware unary and stream gRPC interceptors. Add authorization middleware/interceptors after authentication. Avoid broad middleware on public and device-only routers.
4. Introduce narrow adapters where Huma/MCP/framework contexts need translation, but store only `auth.Principal` in standard request context. Remove custom claim bags and duplicate principal types from new paths; compatibility translation may live only at composition/API seams.
5. Add an additive mapping schema in each product that needs stable local IDs. Suggested fields: local actor/member ID, canonical issuer, canonical subject, optional verified tenant ID, legacy issuer/subject reference, link state, link method, timestamps, and audit actor. Add exact unique/FK constraints and idempotent backfill jobs. Do not store provider access/refresh tokens there.
6. Preserve product-local authorization: LMS educator lookup/role; TV admin membership (add a table/repository if absent); Studio workspace membership and tool policy; Tasks tenant membership; Agents run/session ownership/profile admission.
7. Add a dual-evaluation result type at the composition boundary. It records legacy/authstack allow/deny/error without credentials. In `shadow`, only legacy can grant. In later `dual`, policy must specify whether either path or mapped-equivalent paths grant; never silently default.
8. Add test authenticators implementing current `auth.Authenticator` directly, not `LegacyAuthenticator` or token-only adapters. Test explicit policy behavior through real HTTP/gRPC handlers.
9. Ensure errors map to sanitized 401 + `WWW-Authenticate`, 403, 503, or 500 as authstack specifies. Logs may include stable policy name/outcome, not underlying credential/provider errors containing sensitive responses.
10. Generate OpenAPI/client artifacts after security schemes and cookie/BFF contracts change. At this phase public behavior remains legacy, so compatibility diff should show only additive schemas/config/documentation or explicitly approved contract corrections.

## End-to-End Test Plan

- Start each Go service with a real HTTP listener, Clerk session/M2M JWT plus JWKS fixture, authstack wiring in shadow mode, and its real PostgreSQL testcontainer where membership/mapping is involved.
- For each named policy, call one public boundary with missing, malformed, invalid signature, wrong issuer, wrong audience, wrong authorized party, expired, future `nbf`, disallowed credential kind, and missing tenant. Assert sanitized status and `WWW-Authenticate` where applicable.
- Call HTTP and gRPC variants with the same canonical principal and assert equivalent issuer/subject/kind/tenant semantics without adding a test-only endpoint that simply dumps claims.
- Insert two tenants and memberships in real Postgres; prove cross-tenant read/update/delete fail and inspect fixture rows to confirm canonical issuer/subject/tenant were derived from the verified principal.
- Run the canonical backfill twice and concurrently where supported; assert no duplicates and deterministic quarantine for ambiguous mappings.
- In shadow mode, force legacy/authstack disagreement and prove the authoritative result remains legacy while a redacted mismatch counter changes.
- Exercise signing-key rotation/unknown-key refresh through the selected authstack fixture or provider-supported integration harness.
- Run focused suites after each module, then root/module commands: `make test`, `make tv-test`, `make studio-test`, Primer Agents tests, `make tasks-test`, OpenAPI/client drift checks, race tests for mapping/repository code, and applicable coverage gates.

Cryptographic provider fixtures and test authenticators are allowed for deterministic policy matrices. They do not replace the live-provider flow required in Phase 4.

## Anti-Cheating Audit

- Search handlers/domain packages for `authstack/clerk` imports, JWT parsing, raw headers/cookies, or custom principal structs.
- Confirm policy registries are immutable and no empty `AcceptedCredentials` policy is “fixed” by accepting all kinds.
- Verify middleware ordering in actual routers and both unary/stream gRPC chains.
- Check shadow mode cannot set a principal used for access after legacy denial.
- Inspect mapping migrations for email-based linking, nullable uniqueness gaps, overwrite-upserts, cross-tenant links, and non-idempotent backfills.
- Verify tenant scope is in repository predicates, not only React/MCP tool filtering.
- Reject tests that invoke middleware helpers directly without a real public handler/listener or that assert only status while skipping persisted ownership.
- Confirm device routers are not wrapped by authstack and no API-key policy accidentally accepts opaque device tokens.
- Check logs/errors for token/provider body leakage and test-only bypass flags.

## Completion Gate

- [ ] Authstack dependencies are pinned and provider construction exists only at composition roots.
- [ ] Every ratified HTTP/MCP/gRPC boundary has an explicit non-empty policy and authorization entry.
- [ ] Additive canonical mapping schema/backfill is idempotent, collision-safe, tenant-aware, and rollback-compatible.
- [ ] Shadow mode cannot grant; mismatch observability is redacted and tested.
- [ ] Product-local authorization remains authoritative.
- [ ] Full negative credential matrix, HTTP/gRPC conformance, tenant isolation, key rotation, and device-class rejection pass through public boundaries.
- [ ] Generated artifacts, focused/full tests, race/coverage/lint/build gates pass.
- [ ] Anti-cheating audit finds no provider/domain leakage, fake persistence, or client-only authorization.
