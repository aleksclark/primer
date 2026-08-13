# Phase 17: Collaborative authoring

**File:** `phase-17-collaborative-authoring.md`
**Depends on:** Phases 10 and 16
**Duration guess:** 7–10 days
**Handoff wave:** see index orchestrator map

## Goal

Add collaborative authoring: comments, approval workflow, revision comparison diff, reusable unit library, plan templates, organization policies, and selective curriculum sharing across workspaces with authz checks — product roadmap Phase 5.

## Scope

### In scope

- Comments threads on plan nodes/items (additive schema via db track)
- Approval state machine on draft (reviewer role)
- Revision diff API + UI
- Unit library copy-into-revision
- Templates for brief types from product plan
- Share grant table workspace-to-workspace read-only
- Policy knobs stored per workspace (JSON) without Identity involvement

### Out of scope / YAGNI

- Real-time CRDT multiplayer cursors
- SCIM org provisioning
- Public internet anonymous catalog

## BDD Success Criteria

#### Scenario: P17-S1 — Comments and approval

- **Given** author+reviewer memberships
- **When** comment on outcome; reviewer approves draft
- **Then** comment persists with author name
- **And** approval recorded
- **And** author notified via event optional

#### Scenario: P17-S2 — Revision diff

- **Given** two revisions
- **When** GET diff
- **Then** shows added/removed outcomes by name
- **And** UI renders diff

#### Scenario: P17-S3 — Unit library and templates

- **Given** saved unit library entry + template
- **When** create curriculum from template; import unit
- **Then** new draft populated
- **And** names preserved

#### Scenario: P17-S4 — Selective sharing

- **Given** W1 shares curriculum read-only to W2
- **When** W2 user lists/gets
- **Then** can read
- **And** cannot mutate
- **And** unshared W3 denied

## Implementation Instructions

1. File additive migrations (comments, approvals, shares, templates) through db track before coding.
2. Contracts additive OpenAPI paths via contracts track.
3. Diff algorithm server-side on graph tables.
4. Browser E2E multi-user using two test sessions (Phase 2 mint).
5. House UI for comment inspector drawer.

## End-to-End Test Plan

#### P17-E1 — Comment+approve

- **Setup:** two users browser or API
- **Action:** flow
- **Assert:**
  - states
- **Command:** `make studio-e2e --grep collab`

#### P17-E2 — Diff

- **Setup:** two revs
- **Action:** diff API/UI
- **Assert:**
  - name-level changes
- **Command:** `make studio-test`

#### P17-E3 — Template+library

- **Setup:** fixtures
- **Action:** create from template
- **Assert:**
  - structure
- **Command:** `make studio-e2e --grep template`

#### P17-E4 — Share isolation

- **Setup:** three workspaces
- **Action:** share matrix
- **Assert:**
  - read/deny/mutate deny
- **Command:** `go test ./internal/plan -run Share`

## Anti-Cheating Audit

- Sharing not client-side hide only
- Approvals not forgeable by author without reviewer role
- Diff not always empty
- Multi-user E2E must use two principals

## Completion Gate

- [ ] Db/contract sibling PR merged or blocker closed
- [ ] P17 tests green
- [ ] Roadmap Phase 5 evidenced


## Dependencies

- Upstream: Phases 10 and 16
- Sibling tracks: schema (`curriculum-studio/db`), contracts (`curriculum-studio/contracts`), identity (when leaving test auth)
- Downstream consumers: later phases listed in index

## Rollback

- Revert the phase PR(s); drop any additive migrations only via forward-fix sibling db track (never destructive rollback in prod without backup).
- Feature flags: prefer `STUDIO_*` env gates over half-applied routes.
- SPA: revert `curriculum-studio/web` routes; keep generated client in sync with OpenAPI.
