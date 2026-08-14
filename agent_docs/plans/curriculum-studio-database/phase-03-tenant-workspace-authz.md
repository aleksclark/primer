# Phase 3: Tenant and workspace authorization projections

**Depends on:** Phase 2
**Duration guess:** 3–4 days
**Tables:** `tenants`, `workspaces`, `workspace_memberships`, `integration_identities`

## Goal

Implement durable repositories for multi-tenant authoring boundaries and membership projections keyed by opaque Identity subject refs. Prove tenant isolation (IDOR defenses at SQL), service vs human subjects, and integration identity snapshots without any cross-database access or credential storage.

## BDD Success Criteria

#### Scenario: P3-S1 — Create and get tenant/workspace

- **Given** a migrated Studio DB
- **When** a caller creates tenant `acme` and workspace `hs-g6` kind `family`
- **Then** rows persist with status defaults and timestamps
- **And** unique `(tenant_id, slug)` is enforced

#### Scenario: P3-S2 — Workspace list is tenant-scoped

- **Given** two tenants each with workspaces
- **When** listing workspaces for tenant A
- **Then** only A’s workspaces return

#### Scenario: P3-S3 — Human membership uses identity UUID ref

- **Given** workspace W
- **When** adding membership `subject_ref=identity:<uuid>`, role `author`, status `active`
- **Then** `GetActiveMembership(W, subject_ref)` returns the role
- **And** duplicate `(workspace_id, subject_ref)` is rejected

#### Scenario: P3-S4 — Service principal membership

- **Given** workspace W
- **When** adding `subject_kind=service`, `subject_ref=identity:svc:primer-lms`
- **Then** row stores without passwords/tokens
- **And** schema/repo reject empty subject_ref

#### Scenario: P3-S5 — Integration identity snapshot is opaque and private

- **Given** workspace W
- **When** upserting `system=primer_lms`, `external_kind=learner`, `external_ref=learner_456`, snapshot JSON
- **Then** unique key holds; `external_ref` stored as text only
- **And** no code path opens LMS DSN to resolve the learner
- **And** snapshot JSON may hold display fields but repo APIs do not require email and must not log full snapshots at info level

#### Scenario: P3-S6 — Cross-tenant IDOR read denied

- **Given** resource IDs from tenant B
- **When** a tenant-A-scoped GetWorkspace/GetMembership is invoked with B’s UUID
- **Then** the repo returns not-found (not the row)
- **And** queries include tenant/workspace predicates

## Implementation Instructions

1. Domain types mapping pgx `db` tags for the four tables (mirror LMS `domain` style inside Studio).
2. `TenantRepo`, `WorkspaceRepo`, `MembershipRepo`, `IntegrationIdentityRepo` with Create/Get/List/Update status/Upsert patterns.
3. `MembershipRepo.GetActiveMembership(ctx, workspaceID, subjectRef)` filters `status='active'`.
4. Normalize subject_ref helpers: validate prefix `identity:` / `identity:svc:` at app layer (DB remains opaque text).
5. All list/get methods take `workspaceID` or `tenantID` as required first-class args—no global get-by-id without scope for authorization-sensitive entities (if GetByID exists, pair with workspace check).
6. Extend factory.
7. Tests use harness Tx; assert CHECK violations via real inserts where useful.
8. Out of scope: JWT parsing, invite emails, HTTP middleware.

## End-to-End Test Plan

| ID | Setup | Action | Assertions |
| --- | --- | --- | --- |
| P3-E1 | harness | create tenant+workspace | round-trip fields |
| P3-E2 | two tenants | list by tenant | isolation |
| P3-E3 | workspace | add human membership | get active; unique conflict |
| P3-E4 | workspace | service membership | kind/role CHECKs |
| P3-E5 | workspace | integration identity upsert twice | update snapshot; still one row; rg codebase for LMS DSN usage in repo = 0 |
| P3-E6 | two workspaces | get with wrong workspace id | ErrNotFound |

```bash
cd curriculum-studio && go test ./internal/repo/... -count=1 -run Tenant|Workspace|Membership|Integration
cd curriculum-studio/db && python3 -m pytest tests -q -k membership
```

## Anti-Cheating Audit

- IDOR tests must use two real workspaces and assert SQL-level filtering, not only handler mocks (no handlers yet—repo is the boundary).
- Grep production packages for `password`, `bcrypt`, `refresh_token` columns—must be absent.
- Integration tests must not call Identity HTTP.
- Do not store raw OAuth tokens in `snapshot`.

## Completion Gate

- [ ] P3-S1–P3-S6 pass with P3-E1–P3-E6
- [ ] Factory wired
- [ ] Anti-cheating audit clean
- [ ] SCHEMA subject_ref conventions honored

## Dependencies and rollback

- **Depends on:** Phase 2
- **Rollback:** drop repo files; tables remain via migrations
- **Enables:** Phase 4+ workspace-scoped catalogs and plans
