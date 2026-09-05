# Phase 1: Integrated inventory and provider decision

## Goal

Establish the only safe baseline for the migration: the exact protected-master tip containing the integrated Primer Tasks implementation and baseline repairs, a complete human/service/device trust-boundary inventory, and an explicit Clerk-or-ZITADEL decision. This phase changes no production authentication behavior. It freezes canonical identity, tenant ownership, account-linking, browser-session, provider-provisioning, and rollback decisions so later phases do not improvise security policy.

Wave-00 orchestrator `b6ce6b31-ea58-4f00-935d-d71704fc7e16` completed and PR #70 merged as `cac90a151459a2fc60d506839424155f7f87ef80`, but that merge contains plans only: `primer-tasks/` is absent and WIP phases 3–13 remain open. Provider-neutral research and this plan may land now; Phase 1 acceptance and all production auth implementation require the later exact Tasks-integrated handoff tip.

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

#### Scenario: Provider is chosen explicitly

- **Given** authstack supports Clerk and ZITADEL and current Primer uses Stytch
- **When** the provider decision gate is completed
- **Then** exactly one provider is selected with a canonical issuer URL and subject semantics
- **And** provider-side applications/organizations/projects, redirect URIs, browser origins, resource audiences, M2M clients, key rotation, outage posture, and secret custody are listed
- **And** exactly the selected provider guide is read
- **And** the unselected guide is not used to create a dual-provider abstraction.

#### Scenario: Account linking never relies on email

- **Given** an existing Primer Identity account or product-local educator/member and a selected-provider identity
- **When** migration proposes a canonical link
- **Then** the link is based on explicit administrator/user proof and exact old/new issuer+subject values
- **And** same-email identities remain distinct unless an explicit audited link is approved
- **And** collisions or one-to-many mappings are quarantined for human resolution.

#### Scenario: Tenant ownership remains local

- **Given** provider organization/tenant claims and existing LMS educator, Studio workspace, Tasks tenant, or TV admin state
- **When** the ownership model is frozen
- **Then** provider tenant claims are verified context, not automatic product membership
- **And** each product remains authoritative for local roles and resource membership
- **And** switching tenant cannot reuse data loaded under the prior tenant.

#### Scenario: Rollback is operable before cutover

- **Given** no provider-specific production traffic has been cut over
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
6. Create the provider decision record in this phase file or a linked execution artifact. Compare only facts needed to decide: hosted vs self-managed responsibility, browser session/BFF fit, organization/tenant claim behavior, M2M flow, account export/import, key rotation, local development, outage/backup obligations, and operational cost. Do not read both provider implementation guides as a substitute for selection.
7. After the user or authoritative integrated architecture selects the provider, read exactly one of authstack `docs/agent_skill/clerk.md` or `docs/agent_skill/zitadel.md` completely and update the policy/config checklist with its exact credential kinds and setup.
8. Define rollout flags as per-boundary states (`legacy`, `shadow`, `dual`, `authstack`, and optionally `rollback`) rather than one global switch. Flags may select composition wiring; they must not weaken validation or create test-only authentication.
9. Define telemetry that contains policy name, outcome, credential kind, issuer identifier, and request ID without raw tokens, refresh tokens, secrets, provider payloads, email, or unnecessary claims.
10. Commit the complete plan and decision/inventory before delegating production implementation. The L2 sequence begins only after this commit and predecessor completion.

## End-to-End Test Plan

- In a clean worktree at the integrated base, run route/spec generation and enumerate OpenAPI security requirements for LMS, TV, Studio, Tasks, and Agents. Compare against middleware registration and flag any protected handler lacking a policy.
- Start each service using its normal test configuration and call representative public, human, service, and device routes with no credentials. Record actual status/headers; do not infer behavior from middleware unit tests.
- Run existing focused auth/device suites before changes to establish compatibility baselines: `go test ./server/internal/api/... ./server/internal/tv/api/... -count=1`, `make studio-test`, the Primer Agents test target, and `make tasks-test` once Tasks is integrated.
- Exercise the existing rollback path with credential-free/local fixtures: old parent login/session, Primer Identity JWT validation fixture, Studio test issuer/BFF fixture, and service shared-secret fixture must remain operational because this phase has no cutover.
- Validate database inventories against disposable PostgreSQL instances using existing migration/testcontainer harnesses. Query only schema/fixture identities generated by tests; never inspect production/local secrets.

Permitted fakes in this phase are existing cryptographic test issuers and HTTP provider fixtures used to document current behavior. They do not prove a live selected provider or production readiness.

## Anti-Cheating Audit

- Compare route registration to the policy matrix; reject inventories based only on grep counts or OpenAPI declarations.
- Check for generic CRUD, raw MCP, WebSocket/SSE, callback, metrics, and gRPC routes bypassing normal middleware.
- Inspect all `Authorization`, `X-Service-Token`, `X-Admin-Key`, cookie, token-source, and gRPC metadata writes for forwarding/reuse.
- Verify Tasks Android/browser pairing and LMS/TV pairing stores are classified as device-local, not mislabeled as provider sessions.
- Reject email, display name, provider role, or organization name as canonical-link keys.
- Confirm provider ambiguity is not resolved by silently choosing whichever adapter is easier to test.
- Confirm no secret values, environment contents, tokens, provider responses, or local credential files appear in plan artifacts or command logs.
- Check rollback retains databases, keys, client registrations, grants, and code required for the legacy path.

## Completion Gate

- [x] Initial `paseo wait` succeeded and plans-only PR #70 merge `cac90a15` is recorded and ancestral.
- [ ] The later WIP Phase 13 handoff containing integrated `primer-tasks/` and baseline repairs is recorded and ancestral.
- [ ] The index inventory is refreshed against integrated Tasks and every trust boundary has an owner and policy classification.
- [ ] Clerk or ZITADEL is selected explicitly; exactly one provider guide has been read and cited.
- [ ] Canonical-ID/linking, tenant ownership, browser sessions, provider setup, data migration, observability, and rollback are frozen.
- [ ] Baseline public-boundary and real-Postgres tests pass or failures are recorded as blockers.
- [ ] Device systems are explicitly excluded from authstack.
- [ ] The full phased plan passes link/section/traceability validation and is committed before implementation delegation.
