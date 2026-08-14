# Phase 3: Workspaces API

**File:** `phase-03-workspaces-api.md`
**Depends on:** Phase 2
**Duration guess:** 3–5 days
**Handoff wave:** see index orchestrator map

## Goal

Expose workspace and membership authoring APIs backed by real Studio Postgres tables (`tenants`, `workspaces`, `workspace_memberships`), with server-side list operations and human-readable names as primary display fields. Establishes CRUD+list patterns reused by all later resources.

## Scope

### In scope

- Huma handlers for workspaces list/create/get/update per OpenAPI operationIds
- Tenant create-on-first-workspace or explicit tenant binding policy (document choice: auto-create personal tenant for first workspace)
- Membership list/add/update/revoke endpoints (if not in OpenAPI yet, additive contract via contracts track — minimum: create workspace adds owner membership for caller)
- `api.RegisterCRUD`-style list: `q`, `sort`, `dir`, `limit`, `offset` server-side
- Prefixed opaque API IDs (`ws_…`) mapped from UUID PKs
- Repo layer `internal/repo` + domain types
- Factories in testutil

### Out of scope / YAGNI

- Full admin IdP directory sync
- SPA pages (Phase 9)
- Billing enforcement beyond status field

## BDD Success Criteria

#### Scenario: P3-S1 — Create workspace

- **Given** authenticated author subject with no workspace
- **When** POST /workspaces with name and kind
- **Then** 201 with id ws_* and name
- **And** row in workspaces + owner membership for subject_ref
- **And** tenant row exists

#### Scenario: P3-S2 — List workspaces server-side

- **Given** ≥3 workspaces for subject; one name unique substring
- **When** GET /workspaces?q=…&limit=1&sort=name&dir=asc
- **Then** only membership-visible workspaces returned
- **And** page size respected
- **And** filter applied in SQL not client
- **And** primary field name populated

#### Scenario: P3-S3 — Owner adds membership

- **Given** workspace owner session
- **When** add membership for identity:<uuid> role=author
- **Then** membership active
- **And** new subject can list workspace

#### Scenario: P3-S4 — Revoked membership loses access

- **Given** subject had access then revoked
- **When** subject lists or gets workspace
- **Then** workspace absent or 403/404
- **And** mutations fail

#### Scenario: P3-S5 — Names lead payloads

- **Given** workspace with name “Oak Family Homeschool”
- **When** list and get
- **Then** JSON `name` present and equal
- **And** id is secondary field not substitute for name

## Implementation Instructions

1. Map OpenAPI schemas Workspace/WorkspaceCreate/Page to Huma types.
2. Repository methods using squirrel or pgx SQL; always scope by membership join.
3. On create: insert tenant (if needed), workspace, owner membership in one transaction.
4. ID codec: uuid ↔ `ws_` prefix (and `ten_` if exposed).
5. Validation: kind enum per SCHEMA; status active/archived.
6. Audit event on create/update/membership change.
7. After handlers exist, note OpenAPI drift process: either keep hand YAML until `cmd/openapi-gen` (Phase 9/10) or generate early — document chosen path; contracts track owns breaking changes.
8. Tests: isolation, pagination plant >page size rows.

## End-to-End Test Plan

#### P3-E1 — Workspace CRUD+list

- **Setup:** auth test session + Postgres
- **Action:** create three, list with q/limit/sort
- **Assert:**
  - pagination
  - q matches name
  - foreign workspace excluded
- **Command:** `make studio-test`

#### P3-E2 — Membership lifecycle

- **Setup:** owner + second identity
- **Action:** add author; act as author; revoke
- **Assert:**
  - access then deny
  - DB status revoked
- **Command:** `go test ./internal/api -run Membership`

#### P3-E3 — Name fields

- **Setup:** named workspace
- **Action:** list/get JSON
- **Assert:**
  - name primary
  - id prefixed
- **Command:** `go test ./internal/api -run WorkspaceName`

## Anti-Cheating Audit

- List handler must not load all rows into memory then filter
- Membership checks not skipped on get-by-id
- No write without transaction around workspace+membership
- Opaque IDs not raw uuid strings if contract requires prefixes

## Completion Gate

- [ ] OpenAPI operationIds listWorkspaces/createWorkspace/getWorkspace/updateWorkspace satisfied
- [ ] P3 scenarios/E2E green
- [ ] Cover gate not regressed below threshold for new packages


## Dependencies

- Upstream: Phase 2
- Sibling tracks: schema (`curriculum-studio/db`), contracts (`curriculum-studio/contracts`), identity (when leaving test auth)
- Downstream consumers: later phases listed in index

## Rollback

- Revert the phase PR(s); drop any additive migrations only via forward-fix sibling db track (never destructive rollback in prod without backup).
- Feature flags: prefer `STUDIO_*` env gates over half-applied routes.
- SPA: revert `curriculum-studio/web` routes; keep generated client in sync with OpenAPI.
