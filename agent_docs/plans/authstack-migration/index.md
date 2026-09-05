# Primer authstack migration

## Outcome

Primer authenticates human/admin browser users and service-to-service HTTP, gRPC, and MCP callers through `git.clark.team/aleksclark/authstack`, with provider construction confined to process composition roots and authorization retained in each product. Primer Identity, Stytch broker/JWKS code, handwritten JWT validators, and shared service secrets are retired only after dual-run parity and rollback gates pass.

Student and TV device pairing is deliberately unchanged: pairing codes, opaque device tokens, device-to-student/TV bindings, revocation, re-pairing, and local systems of record never move into authstack.

This plan is provider-gated. Primer currently uses Stytch, while authstack supports Clerk and ZITADEL. No repository evidence at the pre-wave-00 baseline selects one. Phase 1 must record an explicit human decision or unambiguous integrated architecture decision before any provider-specific code or destructive Identity cutover. Until then, only additive provider-neutral contracts, inventories, and tests may land.

## Current-state summary

Evidence below was refreshed after `paseo wait b6ce6b31-ea58-4f00-935d-d71704fc7e16` completed. Wave 00 head `51656816f7954ac1f61ca751b8ffc47eb4caa983` merged through PR #70 as protected-master commit `cac90a151459a2fc60d506839424155f7f87ef80`, and this initiative is based exactly there. That merge contains the WIP integration plan and Primer Tasks plans, but **not the `primer-tasks/` implementation**: WIP phases 3–13 remain unexecuted and `primer-tasks/` is absent from the tree. Consequently this is still a pre-Tasks production baseline. The inventory uses current code plus read-only donor evidence, but no production authstack implementation may begin until the Tasks implementation is integrated and an exact successor master tip is published.

### Existing human/admin authentication

| Product boundary | Existing behavior | Repository evidence | Missing/end-state change |
|---|---|---|---|
| LMS parent/admin API and SPA | `ParentSessionGuard` accepts a product-local opaque parent session or a Primer Identity ES256 JWT, then loads an LMS educator and enforces local `parent`/`admin`. `/auth/login` validates LMS password hashes. The SPA stores the bearer in `localStorage`. | `server/internal/api/auth_parent.go`, `server/internal/repo/parent_auth.go`, `web/src/api/auth.ts`, `web/src/components/parent-token-gate.tsx`, `server/internal/api/dual_auth_test.go` | Provider-backed BFF/session transition; canonical `(issuer, subject)` mapping; no browser bearer storage; retain LMS educator role as authorization SoT. |
| TV admin API and SPA | Admin routes accept either any valid Primer Identity JWT for the TV audience or a shared `X-Admin-Key`/opaque bearer. There is no local role lookup for a human JWT. | `server/internal/tv/api/auth.go`, `server/internal/tv/api/identity_auth_test.go`, `tv-web/src/api/auth.ts` | Separate human and machine policies, product-local admin membership, provider-backed BFF, removal of shared-key/browser key path. |
| Curriculum Studio REST/BFF | Handwritten JWKS validator produces `authn.AuthContext`; REST loads product-local workspace memberships. Existing BFF performs OAuth code+PKCE and stores access/refresh tokens server-side, but defaults to an in-memory store. | `curriculum-studio/internal/authn/`, `curriculum-studio/internal/api/auth.go`, `curriculum-studio/internal/bff/`, `curriculum-studio/internal/app/app.go` | Authstack principal/authenticator at composition root, durable provider-appropriate BFF sessions, canonical membership migration, removal of handwritten validator. |
| Studio MCP | Streamable HTTP extracts and validates a bearer independently, loads Studio memberships, and filters tools by local membership, principal kind, and scopes. | `curriculum-studio/internal/mcp/handler.go`, `curriculum-studio/internal/mcp/authz.go`, `curriculum-studio/internal/mcp/principal.go` | Explicit authstack OAuth/M2M policy and `auth.Authorizer`; retain local workspace/tool authorization and fail-closed tool policy. |
| Primer Agents | Global middleware uses a handwritten Primer Identity JWKS validator and custom principal; route middleware and profile admission enforce scopes. Human and service tokens share the route group. | `primer-agents/internal/authn/`, `primer-agents/internal/api/api.go`, `primer-agents/internal/api/handlers.go`, `primer-agents/internal/profile/admission.go` | Authstack principal, explicit per-boundary credential kinds/audience, authstack authorizer while preserving owner/profile policy. |
| Primer Tasks parent/BFF | Incoming wave-00 Tasks has a parent OIDC/BFF flow and `tasks-test-issuer`; exact files and persisted identity fields must be re-inventoried after integration. | Incoming `primer-tasks/cmd/tasks-test-issuer/`, `primer-tasks/internal/api/`, `primer-tasks/internal/config/`, `primer-tasks/web/` | Provider-backed parent BFF and canonical mapping without changing browser/Android student pairing. Test issuer remains test-only. |
| Primer Identity/Stytch | Separate service owns Stytch broker callbacks, exact Stytch tuple mappings, Primer OAuth grants, JWKS/signing keys, service principals, webhooks, and generated client. | `primer-identity/`, `agent_docs/plans/stytch-identity-ib0/`, `agent_docs/plans/primer-identity-service/` | Dual-run source and rollback authority first; eventually retire after all browser and service consumers have migrated and rollback window closes. |

### Existing service authentication

| Direction/boundary | Existing credential | Repository evidence | End state |
|---|---|---|---|
| TV reporter → LMS instruction ingest | Shared `X-Service-Token` | `server/internal/api/instruction_logs.go`, `server/internal/tv/primer/primer.go` | M2M token for LMS-only audience and `instruction-logs:ingest` permission. |
| Content ingest (and other operators) → TV admin | Shared `X-Admin-Key` | `server/internal/ingest/tvclient/tvclient.go`, `server/internal/tv/api/auth.go` | Distinct M2M identity/audience/permission; not accepted as a human session or device token. |
| LMS → Primer Agents | Environment-selected pre-minted Identity access token | `server/internal/remoteagent/adapter.go` | `oauth.ClientCredentialsTokenSource` and authstack outbound transport for `primer-agents` audience; never forward parent/device bearer. |
| Studio REST migration alias | `X-Service-Token` carries a JWT when explicitly enabled | `curriculum-studio/internal/api/auth.go`, `curriculum-studio/internal/config/config.go` | Standard M2M bearer only; remove alias after parity. |
| Studio gRPC | Handwritten unary JWT metadata interceptor; authorization is incomplete on several methods | `curriculum-studio/internal/grpcapi/server.go` | Authstack unary and stream authentication followed by a complete full-method authorization registry. |
| Other HTTP/MCP/gRPC callers | Exact caller/target matrix not yet centrally recorded | Composition roots, generated clients, deploy jobs, and config packages | One client credential per caller→audience relationship, least privilege, replacement rather than forwarding of inbound authorization. |

### Device credentials that remain product-local

| Device system | Local behavior to preserve | Evidence |
|---|---|---|
| LMS student workstation | Parent issues one-time code; workstation exchanges it for an opaque token tied to one student/device; hashes live in LMS; revoke/rotate forces re-pair; privileged broker keeps the token from the TUI. | `server/internal/api/student_api.go`, `server/internal/repo/student_device.go`, `server/internal/studentclient/broker/`, `server/internal/studentclient/cache/`, `workstation/` |
| TV/Android device | TV admin issues a one-time code; TV API exchanges it for an opaque TV-local token; code rotation/revocation invalidates old token; catalog/playback/release routes require that device. | `server/internal/tv/api/device.go`, `server/internal/tv/api/auth.go`, `server/internal/tv/repo/`, `android/` |
| Primer Tasks browser/Android student | Parent issues a short-lived QR/code; Tasks stores product-local browser/device credentials and student binding; archive/revoke invalidates sessions and pairing. | Incoming `primer-tasks/internal/api/`, `primer-tasks/internal/db/migrations/00002_auth_pairing_hardening.sql`, `primer-tasks/android/` |

Existing tests already cover parts of device/admin separation, but the final migration needs one cross-product compatibility suite proving both directions: device tokens cannot authenticate human/admin/service routes, and authstack human/M2M identities cannot authenticate device routes or bypass pairing.

## Scope boundaries

### In scope

- Explicit Clerk-vs-ZITADEL decision and provider-side setup checklist.
- Canonical identity as `(issuer, subject)`, additive mappings from legacy Primer Identity/Stytch references, explicit account linking, and product-local tenant memberships.
- Authstack `auth.Principal`, `auth.Authenticator`, and `auth.Authorizer` at HTTP, gRPC, MCP, BFF, and application boundaries.
- Explicit non-empty `auth.AuthenticationPolicy` per trust boundary; authentication before authorization.
- Human/admin browser sessions for LMS, TV admin, Tasks parent, and Studio.
- Audience-specific service identities and outbound token replacement.
- Dual-run, telemetry, log/audit redaction, migration/replay behavior, cutover, rollback, and eventual cleanup.
- Retirement of Primer Identity/Stytch and superseded JWT/shared-secret/BFF code only after proof.

### Out of scope

- Moving LMS workstation, TV device, or Primer Tasks student/Android pairing into authstack.
- Accepting device credentials on human/admin/service endpoints or accepting human/M2M identities on device endpoints.
- Treating provider organizations, roles, email, username, or display name as product membership or canonical identity.
- Forwarding inbound browser/device bearer tokens to downstream services.
- Replacing product-local educator roles, Studio workspace memberships, Tasks tenant ownership, TV admin membership, resource ownership, or business authorization with authentication claims alone.
- Reading both authstack provider guides or implementing both providers “just in case.” Exactly the selected provider guide is read and used.

## Global constraints

1. **Predecessor and Tasks gate.** `paseo wait b6ce6b31-ea58-4f00-935d-d71704fc7e16` succeeded and this branch is based on its PR #70 merge `cac90a151459a2fc60d506839424155f7f87ef80`; however, that commit integrates plans only. Production changes remain blocked until WIP phases 3–13 put `primer-tasks/` and the repaired baseline on protected `master`, publish the exact successor tip/handoff, and this branch is updated again.
2. **Provider gate.** Clerk or ZITADEL must be selected explicitly. The decision records canonical IDs, account linking, sessions, tenant ownership, provider-side resources, migration, and rollback. No provider-specific implementation or destructive Identity change precedes it.
3. **Authstack authority.** Use the fetched current `authstack` `master`, module `git.clark.team/aleksclark/authstack`, and its agent skill, Go integration guide, verification guide, and exactly one selected provider guide.
4. **Composition roots only.** Provider constructors, provider config, provider SDK types, token sources, and compatibility adapters stay in `cmd/*`, `internal/app`, or equivalent startup wiring. Handlers/domain code depend only on authstack contracts.
5. **Fail closed.** Every protected boundary has a named, non-empty credential policy, exact audience, authorized party where applicable, and required tenant where applicable. Missing route/full-method authorization policy is an error, not allow.
6. **Order.** Authenticate first, then authorize, then access tenant/resource data. Request body/path/query identity is never trusted as subject or tenant authority.
7. **Canonical identity.** Persist issuer and subject separately (or a documented reversible canonical encoding). Email is lookup/display only and never auto-links accounts.
8. **Tenant ownership.** Products continue to own memberships. Tenant-scoped reads and writes constrain verified issuer, subject, tenant, and resource membership as appropriate.
9. **Service isolation.** Each caller obtains a destination-audience token. Outbound authstack transports replace any inbound authorization header/metadata. Credentials are not shared across LMS, TV, Studio, Tasks, or Agents audiences.
10. **Device isolation.** Device routes keep their current local stores and guards. Authstack middleware must not wrap or reinterpret device pairing/token routes.
11. **Secrets and logs.** Never print, inspect, commit, or persist bearer/refresh tokens, private keys, client secrets, provider response bodies, or local environment values. Audit only sanitized canonical actor references, policy names, outcome, and request IDs.
12. **Rollback.** Additive schema and compatibility paths remain until cutover proof, session drain, and an explicit rollback-window close. No rollback depends on recreating deleted Identity state or old credentials.
13. **Browser proof is supplemental.** Managed headless browser login/session/logout/tenant tests are required, but backend policy, tenancy, service, and cross-credential tests remain authoritative.
14. **Protected master.** Land independently green commits/PRs, never force-push, and keep generated specs/clients/config/deploy docs synchronized.

## Initial policy matrix to ratify in Phase 1

Names and permissions may be adjusted after exact route inventory, but credential classes and audience separation may not be weakened.

| Boundary | Accepted credential kind(s) | Audience / party / tenant | Product authorization |
|---|---|---|---|
| LMS parent/admin browser API | Selected-provider browser session or BFF-held user OAuth only | `primer-lms`; exact browser/BFF party; tenant required if provider tenancy is selected | LMS educator link and local `parent`/`admin`; household/resource checks |
| LMS instruction-log ingest | M2M only | `primer-lms`; caller-specific client | `instruction-logs:ingest` |
| TV admin browser API | Browser session or BFF-held user OAuth only | `primer-tv`; exact TV web party; tenant required | Product-local TV admin membership |
| TV machine admin API | M2M only | `primer-tv`; ingest/LMS caller identities | Narrow route permissions such as catalog/schedule/write; never device pairing use unless explicitly authorized product action |
| Studio REST authoring | Browser session/BFF-held OAuth for humans; M2M only on explicitly machine routes | `curriculum-studio`; exact parties; tenant required for human workspace actions | Active Studio workspace membership and local role |
| Studio MCP | OAuth for human MCP client and M2M for registered service clients, separated by route/tool policy | `curriculum-studio`; exact MCP client parties; tenant for human workspace tools | Local membership, scopes, human-only publish confirmation |
| Studio gRPC | M2M only | `curriculum-studio`; caller-specific audience token | Full-method permission registry and resource/workspace policy |
| Primer Agents control plane | M2M and, only where still required, human OAuth under explicit route policies | `primer-agents`; exact client parties | Route permissions, run/session ownership, profile admission |
| Tasks parent browser API | Selected-provider browser session or BFF-held OAuth only | `primer-tasks`; exact Tasks web party; tenant required | Tasks-local tenant membership |
| LMS/TV/Tasks device APIs | Product-local opaque device credential only; **not authstack** | Local device/store binding | Bound device/student/TV record and revocation state |

## Phase overview

| Phase | Goal | Depends on |
|---|---|---|
| [Phase 1: Integrated inventory and provider decision](./phase-01-inventory-provider-decision.md) | Rebase to wave 00, freeze the exact boundary/policy/data inventory, and select Clerk or ZITADEL with a reversible identity plan. | Wave 00 exact integrated tip |
| [Phase 2: Provider-neutral authstack foundation](./phase-02-authstack-foundation.md) | Add authstack contracts at composition roots, explicit policy registries, canonical mapping schema, and compatibility harnesses without cutover. | Phase 1 |
| [Phase 3: Service boundary migration](./phase-03-service-boundaries.md) | Replace shared secrets, pre-minted JWTs, and handwritten HTTP/gRPC/MCP service adapters with audience-specific authstack M2M identity. | Phase 2 |
| [Phase 4: Human dual-run and session migration](./phase-04-human-dual-run.md) | Run selected-provider human authentication beside existing Identity sessions, map identities safely, and prove browser/BFF rollback. | Phases 2–3 |
| [Phase 5: Product-by-product cutover](./phase-05-product-cutovers.md) | Cut over LMS, TV admin, Tasks parent, Studio/BFF/MCP, and downstream Agents one independently reversible boundary at a time. | Phase 4 |
| [Phase 6: Device boundary certification](./phase-06-device-isolation.md) | Prove all three pairing/token systems are unchanged and mutually isolated from human/service auth. | Phases 2–5 |
| [Phase 7: Legacy retirement and operational reconciliation](./phase-07-retirement-operations.md) | Close rollback gates, remove Primer Identity/Stytch and superseded auth code/secrets, and reconcile generated artifacts, deploy, runbooks, and wave-02 handoff. | Phases 3–6 and rollback-window approval |

## Completion rule

The migration is complete only when:

- the selected provider and exact issuer/subjects/tenant model are recorded and provisioned;
- all protected HTTP, gRPC, MCP, and browser boundaries use explicit authstack authentication followed by product-local authorization;
- every service direction uses an audience-specific credential and outbound transports replace inbound authorization;
- additive canonical mappings are complete, collision reports are empty or explicitly resolved, and no email-based auto-link exists;
- real browser login, session continuity, logout/revocation, tenant switch/isolation, and rollback tests pass for LMS, TV admin, Tasks parent, and Studio;
- backend missing/invalid/wrong-issuer/wrong-audience/wrong-party/wrong-kind/missing-tenant/forbidden/outage tests pass;
- device pairing, one-use/replay, binding, revoke, rotate, and re-pair tests pass unchanged, including cross-credential rejection in both directions;
- token/secret/provider-payload redaction audits and key-rotation/provider-outage tests pass;
- legacy Identity/Stytch/shared-secret/JWT paths are removed only after a signed rollback-window close and a backup/restore or retained-state proof;
- root/module tests, race/coverage gates, generated OpenAPI/clients, browser E2E, build/container/deploy checks, and anti-cheating audits are green on the exact integrated tip;
- protected-master PRs are green and the final commit is recorded as the exact handoff tip for wave 02 Ultracore.
