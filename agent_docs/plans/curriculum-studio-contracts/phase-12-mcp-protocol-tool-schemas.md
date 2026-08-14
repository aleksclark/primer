# Phase 12: MCP protocol, tool schemas, and conformance

**File:** `phase-12-mcp-protocol-tool-schemas.md`
**Depends on:** Phase 8 (auth/errors/idempotency semantics patterns); Phase 10–11 gates patterns; platform MCP transport qualification (platform Phase 19 spike)
**Duration guess:** 4–6 days
**Handoff wave:** delivery `C12`
**Design SoT:** [`../curriculum-studio-mcp-design.md`](../curriculum-studio-mcp-design.md)

## Goal

Establish Curriculum Studio MCP as a **third contract surface** with a clear source of truth: the **pinned official MCP specification** plus **code-defined tool schemas** in the Studio MCP adapter. Deliver schema/inventory freeze, conformance tests (official SDK client + one external Streamable HTTP client), and negative protocol/authz cases — **without** duplicating OpenAPI or protobuf DTOs and without owning domain business logic.

## Scope

### In scope

- Document and enforce SoT split: OpenAPI (authoring REST) ∥ protobuf (Primer/machine) ∥ **MCP tools (spec + code schemas)**
- Pin references: MCP revision **`2026-07-28`**, Go SDK **`github.com/modelcontextprotocol/go-sdk` v1.7.0**
- Tool inventory freeze matching design doc (names, authz class, input/output JSON Schema locations)
- Conformance harness invoking real `/mcp` (process or platform-provided loopback) via:
  - official Go SDK client
  - at least one external Streamable HTTP client (Hermes/mcporter or equivalent)
- Negative matrix: protocol/header mismatch, Origin rejection, wrong audience, IDOR handle, idempotent replay, concurrent patch conflict, publish confirmation required, disconnect/cancel
- Stable machine-readable coverage output mapping REQ-MCP-* → E2E IDs
- Guidance for exclusive consumption: agent clients should use MCP session/tool APIs, not ad-hoc JSON-RPC forks

### Out of scope

- Implementing domain plan/validate/publish business rules (platform)
- Changing hand OpenAPI YAML or `.proto` files for MCP DTOs (forbidden duplication)
- Identity OP implementation (Identity plan)
- Committing generated SDK dumps or large golden SSE captures with secrets
- Roots/sampling/logging capability surface

## BDD Success Criteria

#### Scenario: P12-S1 — SoT non-overlap documented and gated

- **Given** the contracts README and OWNERS (or equivalent) after this phase
- **When** a reviewer inspects ownership tables
- **Then** MCP is listed as a third surface with SoT = pinned MCP spec + code-defined tool schemas
- **And** a gate fails if a PR adds MCP request/response types into OpenAPI or protobuf solely to mirror tools

#### Scenario: P12-S2 — Tool inventory freeze

- **Given** the frozen tool list in design doc §5 and code schema registry
- **When** conformance lists tools via official client as an authorized author
- **Then** names match the freeze set (allowing authz subset)
- **And** each tool has input and output JSON Schema used for `structuredContent` validation
- **And** list order is deterministic for the same principal

#### Scenario: P12-S3 — Official SDK client conformance tour

- **Given** loopback Studio with test JWKS and seeded workspace data
- **When** the official Go SDK Streamable HTTP client runs initialize → tools/list → read tool → draft patch → validate
- **Then** all steps succeed with schema-valid `structuredContent`
- **And** no implicit MCP session resumption is required between calls (opaque ids only)

#### Scenario: P12-S4 — External client conformance

- **Given** the same loopback endpoint and token
- **When** Hermes/mcporter (or equivalent) performs initialize, tools/list, and one read tool
- **Then** results agree with the official client on tool names and primary read payload fields

#### Scenario: P12-S5 — Protocol and header negatives

- **Given** conformance attacker fixtures
- **When** clients send unsupported `MCP-Protocol-Version`, mismatched method headers, or disallowed Origin
- **Then** server rejects without executing domain mutations
- **And** error bodies are generic (no stack traces, no tokens)

#### Scenario: P12-S6 — Authz and tenancy negatives

- **Given** two workspaces and tokens for subject A (member of W1 only)
- **When** A calls tools with W2 opaque draft ids or uses a JWT with `aud=primer-lms`
- **Then** calls fail closed (401/403/not-found)
- **And** no W2 graph payload is returned

#### Scenario: P12-S7 — Idempotency, concurrency, publish confirmation

- **Given** author token on a draft
- **When** patch_graph is replayed with the same idempotency key
- **Then** domain state matches single-apply
- **When** two conflicting versions patch concurrently
- **Then** one conflict is observed and the graph remains valid
- **When** publish.confirm runs without elicitation/step-up
- **Then** conformance asserts non-published revision + required confirmation error/elicitation

#### Scenario: P12-S8 — Disconnect/cancel

- **Given** a long tool that emits request-scoped progress/SSE
- **When** the client disconnects mid-request
- **Then** the conformance harness observes cancellation semantics (no hang past timeout; server accepts subsequent requests)

#### Scenario: P12-S9 — Coverage matrix complete

- **Given** REQ-MCP-* IDs in the contracts index
- **When** the conformance runner emits coverage JSON
- **Then** every REQ-MCP-* maps to ≥1 passed E2E id
- **And** orphan E2E ids without REQ mapping fail the coverage check

## Implementation Instructions

1. **Documentation**
   - Update `curriculum-studio/contracts/README.md` ownership table with MCP third surface (docs-only is OK in the planning commit; code gate lands with implementation wave).
   - Cross-link `agent_docs/plans/curriculum-studio-mcp-design.md`.

2. **Schema registry (code, when implementing)**
   - Prefer Go structs + JSON Schema export beside `internal/mcp/tools`.
   - Do **not** add parallel schemas under `contracts/openapi` or `contracts/proto`.
   - Optional: `contracts/mcp/README.md` pointing at code paths + pin versions (no duplicated JSON Schema trees unless generated from code in CI).

3. **Conformance package**
   - `curriculum-studio/internal/mcp/conformance/` or `curriculum-studio/internal/conformance/mcp/`.
   - Boot or attach to platform test server with `/mcp`.
   - Subtests named with stable E2E IDs below.

4. **Pins**
   - Go module require `github.com/modelcontextprotocol/go-sdk v1.7.0`.
   - Record MCP protocol version string `2026-07-28` in test helpers.

5. **External client**
   - Script target `make studio-mcp-external-e2e` invoking mcporter/Hermes against `$STUDIO_MCP_URL`.
   - Credential-free: fixture JWT from test Identity/JWKS.

6. **Planted red**
   - Once: mutate a tool schema incompatibly and prove conformance fails; restore.

## End-to-End Test Plan

| ID | Setup | Action | Assert |
| --- | --- | --- | --- |
| E12-01 | docs/gates | ownership scan | MCP not in OpenAPI/proto DTO mirror |
| E12-02 | official SDK | initialize+list | deterministic authz list |
| E12-03 | official SDK | read+draft+validate tour | schema-valid structuredContent |
| E12-04 | external client | list+read | interoperable |
| E12-05 | bad version/Origin | POST | reject |
| E12-06 | wrong aud + IDOR | tools/call | fail closed |
| E12-07 | idempotent patch + concurrent conflict | tools/call | single-apply + conflict |
| E12-08 | publish.confirm w/o step-up | tools/call | not published |
| E12-09 | disconnect | long tool | cancel/timeout safe |
| E12-10 | coverage emitter | run matrix | all REQ-MCP-* covered |

## Anti-Cheating Audit

- Conformance must not call tool functions in-process while claiming HTTP Streamable transport proof
- External client test must not be skipped silently in CI when the tool is installed in the job image (mark BLOCKED only with explicit missing-binary reason)
- Schema freeze must not be a markdown-only list without code registry validation
- Must not restate MaterializationContext in MCP schemas
- Must not weaken OpenAPI/proto parity gates to “make room” for MCP
- Publish confirmation test must inspect DB revision status, not only tool text

## Completion Gate

- [ ] P12-S* green
- [ ] E12-* green or explicitly BLOCKED with named external dependency
- [ ] Coverage JSON complete for REQ-MCP-*
- [ ] Planted red recorded once
- [ ] Anti-cheat clean
- [ ] No OpenAPI/proto DTO duplication introduced
- [ ] `git diff --check` clean

## Dependencies

- **Upstream:** contracts Phases 8–11 patterns; platform Phase 19 handler (SOFT for doc freeze; HARD for runtime conformance)
- **Sibling:** Identity protected-resource / client registration for MCP OAuth clients
- **Downstream:** delivery X7; platform Phase 19 completion evidence

## Rollback

- Disable MCP conformance Makefile target; leave REST/gRPC gates untouched
