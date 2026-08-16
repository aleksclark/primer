# Phase 19: Streamable HTTP MCP endpoint

**IB0 status: STOP — candidate dependency under independent exact-tip review; do not dispatch this surface.**

**File:** `phase-19-streamable-http-mcp.md`
**Depends on:** Phases 1–8 (shell, authz, workspaces, catalogs, plan, validate, publish); credential-free test Identity/narrow verifier for local transport work; **I6 / IB2 + I8 / IB4** for production delegated writes; **I7 / IB3 + applicable I12 / IB8** where client registration or publish confirmation applies; contracts Phase 12 for tool-schema/conformance harness
**Duration guess:** 6–10 days
**Handoff wave:** delivery `S19` (see curriculum-studio-delivery)
**Design SoT:** [`../curriculum-studio-mcp-design.md`](../curriculum-studio-mcp-design.md)

## Goal

Mount an authenticated **Streamable HTTP** MCP endpoint on the same Studio deployable at **`/mcp`** so external curriculum-planning agents can plan draft curricula on behalf of an authorized user. Tools call existing application/domain services (never raw repos), enforce per-request Identity JWT + workspace authz on every opaque handle, allow service list/read or draft only with explicit scope plus active service membership, and keep propose/confirm human-only. Propose returns no state. An MRTR-capable initial confirm must issue a one-use five-minute MRTR without publishing, bound to subject/public signed `client_id`/workspace/draft digest/literal confirm tool; a new-ID retry of that same confirm method with exact state+responses and the same validated claim atomically consumes it. Missing/wrong `client_id`, `azp`, and internal OAuth-client UUIDs fail closed with sanitized audit. Non-MRTR clients use named Studio UI without MCP state; no scope/role/test/header bypass. Prove protocol/security negatives with the official Go SDK client and one real external Streamable HTTP client.

This phase exists after planning MVP and publish immutability so MCP cannot become a shadow domain. Transport qualification may start after S1/S2 once a handler host exists; full tool wiring waits on S3–S8 domain surfaces.

## Scope

### In scope

- Pin and qualify `github.com/modelcontextprotocol/go-sdk` **v1.7.0** (`mcp.NewStreamableHTTPHandler` / stateless mode — **exact API confirmed before mass build**)
- Target MCP spec revision **`2026-07-28`**: single `/mcp` endpoint; JSON-RPC over POST; JSON or request-scoped SSE response; **no** implicit protocol sessions/resumption
- MCP adapter package under `curriculum-studio/internal/mcp/` mounted from `cmd/studio-server`
- Per-request Bearer JWT validation (`aud=curriculum-studio`, required public `client_id`, no `azp`/internal client UUID) reusing Phase 2 middleware/principal extraction
- Authorization-filtered deterministic `tools/list`
- Initial tools (see design doc §5): workspace/curriculum discovery; standards/resource search; create draft; get/patch graph; validate; findings; publish propose; publish confirm (step-up)
- `structuredContent` + text fallback; tools-only capability set
- Transport security: Origin allowlist, version/header parity, body/concurrency/timeout/rate limits, disconnect cancel, SSE anti-buffering, redacted audit
- Idempotency + optimistic concurrency via existing domain/DB paths
- E2E with official SDK client + Hermes/mcporter (or equivalent)
- Audit events for tool invocations

### Out of scope / YAGNI

- Third deployable or third database
- Studio as OAuth OP / token issuer / refresh endpoint
- OpenAPI or protobuf duplication of MCP tool schemas (contracts Phase 12 owns schema/conformance SoT rules)
- Roots, sampling, logging capabilities
- Primer materialize gRPC replacement
- Silent publish; agent-owned production deploy
- Live Stytch / live model providers (remain BLOCKED elsewhere)

## BDD Success Criteria

#### Scenario: P19-S1 — Stateless Streamable HTTP discovery and tools/list

- **Given** Studio process with `/mcp` mounted and a valid human JWT `aud=curriculum-studio` with workspace membership
- **When** an MCP client performs optional `server/discover` then `tools/list` over independent Streamable HTTP POST requests
- **Then** the server advertises tools-only capabilities for this revision
- **And** the tool list is non-empty, sorted deterministically, and includes only tools allowed for that principal’s memberships/scopes
- **And** no MCP protocol session id is required for a subsequent tool call

#### Scenario: P19-S2 — Read tools hit real domain services

- **Given** seeded workspace, curriculum, standards, and resources
- **When** the client calls `studio.workspaces.list`, `studio.curricula.list`, `studio.standards.search`, and `studio.resources.search`
- **Then** each returns `structuredContent` matching the tool output schema plus text fallback
- **And** results are workspace-scoped and names-first where applicable
- **And** repository layer is not invoked directly from the MCP adapter (adapter → app service only)

#### Scenario: P19-S3 — Draft create and graph patch with optimistic concurrency

- **Given** an author-role subject on workspace W
- **When** the client creates a draft and applies a graph patch with an idempotency key and expected revision/version
- **Then** the draft graph persists via plan domain services
- **And** replaying the same idempotency key does not double-apply
- **And** a concurrent patch with a stale version returns a conflict error without corrupting the graph

#### Scenario: P19-S4 — Validate and findings

- **Given** a draft revision with a deliberate coverage gap
- **When** the client calls `studio.drafts.validate` then `studio.drafts.get_findings`
- **Then** findings reflect the real validation engine output (same as REST path)
- **And** an audit_events row records subject_ref, tool name, workspace_id, and opaque revision id

#### Scenario: P19-S5 — Publish never silent

- **Given** a valid draft that passes validation
- **When** the client calls `studio.publish.propose`
- **Then** only a delegated human receives a proposal/summary, never `requestState`; no publication occurs and a service caller is denied
- **When** a supported client first calls `studio.publish.confirm`
- **Then** it must receive `InputRequiredResult{resultType:"input_required",inputRequests,requestState}` without publication
- **And** it retries the same confirm tool/method with exact `requestState` plus `inputResponses` and a new JSON-RPC id
- **And** the state is persisted only as a digest, expires in five minutes, binds human subject/public signed `client_id` string/workspace/canonical draft digest/literal confirm tool, and is atomically consumed once
- **And** the retry must carry the same validated public `client_id`; missing/wrong claim, `azp`, or internal OAuth-client UUID is denied and audited without publishing
- **And** non-MRTR clients receive `publish_confirmation_required` naming Studio UI with no MCP state
- **And** elicitation is presentation only and no scope/role/header/idempotency/environment/test/service bypass exists
- **And** `plan_revisions.status` remains non-published until explicit confirmation succeeds

#### Scenario: P19-S6 — Authn/authz negatives

- **Given** Studio `/mcp` is up
- **When** a client presents wrong-aud JWT, expired JWT, no Authorization header, missing/wrong/overlong/control-bearing `client_id`, `azp`, or an internal OAuth-client UUID as `client_id`
- **Then** the request is rejected without tool execution
- **When** a subject holds a draft id from workspace W2 but is only a member of W1
- **Then** get/patch tools return not-found or forbidden with no W2 payload (IDOR closed)
- **And** JWT role claims without membership rows never authorize

#### Scenario: P19-S7 — Transport security negatives

- **Given** production-like Origin allowlist configuration
- **When** a browser-like client sends a disallowed or missing Origin
- **Then** the request is rejected
- **When** protocol version header/body is unsupported or mismatched
- **Then** the server returns a safe protocol error and does not execute tools
- **When** the client disconnects during a long tool with request-scoped SSE/progress
- **Then** server-side work is cancelled or abandoned per context and does not leak unbounded goroutines

#### Scenario: P19-S8 — External client interoperability

- **Given** the same running Studio `/mcp`
- **When** both the official Go SDK client and one external Streamable HTTP client (Hermes/mcporter or equivalent) perform optional discovery, tools/list, and one read tool without an initialize/session handshake
- **Then** both succeed against the same endpoint and auth
- **And** neither requires Studio-minted API keys

## Implementation Instructions

1. **Qualify SDK pin first (anti-redesign gate).**
   - Add a short spike under `curriculum-studio/internal/mcp/spike/` or contracts evidence dir (gitignored outputs OK).
   - Import `github.com/modelcontextprotocol/go-sdk` **v1.7.0**.
   - Confirm exact symbols for Streamable HTTP handler construction (planned name `mcp.NewStreamableHTTPHandler`), stateless mode, JSON vs SSE response selection, progress notifications, and elicitation/`InputRequiredResult` support.
   - Record PROCEED/STOP in phase evidence; **STOP** blocks mass tool wiring if Streamable HTTP stateless mode is unavailable on the pin (escalate pin/revision with a decision note — do not invent a custom transport).

2. **Package layout**
   - `internal/mcp` — HTTP mount, middleware (Origin, limits, version), handler wiring.
   - `internal/mcp/tools` — one file/group per tool; thin; no SQL.
   - Reuse `internal/auth` principal + membership checks from Phase 2.
   - Register route on the shared chi (or equivalent) router **outside** `/studio/v1` prefix: `POST /mcp` (and any SDK-required method variants on same path only).

3. **Auth path**
   - Extract Bearer on every request; validate JWKS plus the required public `client_id`; reject `azp` and internal OAuth-client UUIDs; build `AuthContext` retaining the validated public ClientID.
   - Required for production OAuth clients: RFC 9728 protected-resource metadata for exact resource `https://<studio-host>/mcp`, advertising Primer Identity and scopes; 401 responses carry its exact URL in `WWW-Authenticate`. No token endpoint on Studio. Static/pre-registered Identity clients only in IB1–IB8; delegated humans use code+S256 PKCE.
   - Filter `tools/list` by role + scopes; re-check on every `tools/call`.

4. **Tool → domain wiring**
   - Inject the same app services used by Huma handlers (workspaces, standards, resources, plan, validation, publish).
   - Map tool args ↔ service commands; map domain errors ↔ MCP tool errors (generic public message + stable machine code in structuredContent when safe).
   - Opaque IDs only; always pass `workspace_id` from authorized context, never trust client-supplied tenant alone.

5. **Publish confirmation**
   - `studio.publish.propose` is human-only and returns a proposal only; it never creates, accepts, or returns `requestState`.
   - An MRTR-capable client's initial `studio.publish.confirm` must return `InputRequiredResult` without publishing and persist only the five-minute state digest bound to the validated public `client_id` string. The client retries the same confirm tool/method with exact state+responses, a new JSON-RPC id, and the same validated claim; current membership/scope/client/bindings are revalidated and consume is atomic with publish. Never store or compare `azp` or Identity's internal OAuth-client UUID.
   - SDK elicitation presents input only. Non-MRTR clients receive `publish_confirmation_required` naming Studio UI and no MCP state. Services, replay, same-ID retry, any method other than the same confirm method, and every bypass fail closed.
   - Publish domain path must be identical to REST publish (immutability triggers fire).

6. **Idempotency & concurrency**
   - Accept idempotency key in tool args or `_meta` per freeze in contracts Phase 12; store via existing `idempotency_keys` scope `mcp:<tool>`.
   - Graph patch requires version/ETag from `get_graph`.

7. **Observability & audit**
   - Write `audit_events` for mutate tools and publish propose/confirm attempts.
   - Metrics: mcp_requests, mcp_tool_calls, mcp_auth_fails, mcp_origin_rejects.
   - Logs: never print Authorization or token fragments.

8. **Config (`STUDIO_` prefix)**
   - `STUDIO_MCP_ENABLED` (default on in non-prod once shipped; explicit in prod)
   - `STUDIO_MCP_ORIGIN_ALLOWLIST`
   - `STUDIO_MCP_MAX_BODY_BYTES`, `STUDIO_MCP_MAX_CONCURRENT`, `STUDIO_MCP_REQUEST_TIMEOUT`
   - `STUDIO_MCP_PROTOCOL_VERSIONS` (include `2026-07-28`)
   - Production fails closed if MCP enabled without JWKS mode / allowlist as required by env policy

9. **Tests**
   - Unit: authz filter matrix; schema validation of tool I/O.
   - Process E2E: real Postgres + real HTTP `/mcp`.
   - External client script documented in Makefile target `studio-mcp-e2e`.

## End-to-End Test Plan

| ID | Setup | Action | Assert | Command |
| --- | --- | --- | --- | --- |
| P19-E1 | Process+DB+JWKS; official SDK client | optional discover + stateless tools/list | deterministic filtered list; no MCP session | `make studio-mcp-e2e` / go test |
| P19-E2 | Seed catalogs+plan | read tools | structuredContent schema OK | same |
| P19-E3 | Author JWT | create draft + patch + replay idempotency | single apply | same |
| P19-E4 | Two concurrent patch clients | stale version | conflict; graph consistent | go test -race |
| P19-E5 | Human/service + valid draft | proposal-only response; initial confirm `InputRequiredResult`; same-confirm exact state+responses/new canonical string-ID retry; public `client_id` handle binding; expiry terminalization/reissue, concurrent reissue, replay/same-ID/wrong method/wrong binding/missing-or-wrong-client claim; non-MRTR/bypass probes | human-only one-use atomic publish; proposal has no state; live slot cannot strand; bad state/client denied with sanitized audit; Studio UI fallback has no MCP state | same |
| P19-E6 | Wrong aud / missing-wrong-overlong-control `client_id` / `azp` / internal UUID / IDOR handle | tool call | 401/403/not-found; no leak or publish | same |
| P19-E7 | Bad Origin + bad protocol version | POST /mcp | rejected | same |
| P19-E8 | External client (mcporter/Hermes) | list + read | success | documented script |
| P19-E9 | Disconnect mid SSE tool | cancel | context done; no stuck worker | same |
| P19-E10 | Audit query | after mutate | audit_events row; no secrets in logs | same |

Permitted fakes: loopback/test Identity JWKS, scripted model providers (not required for initial tools). **Not permitted:** fake Postgres, in-memory plan repo behind MCP E2E, hard-coded tool success without domain calls.

## Anti-Cheating Audit

Reviewers must verify:

- `/mcp` handler is not a stub returning fixed JSON without domain service calls
- Tool handlers do not import `internal/repo` directly
- Publish confirm cannot be skipped by setting a test-only env flag in production builds
- `tools/list` is not a static full inventory ignoring authz
- IDOR tests use real second workspace rows, not mocked authz true
- Origin checks are not disabled when `STUDIO_ENV=production`
- Official + external client tests both run (not only unit mocks of the SDK)
- Idempotency is durable in Postgres (`idempotency_keys`), not process memory
- Confirmation persistence and audit use the validated public `client_id`; no `azp`, internal OAuth-client UUID, or caller-supplied header substitutes for it
- No Studio token mint endpoints added under `/mcp`
- No cross-DSN usage toward LMS/Identity databases
- Contracts implementation files (OpenAPI/proto) unchanged by this phase unless a separately owned additive enum is required (prefer none)

## Completion Gate

- [ ] SDK v1.7.0 qualification evidence PROCEED with exact API names recorded
- [ ] All P19-S* scenarios green
- [ ] All P19-E* E2E green including external client
- [ ] Anti-cheating audit clean
- [ ] `make studio-test` / `make studio-cover` (≥85%) green for touched packages
- [ ] `git diff --check` clean
- [ ] LikeC4 `studio_mcp` view still validates if architecture touched
- [ ] No commit of MCP secrets; no `.env` read required for tests
- [ ] Delivery wave `S19` acceptance commands recorded

## Dependencies

- **Upstream platform:** Phases 1–8 (hard for full tools); Phase 2 authz (hard for any authenticated MCP); Phase 1 (hard for transport spike)
- **Upstream identity:** credential-free test Identity/narrow verifier is sufficient only for local transport/tests. Production delegated MCP writes and X7 require **I6 / IB2 Primer ES256/JWKS bridge + I8 / IB4 signed webhook/two-plane revocation**; require **I7 / IB3 + applicable I12 / IB8** for Identity client registration, BFF mediation, or publish-confirmation alignment. Raw Stytch bearer/SessionJWTs are never accepted.
- **IB0 candidate authority (STOP):** [`../stytch-identity-ib0/02-http-oauth-bff-mcp-contract.md`](../stytch-identity-ib0/02-http-oauth-bff-mcp-contract.md) records protected-resource metadata, exact resource→audience mapping, no DCR, the human/service authority matrix, and mandatory one-use MRTR; it is not authoritative until a fresh exact-tip review reports zero findings.
- **Upstream contracts:** Phase 12 tool-schema/conformance (parallelizable after spike; hard before claiming protocol conformance)
- **Upstream database:** D3–D6, D11–D12 paths as consumed by domain (no MCP-specific schema by default)
- **Downstream:** delivery X7 MCP conformance gate; S15 remains Primer gRPC (independent); do not block S15 on MCP unless explicitly re-sequenced

## Rollback

- Feature-flag `STUDIO_MCP_ENABLED=false` removes route registration
- No data migration to reverse if no additive MCP tables were introduced
- If additive confirmation table was added via DB track, down only in non-live envs per migration policy


## Stytch-backed identity reconciliation (authoritative)

Stytch B2B is upstream human authentication/session authority. Primer Identity is its sole SDK/API client and downstream Primer token broker: it validates opaque Stytch sessions, maps exact `(project_id, organization_id, member_id)` to a local account, and issues only short-lived single-audience Primer JWTs/JWKS. Studio, LMS, TV, MCP, product APIs, and browser JS never receive, store, forward, log, or validate a Stytch session token, SessionJWT, tuple, or role.

Stytch organization/member roles are eligibility hints only. Studio workspace membership, LMS educator roles, and TV device authentication remain local systems of record; a Stytch organization does not create a Studio tenant/workspace. Provisioning is explicit invite/admin only, and distinct cross-org tuples stay distinct personas without email merge/linking. IA is library-only and unapproved; production auth waits for IA-R and IB1–IB4, with signed webhook/cache+grant revocation as a hard BFF/MCP gate.

Required E2Es: mapped tuple to local membership; valid no-membership token denied; same-email cross-org isolation; Stytch admin-like role denied without local role; raw Stytch bearer rejected; outage fails closed/no negative cache; signed webhook forgery/replay/dedupe/out-of-order; explicit revocation bound; LMS local-role dual run; and no token/provider payload in audit logs.
