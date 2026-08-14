# Phase 12: Studio integration

**File:** `phase-12-studio-integration.md`
**Depends on:** Phases 3, 7, 8
**Duration guess:** 4–6 days
**Migration stage:** S2
**Handoff wave:** W6

## Goal

> Integration freeze: Studio validates Identity JWTs only — Studio is **never** an auth/session issuer. Credential-free Studio E2E uses this service's loopback/test OP (or identical verifier path), not a Studio-local token mint.

Wire Curriculum Studio (planned platform module) to consume Identity as JWKS issuer and OIDC OP for `studio-bff`: validate `aud=curriculum-studio`, map `sub` → `workspace_memberships.subject_ref` as `identity:<uuid>`, service callers as `identity:svc:<id>`. Prove cross-audience rejection and that Identity tokens never convey Studio roles. Do **not** duplicate Studio API schema.

**MCP extension (same phase family / delivery I12 + S19 coordination):** register or document OAuth clients used by MCP agents; support protected-resource discovery that points MCP clients at Identity for tokens with MCP scopes (`studio:mcp`, `studio:read`, `studio:draft`, `studio:publish` as frozen). Studio `/mcp` remains validator-only. Identity still does **not** own Studio tool authz or workspace roles. See [`../curriculum-studio-mcp-design.md`](../curriculum-studio-mcp-design.md).

## Scope

### In scope

- Shared verification notes + optional small package path decision:
  - Prefer Studio copies verifier pattern from `primer-identity` docs/tests **or** tiny `pkg/identitytoken` under identity module importable by Studio
- Studio config: `STUDIO_IDENTITY_ISSUER`, `STUDIO_JWKS_URL`, `STUDIO_AUTH_MODE=jwks` replacing pure test mode when Identity up
- Integration test harness: run Identity testcontainer stack + Studio authz tests (may live in Studio module once exists; until then identity repo hosts `tests/studio_consumer` httptest double enforcing memberships)
- subject_ref formatting helpers
- Negative: primer-lms aud rejected by Studio validator; missing membership 403; no role claims trusted from JWT
- Document S2 feature flag path
- MCP client/resource registration notes + scopes for Streamable HTTP MCP consumers (no Studio token endpoint)

### Out of scope

- Studio workspace CRUD features (platform plan)
- Rewriting Studio OpenAPI
- LMS changes (Phase 13)
- Implementing Studio `/mcp` handler (platform Phase 19)
- Studio product authz rules inside Identity

## BDD Success Criteria

#### Scenario: P12-S1 — Studio accepts Identity JWT

- **Given** Identity JWKS and membership subject_ref `identity:<sub>`
- **When** Studio API called with Bearer access token aud=curriculum-studio
- **Then** 200 on authorized probe
- **And** principal subject_ref matches

#### Scenario: P12-S2 — Wrong audience rejected

- **Given** valid JWT aud=primer-lms
- **When** presented to Studio
- **Then** 401
- **And** no workspace data

#### Scenario: P12-S3 — Service subject mapping

- **Given** service principal JWT scope studio:materialize
- **When** machine probe
- **Then** subject_ref `identity:svc:<id>`
- **And** scope enforced locally

#### Scenario: P12-S4 — Roles in JWT ignored

- **Given** attacker-crafted claim `roles=["owner"]` if somehow signed by test key without membership
- **When** Studio authorizes
- **Then** still denied without membership row
- **And** membership DB is SoT

#### Scenario: P12-S5 — MCP audience and scopes issued by Identity

- **Given** an OAuth client registered for MCP (user-delegated or service) with `aud=curriculum-studio` and MCP scopes
- **When** the client obtains an access token from Identity (not Studio)
- **Then** the JWT validates on Studio JWKS path with exact audience
- **And** Identity does not embed workspace role grants
- **And** token revoke/expiry is enforced on subsequent Studio MCP requests without Identity owning tool authz

## Implementation Instructions

1. Coordinate with platform plan Phase 2 and Phase 19; if Studio module absent, ship consumer double + skip-integrate tag.
2. Do not add credential tables to Studio DB.
3. BFF login uses studio-bff client from Phase 7 seeds.
4. E2E command may be `make identity-studio-consumer-test`.
5. File blocker if platform authz not merged — gate remains explicit.
6. Document MCP protected-resource pointer + client registration alongside studio-bff; keep AS endpoints on Identity only.

## End-to-End Test Plan

| ID | Setup | Action | Assert | Command |
| --- | --- | --- | --- | --- |
| P12-E1 | Identity + Studio validator double | Bearer studio aud | 200 + subject_ref | consumer test |
| P12-E2 | lms aud token | Studio call | 401 | consumer test |
| P12-E3 | service JWT | machine probe | svc subject + scope | consumer test |
| P12-E4 | MCP-scoped token from Identity | Studio MCP auth path / validator probe | aud+scopes accepted; roles ignored | consumer test |

## Anti-Cheating Audit

- Studio must not trust `X-User-Id`.
- Membership check hits Studio DB (or double’s DB), not JWT roles.
- No email-based membership bind.

## Completion Gate

- [ ] P12-S*/E* green or BLOCKED on missing Studio module with filed issue
- [ ] S2 path documented
- [ ] Anti-cheat clean

## Dependencies

- Upstream: 3, 7, 8
- Sibling: curriculum-studio-plan-platform
- Downstream: S2 production enable

## Rollback

- Studio `AUTH_MODE=test` local only; disable Identity issuer config.
