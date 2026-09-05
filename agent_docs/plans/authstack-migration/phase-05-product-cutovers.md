# Phase 5: Product-by-product cutover

## Goal

Make authstack authoritative for human/admin access one product boundary at a time—LMS, TV admin, Primer Tasks parent, Curriculum Studio REST/BFF/MCP/gRPC, then downstream Primer Agents—while preserving independent rollback and product-local authorization. Each cutover is a separate green, observable release rather than a flag day.

Service M2M is already migrated and dual-run human behavior already proved; this phase changes authority and removes access through superseded paths only for the boundary currently being cut over.

## BDD Success Criteria

#### Scenario: LMS human cutover preserves educator authorization

- **Given** a canonically linked selected-provider user with local LMS `parent` or `admin` role
- **When** LMS human authority changes to authstack
- **Then** browser/API actions succeed under the same local educator identity and household rules
- **And** an authenticated provider user without a local allowed role receives 403
- **And** legacy password/opaque sessions follow the declared drain or rollback policy rather than silently remaining a second authority.

#### Scenario: TV separates humans, services, and devices

- **Given** a local TV admin member, a TV M2M caller, and a paired TV device
- **When** each calls its intended routes after TV cutover
- **Then** the human can use admin browser routes, the service only its machine permissions, and the device only catalog/playback/device routes
- **And** every cross-presentation is rejected
- **And** no shared admin key remains an active browser credential.

#### Scenario: Tasks parent cutover leaves student pairing unchanged

- **Given** an authstack-authenticated parent with Tasks tenant membership and an existing paired browser/Android student
- **When** parent authority cuts over
- **Then** parent task/schedule/pairing administration uses the canonical parent and tenant
- **And** the existing student remains bound to the same Tasks-local student/device/session records
- **And** parent logout/provider revocation cannot act as a student device credential
- **And** student re-pair semantics are unchanged.

#### Scenario: Studio and MCP enforce local workspace/tool policy

- **Given** linked humans/services with differing Studio memberships and permissions
- **When** Studio REST, BFF, MCP, and gRPC cut over
- **Then** each transport yields equivalent canonical principals
- **And** workspace reads/mutations remain tenant and membership scoped
- **And** MCP tool discovery/invocation filters by local membership, principal kind, and permission
- **And** publish confirmation remains human-only
- **And** missing route/tool/full-method policies fail closed.

#### Scenario: Primer Agents preserves ownership and profile admission

- **Given** authorized service/human callers and another canonical principal
- **When** Agents cuts over
- **Then** route permissions and profile admission use authstack authorization
- **And** run/session/job ownership still prevents the second principal from reading/canceling the first principal's work
- **And** retries/idempotency/SSE behavior are unchanged.

#### Scenario: Each cutover rolls back independently

- **Given** earlier products are stable on authstack and the current product exhibits a release issue
- **When** operators invoke the current product's rollback procedure
- **Then** only that product returns to its rehearsed legacy/dual state
- **And** canonical links and service auth remain intact
- **And** no traffic is routed to deleted Identity state.

## Implementation Instructions

Perform cutovers as separate protected-master PR/release gates in this order unless Phase 1 evidence documents a safer dependency:

1. **LMS:** make authstack human/BFF policy authoritative for parent/admin routes; map canonical identity to local educator; remove the SPA's bearer paste/localStorage production path; keep explicit break-glass policy separate and audited if required. Update Huma security schemes and generated `web` client.
2. **TV admin:** require local human admin membership; mount distinct human and machine policy groups; remove browser/shared-key path; retain device guard/router unchanged. Update TV OpenAPI/client and content-ingest callers already migrated in Phase 3.
3. **Primer Tasks parent:** make selected-provider BFF authoritative for parent routes and verified tenant; keep `tasks-test-issuer` only under test wiring; preserve student pair endpoints, opaque credentials, WebSockets, Android encrypted store, revocation, and archive semantics.
4. **Curriculum Studio:** replace handwritten `authn.Validator`/custom contexts across REST, BFF, MCP, and gRPC with authstack contracts. Adapt local membership repos and tool policy behind `auth.Authorizer`. Ensure unary and stream full-method registries cover all RPCs. Remove migration alias only after callers prove M2M bearer.
5. **Primer Agents/downstream:** replace custom authn principal and scope middleware with authstack principal/authorizer. Keep run/session owner identity and profile admission. Update LMS/Studio generated client composition to token transports, not raw bearer constructors.
6. For each product, move rollout state `dual`→`authstack`, monitor redacted parity/denial/provider-outage metrics for the agreed window, run browser and backend evidence, then mark cutover accepted before starting the next.
7. Keep legacy code/data/config available but inactive for rollback. Do not remove Primer Identity, Stytch, JWT validators, old session tables, or shared-secret fields here; Phase 7 owns destructive cleanup.
8. Update public API errors/contracts only when authstack's standardized semantics require it. Regenerate specs/clients and update callers in the same product cutover.
9. Add release runbooks with exact flag transitions, smoke tests, abort thresholds, rollback steps, and evidence locations. Never place credential values or provider response bodies in them.

## End-to-End Test Plan

For each cutover, run all of the following before advancing:

- **Browser flow:** managed headless browser performs selected-provider login, callback, authenticated page/data load, permitted and denied action, refresh/session continuity, tenant switch where supported, logout, and post-logout denial. Inspect storage/cookie attributes without exposing values.
- **Backend policy:** missing/malformed/wrong issuer/audience/party/kind/tenant, expired/`nbf`, no local membership, forbidden role/permission, provider unavailable, and unexpected authorization errors through public HTTP/MCP/gRPC boundaries.
- **Persistence:** real PostgreSQL proves canonical link/local actor/membership/tenant ownership and cross-tenant/cross-owner denial for reads and mutations.
- **Rollback:** flip only the current product to rollback, repeat its legacy smoke test, then restore authstack and repeat the selected-provider smoke test.
- **LMS:** create/read/modify representative parent resources and manage a workstation pairing code; verify educator audit identity and device remains local.
- **TV:** admin catalog/schedule/device management as human; content-ingest machine operation; paired Android catalog/playback; full cross-credential matrix.
- **Tasks:** parent author/publish/schedule/issue pairing; paired browser and Android student checklist/dialogue/artifact path; archive/revoke/re-pair; cross-tenant denial.
- **Studio:** REST authoring, BFF, MCP tools/list+tool call, human-only publish confirmation, gRPC permitted/forbidden calls, and HTTP/gRPC principal conformance.
- **Agents:** generated client creates/reads/cancels run/session under one principal; another principal denied; idempotency reconnect and SSE/replay still work.
- Run product commands and global gates: `make test`, `make tv-test`, `make studio-test`, Primer Agents tests, `make tasks-test`, browser suites, OpenAPI/client drift checks, race/coverage/build/container checks, and deploy manifest validation.

Real selected-provider browser evidence is mandatory for each human product. Backend deterministic fixtures remain necessary for exhaustive negative cases.

## Anti-Cheating Audit

- Verify rollout state in actual production composition, not only tests or docs.
- Inspect routers for leftover legacy middleware that accepts credentials in parallel after a product is declared cut over.
- Confirm local roles/memberships/ownership are checked server-side after authstack; provider role/tenant alone never grants product access.
- Check TV human, machine, and device policy groups are distinct and cannot fall back to shared secret or token-shape guessing.
- Check Tasks parent changes do not touch student token schema/guards or replace Android/browser pairing with provider sessions.
- Enumerate all Studio MCP tools and gRPC methods against authorization registries; reject partial probe-only coverage.
- Confirm Agents owner comparisons include canonical issuer+subject semantics or a collision-safe encoded/local mapping, not subject alone across issuers.
- Inspect browser source/storage for legacy bearer/admin-key paths still enabled in production.
- Reject smoke tests using internal handler calls, preinstalled context principals, in-memory production repositories, or status-only assertions.
- Verify rollback does not require force-push, destructive down-migration, or secret recreation.

## Completion Gate

- [ ] LMS, TV admin, Tasks parent, Studio/BFF/MCP/gRPC, and Agents cut over in separate green gates with recorded exact tips.
- [ ] Real browser and exhaustive backend policy/tenant tests pass for each human boundary.
- [ ] Product-local roles, memberships, owner checks, tool policy, and profile admission remain authoritative.
- [ ] TV and Tasks/LMS device systems continue on their local guards and pass cross-credential rejection.
- [ ] Generated specs/clients and all product/global test, race, coverage, build/container/deploy gates pass.
- [ ] Each product's rollback is rehearsed independently after cutover.
- [ ] Monitoring window/abort thresholds pass without unresolved auth parity, tenant, or provider errors.
- [ ] Legacy infrastructure remains inactive but intact for Phase 7 rollback-window closure.
- [ ] Anti-cheating audit confirms actual authority changes and no hidden second authentication path.
