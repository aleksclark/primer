# Phase 7: Caller integration and gradual cutover

## Goal

Adopt the service from real Primer callers through generated clients while retaining reversible, disabled-by-default cutover. LMS admin gains parent/admin agent chat and the workstation gains the sandboxed student path; existing LMS tutor routes and Fantasy TUI loops remain available until their separate acceptance gates pass. Curriculum Studio and future MCP receive a supported client/contract handoff only—this phase does not implement Studio S19 or an MCP server.

## BDD Success Criteria

#### Scenario: LMS admin uses the remote service only when opted in

- **Given** LMS/admin configuration with the remote agents flag disabled
- **When** existing admin/tutor flows run
- **Then** no `primer-agents` call occurs and legacy behavior is unchanged
- **Given** the flag is enabled with an Identity access JWT for `aud=primer-agents`
- **When** an authorized parent/admin starts or resumes chat
- **Then** the LMS admin surface uses the generated agents client to create/session/stream/cancel
- **And** run/session identity and events come from `primer-agents`, not the LMS process-local controller

#### Scenario: LMS-local product authorization happens before remote invocation

- **Given** an LMS parent/admin request involving a learner or LMS-owned context
- **When** the user is not locally authorized
- **Then** the LMS denies before invoking `primer-agents`
- **And** no remote run/session is created
- **When** authorized
- **Then** only a bounded opaque context is sent and remote ownership remains derived from the Identity JWT

#### Scenario: Workstation student cutover preserves the sandbox

- **Given** a workstation with remote student mode enabled and a reviewed Identity-issued agents credential
- **When** the student starts a tutoring turn
- **Then** the workstation uses the generated Go client and student stream helper
- **And** the service persists/executes the student profile with `max_children=0`
- **And** reconnect/cancel/status work across service restart

#### Scenario: Disabled or unavailable remote mode has explicit compatibility behavior

- **Given** remote mode is disabled or fails before any run is accepted
- **When** LMS/workstation starts a flow
- **Then** the documented legacy Fantasy/LMS path remains usable
- **Given** a remote run has been accepted and receives a server run ID
- **When** the network later fails
- **Then** the caller reconnects/polls that same run and never silently starts a duplicate legacy run
- **And** the UI reports unavailable/interrupted truthfully if recovery cannot complete

#### Scenario: Existing LMS preview routes migrate without trapping the engine

- **Given** the current `/agent/runs` compatibility contract and `AGENT_RUNTIME_ENABLED` preview
- **When** remote compatibility mode is enabled
- **Then** the LMS boundary delegates through the generated Go client or redirects clients according to the documented migration contract
- **And** it does not maintain an independently evolving MAF engine/controller copy
- **And** local preview remains available only as an explicit rollback until its removal gate is approved

#### Scenario: Consumer code uses generated contracts

- **Given** LMS admin TypeScript, LMS server Go, and workstation Go integrations
- **When** static import/transport gates run
- **Then** they import the official generated agents clients/event types
- **And** no handwritten DTO, raw `fetch`, manually assembled URL, or copied SSE event parser is used outside approved generated-client packages

#### Scenario: Studio and future MCP scope stays honest

- **Given** the generated Go client and service contract
- **When** Studio maintainers evaluate later integration
- **Then** they can authenticate to the service using `aud=primer-agents` once their caller authorization/grant is ready
- **And** no Studio agent runner, `/mcp`, S19 tool, or live Stytch flow is claimed or mounted by this phase

## Implementation Instructions

- Add caller-side configuration with explicit disabled defaults and distinct names from the service process, for example LMS/workstation `PRIMER_AGENTS_ENABLED`, base URL, timeout/reconnect bounds, and Identity token source. Validate HTTPS in production and never accept an agents API key/static shared-secret fallback.
- LMS admin integration must use the generated TypeScript client directly only where the reviewed BFF/token design safely supplies a short-lived `aud=primer-agents` token. Otherwise use an LMS BFF adapter built on the generated Go client; do not expose service credentials to browser JS. In either design, LMS product authorization occurs before remote create.
- Add parent/admin session/run/stream/cancel UI behavior to the LMS admin surface with persisted remote IDs, resumable event cursor, terminal/error display, and no optimistic fake completion. Follow the house frontend design skill during implementation because this materially changes browser UI.
- Add workstation generated-Go-client integration behind a remote student flag. Store only the minimum resumable run/session IDs/cursor appropriate to workstation policy; never store long-lived raw access tokens in transcript/state. Keep Fantasy as the default until TUI end-to-end acceptance.
- Define fallback precisely: before remote acceptance, callers may remain on legacy behavior; after receipt of a remote run ID, retries/reconnects remain attached to that remote run. No dual execution and no result blending.
- Migrate the current LMS `/agent/runs` preview boundary to an explicit compatibility adapter/proxy or deprecate it only after admin clients have moved. The runtime implementation remains agents-owned; reject divergent source copies. Remove the LMS MAF dependency/local controller only after the local-preview rollback gate is explicitly closed, not merely because the service exists.
- Regenerate the LMS OpenAPI/client only if the LMS compatibility/BFF signature changes; agents OpenAPI remains authoritative for direct service DTOs. Add cross-repo client-version compatibility tests.
- Publish a short Studio handoff documenting client package, audience/scopes, opaque context, and deferred integration. Do not modify Studio S19, add `/mcp`, or claim live Stytch/BFF proof.
- Update runbooks/feature matrix with flags, prerequisites, cutover cohorts, rollback, and who owns authorization. Identity live-provider qualification remains separately blocked.

## End-to-End Test Plan

- Run LMS, `primer-agents`, real separate PostgreSQL databases, and credential-free Identity/JWKS. Through the real admin browser flow, locally authorize a parent, start remote chat, observe incremental child-attributed events, disconnect/reload, resume, cancel, and inspect agents DB status. Use the browser E2E skill when implementing.
- Attempt the same LMS action as an unauthorized parent/context and assert no agents row exists.
- Run the actual workstation TUI/headless acceptance against the service with a student-scoped Identity token; verify typing/stream/reconnect/cancel plus zero child/MCP invocation. Use the TUI E2E skill during implementation.
- Exercise flags off, pre-acceptance outage fallback, post-acceptance network failure, and rollback. Count provider invocations and run rows to prove no dual execution.
- Exercise the legacy `/agent/runs` compatibility route in local-preview and remote modes; assert one authoritative runtime source and expected deprecation headers/docs if applicable.
- Run import scans and build generated clients from clean OpenAPI. Ensure consumer tests fail if a raw endpoint call or duplicate DTO is planted.
- Run existing LMS tutor/workstation acceptance with flags off, then full root and module gates.

## Anti-Cheating Audit

- Trace browser/TUI actions through official generated clients to a real agents process and database; reject fixture-only UI success or hard-coded stream data.
- Verify LMS local authorization occurs before the outbound request by asserting zero agents rows/provider calls on denial.
- Inspect token handling: no raw Stytch material, static agents key, browser-exposed service token, logged JWT, or persisted long-lived bearer.
- Search consumer source for raw `fetch`/HTTP URLs, copied DTOs/event structs, and manual SSE parsing outside generated-client packages.
- Force a network break after remote acceptance and verify no Fantasy/local duplicate starts.
- Diff LMS and agents runtime sources/imports; reject a maintained duplicate or silent behavior drift.
- Search Studio/router changes for `/mcp`, S19 claims, or live Stytch integration; generated-client handoff alone is the permitted Studio change.

## Completion Gate

- [ ] All BDD scenarios pass through real LMS admin and workstation boundaries with separate services/databases and credential-free Identity JWTs.
- [ ] Flags-off compatibility and post-acceptance no-duplicate behavior are proven.
- [ ] LMS local authorization denial creates no remote state; student cutover remains zero-child/fail-closed.
- [ ] Consumer generated-client/import gates pass; no duplicate DTO/raw transport exists.
- [ ] Existing Fantasy/LMS paths remain usable until explicit removal gates; runtime source is not trapped/duplicated in LMS.
- [ ] Studio/MCP/live-Stytch scope remains honest and deferred.
- [ ] Browser/TUI acceptance, module/root build/test/coverage gates pass unchanged.
