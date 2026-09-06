# Primer authstack migration

## Outcome

Primer authenticates human/admin browser users and service-to-service HTTP, gRPC, and MCP callers through `git.clark.team/aleksclark/authstack`, with provider construction confined to process composition roots and authorization retained in each product. Primer Identity, Stytch broker/JWKS code, handwritten JWT validators, and shared service secrets are retired only after dual-run parity and rollback gates pass.

Student and TV device pairing is deliberately unchanged: pairing codes, opaque device tokens, device-to-student/TV bindings, revocation, re-pairing, and local systems of record never move into authstack.

**Provider decision: Clerk.** The user selected Clerk on 2026-09-05. Authstack's Clerk guide has been read completely; the ZITADEL guide remains intentionally unread. Phase 1 freezes the Clerk issuer/subject, Organizations, browser-session/BFF, M2M, linking, dual-run, rollback, and configuration contracts below. Provider-specific production work is still blocked on the Tasks-integrated WIP Phase 13 handoff; destructive Identity cutover remains blocked until dual-run and rollback gates pass.

## Current-state summary

Evidence below was refreshed after `paseo wait b6ce6b31-ea58-4f00-935d-d71704fc7e16` completed. Wave 00 head `51656816f7954ac1f61ca751b8ffc47eb4caa983` merged through PR #70 as protected-master commit `cac90a151459a2fc60d506839424155f7f87ef80`, and this initiative is based exactly there. That merge contains the WIP integration plan and Primer Tasks plans, but **not the `primer-tasks/` implementation**: WIP phases 3–13 remain unexecuted and `primer-tasks/` is absent from the tree. Consequently this is still a pre-Tasks production baseline. The inventory uses current code plus read-only donor evidence, but no production authstack implementation may begin until the Tasks implementation is integrated and an exact successor master tip is published.

### Existing human/admin authentication

| Product boundary | Existing behavior | Repository evidence | Missing/end-state change |
|---|---|---|---|
| LMS parent/admin API and SPA | `ParentSessionGuard` accepts a product-local opaque parent session or a Primer Identity ES256 JWT, then loads an LMS educator and enforces local `parent`/`admin`. `/auth/login` validates LMS password hashes. The SPA stores the bearer in `localStorage`. | `server/internal/api/auth_parent.go`, `server/internal/repo/parent_auth.go`, `web/src/api/auth.ts`, `web/src/components/parent-token-gate.tsx`, `server/internal/api/dual_auth_test.go` | Clerk session→product BFF transition; canonical Clerk `(issuer, subject)` link; active Organization/local educator authorization; no browser bearer storage. |
| TV admin API and SPA | Admin routes accept either any valid Primer Identity JWT for the TV audience or a shared `X-Admin-Key`/opaque bearer. There is no local role lookup for a human JWT. | `server/internal/tv/api/auth.go`, `server/internal/tv/api/identity_auth_test.go`, `tv-web/src/api/auth.ts` | Separate Clerk human-session and M2M policies, product-local admin membership, product BFF, removal of shared-key/browser key path. |
| Curriculum Studio REST/BFF | Handwritten JWKS validator produces `authn.AuthContext`; REST loads product-local workspace memberships. Existing BFF performs OAuth code+PKCE and stores access/refresh tokens server-side, but defaults to an in-memory store. | `curriculum-studio/internal/authn/`, `curriculum-studio/internal/api/auth.go`, `curriculum-studio/internal/bff/`, `curriculum-studio/internal/app/app.go` | Authstack principal/authenticator at composition root, durable provider-appropriate BFF sessions, canonical membership migration, removal of handwritten validator. |
| Studio MCP | Streamable HTTP extracts and validates a bearer independently, loads Studio memberships, and filters tools by local membership, principal kind, and scopes. | `curriculum-studio/internal/mcp/handler.go`, `curriculum-studio/internal/mcp/authz.go`, `curriculum-studio/internal/mcp/principal.go` | Explicit Clerk session/M2M authstack policies and `auth.Authorizer`; retain local workspace/tool authorization and fail-closed tool policy. |
| Primer Agents | Global middleware uses a handwritten Primer Identity JWKS validator and custom principal; route middleware and profile admission enforce scopes. Human and service tokens share the route group. | `primer-agents/internal/authn/`, `primer-agents/internal/api/api.go`, `primer-agents/internal/api/handlers.go`, `primer-agents/internal/profile/admission.go` | Authstack principal, explicit per-boundary credential kinds/audience, authstack authorizer while preserving owner/profile policy. |
| Primer Tasks parent/BFF | Incoming wave-00 Tasks has a parent OIDC/BFF flow and `tasks-test-issuer`; exact files and persisted identity fields must be re-inventoried after integration. | Incoming `primer-tasks/cmd/tasks-test-issuer/`, `primer-tasks/internal/api/`, `primer-tasks/internal/config/`, `primer-tasks/web/` | Provider-backed parent BFF and canonical mapping without changing browser/Android student pairing. Test issuer remains test-only. |
| Primer Identity/Stytch | Separate service owns Stytch broker callbacks, exact Stytch tuple mappings, Primer OAuth grants, JWKS/signing keys, service principals, webhooks, and generated client. | `primer-identity/`, `agent_docs/plans/stytch-identity-ib0/`, `agent_docs/plans/primer-identity-service/` | Dual-run source and rollback authority first; eventually retire after all browser and service consumers have migrated and rollback window closes. |

### Existing service authentication

| Direction/boundary | Existing credential | Repository evidence | End state |
|---|---|---|---|
| TV reporter → LMS instruction ingest | Shared `X-Service-Token` | `server/internal/api/instruction_logs.go`, `server/internal/tv/primer/primer.go` | M2M token for LMS-only audience and `instruction-logs:ingest` permission. |
| Content ingest (and other operators) → TV admin | Shared `X-Admin-Key` | `server/internal/ingest/tvclient/tvclient.go`, `server/internal/tv/api/auth.go` | Distinct M2M identity/audience/permission; not accepted as a human session or device token. |
| LMS → Primer Agents | Environment-selected pre-minted Identity access token | `server/internal/remoteagent/adapter.go` | Target-specific Clerk JWT M2M `auth.TokenSource` and authstack outbound transport for `primer-agents`; never use OAuth client credentials or forward parent/device bearer. |
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

- Clerk application, Organizations, session-token template, M2M machine, JWKS, origin/party, and provider-side setup checklist.
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
- Reading the ZITADEL guide or implementing a second provider abstraction “just in case.” Only the Clerk guide is selected and used.

## Global constraints

1. **Predecessor and Tasks gate.** `paseo wait b6ce6b31-ea58-4f00-935d-d71704fc7e16` succeeded and this branch is based on its PR #70 merge `cac90a151459a2fc60d506839424155f7f87ef80`; however, that commit integrates plans only. Production changes remain blocked until WIP phases 3–13 put `primer-tasks/` and the repaired baseline on protected `master`, publish the exact successor tip/handoff, and this branch is updated again.
2. **Clerk gate.** Clerk is selected. Production wiring must use `authstack/clerk` exactly as frozen below; no ZITADEL or generic second-provider path. No destructive Identity change precedes live Clerk dual-run and rollback proof.
3. **Authstack authority.** Use fetched authstack `master` `af1841573db40fda84a29d33abe8f7507a11e068` (or a later explicitly reviewed pin), module `git.clark.team/aleksclark/authstack`, and the completely read skill, Go integration, Clerk, and verification guides.
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

## Frozen Clerk decisions

1. **Application and issuer.** Provision one Primer Clerk application with Organizations enabled. `CLERK_ISSUER` is the exact Clerk instance issuer and is never inferred from another URL. All resource servers use a configured Clerk JWKS URL or public-key path; JWKS URLs are not formed by string concatenation. Resource servers need no Clerk secret key for local JWT verification.
2. **Canonical human identity.** A human is keyed by the exact pair `(Principal.Issuer, Principal.Subject)`, where issuer equals `CLERK_ISSUER` and subject is Clerk's verified session JWT `sub` (the Clerk user ID). `sid`, email, username, display name, `azp`, organization, role, and permissions are attributes—not identity keys.
3. **Canonical service identity.** A Clerk JWT containing `machine_id` is M2M; authstack classifies it as service/M2M and canonical subject is the verified `machine_id`, not fallback `sub`. Human and machine hints cannot reclassify each other. Persisted actor identity remains issuer+subject; principal kind is validated context.
4. **Organizations and tenant context.** Every human Primer product session requires an active Clerk Organization. Verified `org_id` (or compact `o.id`) becomes `Principal.TenantID`. One Clerk Organization represents the outer Primer household/organization tenancy context, but it never auto-creates an LMS educator, TV administrator, Tasks membership, Studio workspace, role, or resource permission. Each product maps `(CLERK_ISSUER, org_id)` to its local household/tenant and checks local membership. Studio may host multiple workspaces under that local tenant.
5. **Provider roles and permissions.** Clerk `org_role`/`o.role` and `org_permissions` are not Primer application permissions and are not passed directly to `rbac.Authorizer`. They are retained only as verified diagnostics/eligibility inputs if a product explicitly needs them; LMS roles, TV admin membership, Tasks membership, Studio workspace roles, MCP tool policy, and Agents ownership/profile admission remain local.
6. **Human session policy.** Human APIs accept only `auth.CredentialSession`. Policies require the exact per-product browser origin in `AuthorizedParties`, `RequireTenant: true`, and a per-product audience. Primer will configure Clerk session-token templates for `primer-lms`, `primer-tv`, `primer-tasks`, and `curriculum-studio`; if Clerk qualification proves a selected flow cannot carry `aud`, omitting `Audiences` requires a reviewed plan amendment and retains exact issuer, party, lifetime, and tenant checks—never a silent fallback.
7. **Browser/BFF model.** Clerk is the browser authentication/session authority and product BFFs remain the application boundary. Frontends expose only Clerk's publishable key. A Clerk session JWT may exist transiently in browser memory for a one-time TLS session-establishment request if required by Clerk's frontend SDK; it is never stored in localStorage, sessionStorage, IndexedDB, a URL, or logs. The BFF validates it with authstack, stores required Clerk session material only in encrypted/durable server-side session state, and issues an opaque `HttpOnly`, `Secure`, host-scoped, appropriately `SameSite` product cookie. Subsequent product calls use that cookie; provider material is not returned to JavaScript. Authstack does not supply the BFF, so this composition is product-owned and requires a qualification spike before cutover.
8. **Organization switch.** Switching Clerk Organizations produces a new session token. The BFF re-authenticates it, replaces tenant-bound server session state, rotates the product cookie/session, clears prior product caches, and reloads local memberships. Browser-supplied organization IDs are never authorization input.
9. **M2M model.** Primer services use Clerk-issued JWT M2M tokens containing `machine_id`, one Clerk Machine identity per caller and one target audience per credential/template. Service policies accept only `auth.CredentialM2M`; `RequireTenant` is false unless a specific service route is deliberately tenant-bound, in which case tenant is verified and local policy still applies. Clerk OAuth client credentials are unsupported by the authstack Clerk adapter, so this plan does **not** use `oauth.NewClientCredentialsTokenSource`.
10. **M2M outbound source.** Each caller composition root supplies an application-owned `auth.TokenSource` backed by the normal secret/workload delivery mechanism for the exact Clerk machine→audience token and overlap rotation. `authhttp.TokenTransport`/`authgrpc` clients replace inbound authorization. No generic environment-selected token name, shared machine, cross-audience cache, opaque API key, or inbound bearer forwarding is allowed.
11. **Opaque Clerk credentials.** Opaque Clerk API keys/M2M credentials and `clerk.RemoteVerifier` are out of scope. All accepted Clerk human and service credentials are locally verified JWTs. A future opaque need requires a dedicated endpoint, explicit kind adapter, authoritative remote verifier, and separate reviewed plan.
12. **Key rotation.** Composition roots load the exact configured Clerk JWKS and provide bounded refresh on unknown `kid`/scheduled rotation before rebuilding/swapping the immutable Clerk authenticator. Startup fails if JWKS is empty/unreachable in production. Rotation tests must prove old/new overlap and post-retirement rejection without a Clerk secret key in resource servers.
13. **Account linking.** Existing Primer Identity `accounts.id` and product-local actor IDs remain stable during migration. Additive links store legacy issuer+subject and Clerk issuer+subject, with explicit administrator/user proof and audit. Email similarity never links or merges. Collisions and one-to-many candidates are quarantined and deny.
14. **Dual-run.** Per product/boundary states are `legacy`, `shadow`, `dual`, `clerk`, and `rollback`. Shadow Clerk success cannot grant. Dual grants only after both authentication results map to the same approved local actor and local membership; disagreement denies/quarantines. No raw Stytch credential is sent to Clerk or Clerk token to Primer Identity.
15. **Rollback.** Keep Primer Identity/Stytch registrations, keys, grants, product legacy sessions, actor IDs, and additive links intact through the observation window. Rollback is per product and does not unwind Clerk links or M2M migrations. Retirement requires drained legacy sessions/grants, zero unresolved mismatches, browser/backend/device proof, and backup/restore evidence.
16. **Configuration names.** Use the exact prefix table below. Each backend sets either its configured `*_CLERK_JWKS_URL` or `*_CLERK_JWT_PUBLIC_KEY_PATH`, never both; the loader turns that into `clerk.Config.JWKS`/`JWTKeyPEM`. Frontends expose only the publishable key and session-template name. M2M values are delivered by target-specific secret files/workload mounts rather than raw environment values. No Clerk secret key is loaded by resource servers, and no secret value enters source, logs, plans, or generated clients.

| Process/workspace | Frozen non-secret configuration | Secret/session configuration references |
|---|---|---|
| LMS server / `web` | `CLERK_ISSUER`, `CLERK_JWKS_URL` or `CLERK_JWT_PUBLIC_KEY_PATH`, `CLERK_AUTHORIZED_PARTIES`, `CLERK_AUDIENCE=primer-lms`, `CLERK_SESSION_TEMPLATE`, `AUTH_ROLLOUT_MODE`; frontend `VITE_CLERK_PUBLISHABLE_KEY`, `VITE_CLERK_SESSION_TEMPLATE` | `BFF_SESSION_STORE_URL`, `BFF_COOKIE_KEY_FILE`; target-specific `CLERK_PRIMER_AGENTS_M2M_TOKEN_FILE` if LMS calls Agents |
| TV server / `tv-web` | `TV_CLERK_ISSUER`, `TV_CLERK_JWKS_URL` or `TV_CLERK_JWT_PUBLIC_KEY_PATH`, `TV_CLERK_AUTHORIZED_PARTIES`, `TV_CLERK_AUDIENCE=primer-tv`, `TV_CLERK_SESSION_TEMPLATE`, `TV_AUTH_ROLLOUT_MODE`; frontend `VITE_CLERK_PUBLISHABLE_KEY`, `VITE_CLERK_SESSION_TEMPLATE` | `TV_BFF_SESSION_STORE_URL`, `TV_BFF_COOKIE_KEY_FILE`, `TV_CLERK_LMS_M2M_TOKEN_FILE` |
| Curriculum Studio / its web | `STUDIO_CLERK_ISSUER`, `STUDIO_CLERK_JWKS_URL` or `STUDIO_CLERK_JWT_PUBLIC_KEY_PATH`, `STUDIO_CLERK_AUTHORIZED_PARTIES`, `STUDIO_CLERK_AUDIENCE=curriculum-studio`, `STUDIO_CLERK_SESSION_TEMPLATE`, `STUDIO_AUTH_ROLLOUT_MODE`; frontend `VITE_CLERK_PUBLISHABLE_KEY`, `VITE_CLERK_SESSION_TEMPLATE` | `STUDIO_BFF_SESSION_STORE_URL`, `STUDIO_BFF_COOKIE_KEY_FILE`, target-specific `STUDIO_CLERK_*_M2M_TOKEN_FILE` |
| Primer Agents | `PRIMER_AGENTS_CLERK_ISSUER`, `PRIMER_AGENTS_CLERK_JWKS_URL` or `PRIMER_AGENTS_CLERK_JWT_PUBLIC_KEY_PATH`, `PRIMER_AGENTS_CLERK_AUDIENCE=primer-agents`, `PRIMER_AGENTS_AUTH_ROLLOUT_MODE` | No provider secret key; callers hold their own target-specific token files |
| Primer Tasks / its web | `TASKS_CLERK_ISSUER`, `TASKS_CLERK_JWKS_URL` or `TASKS_CLERK_JWT_PUBLIC_KEY_PATH`, `TASKS_CLERK_AUTHORIZED_PARTIES`, `TASKS_CLERK_AUDIENCE=primer-tasks`, `TASKS_CLERK_SESSION_TEMPLATE`, `TASKS_AUTH_ROLLOUT_MODE`; frontend `VITE_CLERK_PUBLISHABLE_KEY`, `VITE_CLERK_SESSION_TEMPLATE` | `TASKS_BFF_SESSION_STORE_URL`, `TASKS_BFF_COOKIE_KEY_FILE`, target-specific `TASKS_CLERK_*_M2M_TOKEN_FILE` |
| Content ingest | Target audience metadata `INGEST_CLERK_TV_AUDIENCE=primer-tv` | `INGEST_CLERK_TV_M2M_TOKEN_FILE` |

`*_TOKEN_FILE`, `*_COOKIE_KEY_FILE`, and store URL values are provided through the repository's approved secret/workload mechanism and are never read or printed during agent work. Final names must be checked against integrated config conventions before code lands; changing a name requires updating this table, config validation, deploy manifests, and redaction tests together.

## Clerk policy matrix to ratify against integrated routes

Names and permissions may be adjusted after exact Tasks route inventory, but Clerk credential classes, organization requirements, exact parties, and audience separation may not be weakened.

| Boundary | Accepted credential kind(s) | Audience / party / tenant | Product authorization |
|---|---|---|---|
| LMS parent/admin browser API | Clerk session only | `primer-lms`; exact LMS origin in `azp`; active Clerk Organization required | Clerk link→LMS educator and local `parent`/`admin`; household/resource checks |
| LMS instruction-log ingest | M2M only | `primer-lms`; caller-specific client | `instruction-logs:ingest` |
| TV admin browser API | Clerk session only | `primer-tv`; exact TV web origin in `azp`; active Clerk Organization required | Clerk link→product-local TV admin membership |
| TV machine admin API | M2M only | `primer-tv`; ingest/LMS caller identities | Narrow route permissions such as catalog/schedule/write; never device pairing use unless explicitly authorized product action |
| Studio REST authoring | Clerk session for humans; Clerk M2M only on explicitly machine routes | `curriculum-studio`; exact Studio origin; active Organization required for humans | Active Studio workspace membership and local role |
| Studio MCP | Clerk session for interactive human MCP clients and Clerk M2M for services, separated by route/tool policy | `curriculum-studio`; exact registered client parties; active Organization for humans | Local membership, scopes, human-only publish confirmation |
| Studio gRPC | M2M only | `curriculum-studio`; caller-specific audience token | Full-method permission registry and resource/workspace policy |
| Primer Agents control plane | Clerk M2M and, only where still required, Clerk session under separate explicit route policies | `primer-agents`; exact registered parties; active Organization for humans | Route permissions, run/session ownership, profile admission |
| Tasks parent browser API | Clerk session only | `primer-tasks`; exact Tasks web origin in `azp`; active Clerk Organization required | Clerk link→Tasks-local tenant membership |
| LMS/TV/Tasks device APIs | Product-local opaque device credential only; **not authstack** | Local device/store binding | Bound device/student/TV record and revocation state |

## Phase overview

| Phase | Goal | Depends on |
|---|---|---|
| [Phase 1: Integrated inventory and Clerk decision freeze](./phase-01-inventory-provider-decision.md) | Rebase to the Tasks-integrated handoff, freeze the exact boundary/policy/data inventory, and validate the frozen Clerk migration contract. | WIP Phase 13 exact integrated tip |
| [Phase 2: Clerk authstack foundation](./phase-02-authstack-foundation.md) | Add Clerk/authstack composition roots, explicit policy registries, canonical mapping schema, and compatibility harnesses without cutover. | Phase 1 |
| [Phase 3: Service boundary migration](./phase-03-service-boundaries.md) | Replace shared secrets, pre-minted JWTs, and handwritten HTTP/gRPC/MCP service adapters with audience-specific authstack M2M identity. | Phase 2 |
| [Phase 4: Clerk human dual-run and session migration](./phase-04-human-dual-run.md) | Run Clerk human authentication beside existing Identity sessions, map identities safely, and prove browser/BFF rollback. | Phases 2–3 |
| [Phase 5: Product-by-product cutover](./phase-05-product-cutovers.md) | Cut over LMS, TV admin, Tasks parent, Studio/BFF/MCP, and downstream Agents one independently reversible boundary at a time. | Phase 4 |
| [Phase 6: Device boundary certification](./phase-06-device-isolation.md) | Prove all three pairing/token systems are unchanged and mutually isolated from human/service auth. | Phases 2–5 |
| [Phase 7: Legacy retirement and operational reconciliation](./phase-07-retirement-operations.md) | Close rollback gates, remove Primer Identity/Stytch and superseded auth code/secrets, and reconcile generated artifacts, deploy, runbooks, and wave-02 handoff. | Phases 3–6 and rollback-window approval |

## Completion rule

The migration is complete only when:

- the frozen Clerk application, exact issuer, user/machine subject semantics, Organizations tenancy, session templates, origins/parties, audiences, JWKS rotation, and M2M machines are provisioned and verified;
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
