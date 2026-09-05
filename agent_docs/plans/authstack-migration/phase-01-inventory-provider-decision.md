# Phase 1: Integrated inventory and Clerk decision freeze

## Goal

Establish the only safe baseline for the migration: the exact protected-master tip containing the integrated Primer Tasks implementation and baseline repairs, a complete human/service/device trust-boundary inventory, and a verified Clerk-specific contract. This phase changes no production authentication behavior. It freezes canonical Clerk identity, Organizations tenancy, account linking, browser/BFF sessions, M2M service tokens, provider provisioning, dual-run, configuration, and rollback so later phases do not improvise security policy.

The user selected Clerk on 2026-09-05. Authstack's Clerk guide was read completely together with the skill, Go integration guide, and verification guide; the ZITADEL guide remains intentionally unread. Wave-00 orchestrator `b6ce6b31-ea58-4f00-935d-d71704fc7e16` completed and PR #70 merged as `cac90a151459a2fc60d506839424155f7f87ef80`, but that merge contains plans only: `primer-tasks/` is absent and WIP phases 3–13 remain open. Clerk planning may land now; Phase 1 acceptance and production auth implementation require the later exact Tasks-integrated handoff tip.

## BDD Success Criteria

#### Scenario: Exact predecessor baseline is used

- **Given** the WIP integration plan has completed phases 3–13 and published a protected-master handoff containing `primer-tasks/`
- **When** this initiative fetches protected `master` and updates its branch
- **Then** that Tasks-integrated handoff tip is an ancestor of the initiative tip
- **And** the recorded inventory names that exact commit
- **And** no production auth commit is based only on pre-Tasks `b7a2027c` or plans-only `cac90a15`.

#### Scenario: Every credential boundary is classified

- **Given** the integrated repository contains LMS, TV, Studio, Primer Agents, Primer Tasks, workstation, Android, and all deploy/config code
- **When** the inventory is reviewed
- **Then** every inbound human session, user bearer, service bearer/shared secret, gRPC metadata credential, MCP credential, pairing code, and device token has one owner, issuer/verifier, audience or local binding, accepted routes, and revocation authority
- **And** every outbound authenticated call records whether it currently forwards, reuses, pre-mints, or independently obtains credentials
- **And** unknown boundaries block Phase 2 rather than being omitted.

#### Scenario: Clerk decision is applied exactly

- **Given** the user selected Clerk and the authstack Clerk guide is authoritative
- **When** the Clerk decision gate is reviewed
- **Then** one Clerk application with Organizations, an exact issuer/JWKS source, exact browser origins/authorized parties, per-product session audiences/templates, and caller-specific M2M machines/audiences are listed
- **And** human canonical subject is Clerk session JWT `sub`, service canonical subject is verified `machine_id`, and both are paired with the exact issuer
- **And** only JWT session/M2M credentials are accepted; opaque Clerk credentials and OAuth client credentials remain out of scope
- **And** the unread ZITADEL guide is not used to create a dual-provider abstraction.

#### Scenario: Account linking never relies on email

- **Given** an existing Primer Identity account or product-local educator/member and a Clerk identity
- **When** migration proposes a canonical link
- **Then** the link is based on explicit administrator/user proof and exact old/new issuer+subject values
- **And** same-email identities remain distinct unless an explicit audited link is approved
- **And** collisions or one-to-many mappings are quarantined for human resolution.

#### Scenario: Tenant ownership remains local

- **Given** a verified active Clerk Organization and existing LMS educator, Studio workspace, Tasks tenant, or TV admin state
- **When** the ownership model is frozen
- **Then** provider tenant claims are verified context, not automatic product membership
- **And** each product remains authoritative for local roles and resource membership
- **And** switching tenant cannot reuse data loaded under the prior tenant.

#### Scenario: Rollback is operable before cutover

- **Given** no Clerk production traffic has been cut over
- **When** operators execute the documented rollback rehearsal
- **Then** existing Primer Identity/Stytch and product-local session behavior remains available
- **And** additive mapping data can be ignored without deletion
- **And** no old key, account, grant, or session store has been destroyed.

## Implementation Instructions

1. Record the completed initial wait/merge: predecessor head `51656816f7954ac1f61ca751b8ffc47eb4caa983`, PR #70 merge `cac90a151459a2fc60d506839424155f7f87ef80`, and the fact that it is plans-only. Wait for WIP integration phases 3–13 and `handoff.json`, fetch without reading environment values, then rebase/update `initiative/01-authstack` onto that exact later protected-master commit. Record both bases and prove ancestry.
2. Refresh `index.md` current-state evidence after Tasks code integration. Inventory composition roots (`server/cmd/*`, `curriculum-studio/cmd/*`, `primer-agents/cmd/*`, `primer-tasks/cmd/*`), middleware/interceptors, BFFs, configs, schemas, generated contracts, clients, deploy jobs, CI, and browser/Android token stores.
3. Produce a checked policy matrix keyed by stable route group or gRPC full method—not informal URL comments. Include public probes, human-only, service-only, mixed-but-explicit, and device-only boundaries. Identify routes currently left inert when config is absent.
4. Inventory persisted identity fields and constraints: LMS `educators.identity_subject` and parent sessions; Studio membership `subject_ref`; Tasks parent/tenant/auth-state/session records; TV human admin membership gap; Primer Agents owner subject references; Primer Identity accounts/external identities/grants/sessions.
5. Choose whether products retain stable local person/member IDs behind an additive canonical link table (preferred for rollback) or migrate ownership columns directly. If direct migration is proposed, specify reversible migrations and foreign-key ordering. Canonical external identity remains issuer+subject either way.
6. Apply the frozen Clerk decisions from `index.md`: exact `CLERK_ISSUER`; configured JWKS URL/public-key path; human `sub`; service `machine_id`; active Organization as verified `TenantID`; product-local tenant/membership authority; session-only human boundaries; JWT M2M-only service boundaries; exact `azp`; per-product audiences/session templates; opaque credential/RemoteVerifier exclusion; and resource-server operation without Clerk secret keys.
7. Qualify the application-owned BFF/session-establishment design against a Clerk development instance. Frontend session JWTs are memory-only and submitted once if Clerk SDK mechanics require it; BFF state is durable/encrypted and browser cookies opaque/HttpOnly/Secure/host-scoped. Document exact organization-switch/session-refresh/logout mechanics before code cutover.
8. Inventory each Clerk Machine→target audience and the application-owned `auth.TokenSource`/secret-manager rotation path. Do not use `oauth.NewClientCredentialsTokenSource`: Clerk OAuth client credentials are unsupported by authstack's Clerk adapter. Do not enable opaque M2M/API keys or `RemoteVerifier`.
9. Define rollout flags as per-boundary states (`legacy`, `shadow`, `dual`, `clerk`, and `rollback`) rather than one global switch. Shadow cannot grant; dual grants only when legacy and Clerk map to the same approved local actor/membership. Flags may select composition wiring; they must not weaken validation or create test-only authentication.
10. Define telemetry that contains policy name, outcome, credential kind, issuer identifier, and request ID without raw tokens, refresh tokens, secrets, Clerk response bodies, email, or unnecessary claims.
11. Commit the complete plan and Clerk decision/inventory before delegating production implementation. The L2 sequence begins only after this commit, the Tasks-integrated handoff, and refreshed inventory.

## End-to-End Test Plan

- In a clean worktree at the integrated base, run route/spec generation and enumerate OpenAPI security requirements for LMS, TV, Studio, Tasks, and Agents. Compare against middleware registration and flag any protected handler lacking a policy.
- Start each service using its normal test configuration and call representative public, human, service, and device routes with no credentials. Record actual status/headers; do not infer behavior from middleware unit tests.
- Run existing focused auth/device suites before changes to establish compatibility baselines: `go test ./server/internal/api/... ./server/internal/tv/api/... -count=1`, `make studio-test`, the Primer Agents test target, and `make tasks-test` once Tasks is integrated.
- Exercise the existing rollback path with credential-free/local fixtures: old parent login/session, Primer Identity JWT validation fixture, Studio test issuer/BFF fixture, and service shared-secret fixture must remain operational because this phase has no cutover.
- Validate database inventories against disposable PostgreSQL instances using existing migration/testcontainer harnesses. Query only schema/fixture identities generated by tests; never inspect production/local secrets.

Permitted fakes in this phase are existing cryptographic test issuers and Clerk JWT/JWKS fixtures used to document current behavior. They do not prove a live Clerk application, Organization switch, or M2M flow.

## Anti-Cheating Audit

- Compare route registration to the policy matrix; reject inventories based only on grep counts or OpenAPI declarations.
- Check for generic CRUD, raw MCP, WebSocket/SSE, callback, metrics, and gRPC routes bypassing normal middleware.
- Inspect all `Authorization`, `X-Service-Token`, `X-Admin-Key`, cookie, token-source, and gRPC metadata writes for forwarding/reuse.
- Verify Tasks Android/browser pairing and LMS/TV pairing stores are classified as device-local, not mislabeled as provider sessions.
- Reject email, display name, provider role, or organization name as canonical-link keys.
- Confirm no ZITADEL/generic second-provider code is introduced and no Clerk guide requirement is silently replaced by easier generic OIDC behavior.
- Confirm no secret values, environment contents, tokens, provider responses, or local credential files appear in plan artifacts or command logs.
- Check rollback retains databases, keys, client registrations, grants, and code required for the legacy path.

## Completion Gate

- [x] Initial `paseo wait` succeeded and plans-only PR #70 merge `cac90a15` is recorded and ancestral.
- [ ] The later WIP Phase 13 handoff containing integrated `primer-tasks/` and baseline repairs is recorded and ancestral.
- [ ] The index inventory is refreshed against integrated Tasks and every trust boundary has an owner and policy classification.
- [x] Clerk is selected explicitly; the Clerk guide has been read completely and the ZITADEL guide remains unread.
- [x] Clerk canonical user/machine IDs, Organizations tenancy, browser/BFF sessions, JWT M2M, linking, dual-run, configuration, JWKS rotation, and rollback decisions are frozen in `index.md`.
- [ ] Those frozen decisions are qualified against a real Clerk development application after Tasks integration.
- [ ] Baseline public-boundary and real-Postgres tests pass or failures are recorded as blockers.
- [ ] Device systems are explicitly excluded from authstack.
- [ ] The full phased plan passes link/section/traceability validation and is committed before implementation delegation.
