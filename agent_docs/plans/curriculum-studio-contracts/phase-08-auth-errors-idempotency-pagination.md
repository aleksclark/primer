# Phase 8: Auth, errors, idempotency, pagination semantics

## Goal

Implement and prove cross-cutting contract semantics on both surfaces: JWT/JWKS
auth presentation and scopes, shared ErrorCode mapping, idempotency keys, and
transport-specific pagination — via adapters/harnesses and generated-client E2E.

**Depends on:** Phases 5 and 7
**Duration guess:** 3–5 days

## BDD Success Criteria

#### Scenario: P8-S1 — REST Bearer JWT with audience curriculum-studio

- **Given** fixture JWKS and valid access token `aud=curriculum-studio`
- **When** generated REST client calls a protected authoring route
- **Then** request succeeds
- **And** token with `aud=primer-lms` is rejected as unauthenticated/forbidden
  per policy

#### Scenario: P8-S2 — gRPC authorization metadata

- **Given** same fixture issuer
- **When** generated gRPC client attaches `authorization: Bearer <jwt>`
- **Then** integration RPC succeeds
- **And** missing/invalid metadata fails with `ERROR_CODE_UNAUTHENTICATED`

#### Scenario: P8-S3 — Scope metadata enforced on service calls

- **Given** client_credentials-style token with scopes
- **When** Materialize is called without `materialize:write` (or documented
  scope name)
- **Then** permission denied
- **And** with scope, succeeds
- **And** product workspace authz remains local (membership check harness)

#### Scenario: P8-S4 — Migration X-Service-Token alias

- **Given** migration mode enabled in harness config
- **When** caller sends `X-Service-Token: <JWT>` without Authorization
- **Then** accepted with metric/log “migration alias”
- **And** when migration mode disabled, alias rejected
- **And** static legacy secret path is separate dual-run flag default off in
  tests unless explicitly testing sunset

#### Scenario: P8-S5 — Typed errors on REST and gRPC

- **Given** harness triggers not_found, validation_failed, resource_locked,
  revision_immutable, idempotency_key_conflict
- **When** clients call
- **Then** HTTP problem+json includes `code` matching ErrorCode wire strings
- **And** gRPC details include ErrorDetail with same codes
- **And** parity gate includes ErrorCode set

#### Scenario: P8-S6 — Idempotency for Materialize, PublishRevision, Export

- **Given** REST headers `Idempotency-Key` and gRPC `idempotency_key`
- **When** duplicate submits occur
- **Then** first response is replayed for equivalent payloads
- **And** conflicting payloads return `idempotency_key_conflict`
- **And** missing key on required ops returns `invalid_argument`

#### Scenario: P8-S7 — Pagination semantics

- **Given** > page size resources in harness
- **When** REST lists with limit/offset and gRPC lists with page tokens
- **Then** full iteration returns all ids without duplicates
- **And** totals behave per contract (0 means omitted on gRPC when unknown)

## Implementation Instructions

1. **`internal/authn`:**
   - JWKS provider interface; fixture file provider for tests
   - Validate `iss`, `aud`, `exp`, `nbf`, `kid`, signature
   - Extract `sub` → `identity:<uuid>` / `identity:svc:<id>` mapping helper
   - Scope split on space

2. **Huma middleware / gRPC interceptors** share validation core.

3. **Error mapper** single table code → HTTP status → gRPC code.

4. **Idempotency store** interface; memory impl for harness; document DB table
   ownership remains with DB plan (`idempotency_keys` if present — check
   migrations; if absent, harness-only until DB plan adds).

5. **Pagination helpers** separate REST vs gRPC packages.

6. **OpenAPI security schemes** emitted: bearerAuth primary; serviceCredential
   documented as migration.

7. **Identity interface appendix update:** exact claims list consumed.

### Focused verification

- table-driven authn tests
- client E2E for REST+gRPC error/idempotency/page

## End-to-End Test Plan

| ID | Setup | Action | Assert |
| --- | --- | --- | --- |
| E8-01 | fixture JWT aud studio | REST protected call | 200 |
| E8-02 | JWT aud lms | REST call | 401/403 |
| E8-03 | gRPC with/without bearer | Materialize | success/unauthenticated |
| E8-04 | scope matrix | Materialize | deny/allow |
| E8-05 | migration on/off | X-Service-Token | accept/reject |
| E8-06 | error matrix | REST+gRPC | codes match wire |
| E8-07 | idempotency | publish/materialize/export | replay + conflict |
| E8-08 | pagination | list endpoints both transports | complete iteration |

## Anti-Cheating Audit

- Auth only on REST not gRPC (or reverse)
- Accepting any JWT without aud check
- Idempotency always creating new ids
- Error bodies as plain text without code
- Pagination tested with single-page only
- Claiming live Identity complete from fixture JWKS

## Completion Gate

- [ ] P8-S1…P8-S7 pass
- [ ] E8-01…E8-08 evidence
- [ ] Security schemes present in emitted OpenAPI
- [ ] Identity plan interface claims list updated
- [ ] Migration flags default safe

## Dependencies and rollback

- **Depends on:** Phase 5, 7
- **Unblocks:** Phases 9–11
- **Rollback:** disable migration alias; keep strict Bearer only


## Stytch-backed identity reconciliation (authoritative)

Stytch B2B is upstream human authentication/session authority. Primer Identity is its sole SDK/API client and downstream Primer token broker: it validates opaque Stytch sessions, maps exact `(project_id, organization_id, member_id)` to a local account, and issues only short-lived single-audience Primer JWTs/JWKS. Studio, LMS, TV, MCP, product APIs, and browser JS never receive, store, forward, log, or validate a Stytch session token, SessionJWT, tuple, or role.

Stytch organization/member roles are eligibility hints only. Studio workspace membership, LMS educator roles, and TV device authentication remain local systems of record; a Stytch organization does not create a Studio tenant/workspace. Provisioning is explicit invite/admin only, and distinct cross-org tuples stay distinct personas without email merge/linking. IA is library-only and unapproved; production auth waits for IA-R and IB1–IB4, with signed webhook/cache+grant revocation as a hard BFF/MCP gate.

Required E2Es: mapped tuple to local membership; valid no-membership token denied; same-email cross-org isolation; Stytch admin-like role denied without local role; raw Stytch bearer rejected; outage fails closed/no negative cache; signed webhook forgery/replay/dedupe/out-of-order; explicit revocation bound; LMS local-role dual run; and no token/provider payload in audit logs.
