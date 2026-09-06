# Phase 7: Legacy retirement and operational reconciliation

## Goal

After all service/human cutovers, device certification, monitoring windows, and rollback rehearsals pass, close the rollback window deliberately and remove Primer Identity/Stytch plus superseded shared-secret, handwritten JWT, legacy BFF/session, and browser bearer infrastructure. Reconcile schemas, generated contracts, CI, deploy/secret-manager configuration, runbooks, observability, backups, and architecture, then publish an exact wave-02 Ultracore handoff tip.

This is the only destructive phase. It must leave no stale second authority while preserving product-local authorization, canonical mapping/audit history needed for provenance, and all device systems.

## BDD Success Criteria

#### Scenario: Rollback window closes with objective evidence

- **Given** every cutover and device certification is green for the agreed observation period
- **When** operators approve legacy retirement
- **Then** the approval records exact release tips, live/provider/browser/backend evidence, unresolved mismatch count, session/grant drain state, backup/restore evidence, and rollback decision
- **And** destructive cleanup cannot run without that approval
- **And** retained canonical/local identity data is sufficient for audit and recovery.

#### Scenario: Primer Identity and Stytch are no longer runtime dependencies

- **Given** protected master after retirement
- **When** all Primer services build, start, and authenticate users/services
- **Then** no runtime route, config, deploy job, client, module import, JWKS URL, broker callback, webhook, signing-key job, or secret references Primer Identity/Stytch
- **And** Clerk authstack flows remain green
- **And** old Identity endpoints are unreachable/retired according to operations policy.

#### Scenario: Superseded service and JWT paths fail

- **Given** old `X-Service-Token`, `X-Admin-Key`, Primer Identity JWT, legacy password/opaque parent session, and Studio alias credentials
- **When** they are presented after retirement
- **Then** each protected human/service route rejects them
- **And** no empty-config/inert guard reopens a route
- **And** standardized authstack errors reveal no migration detail.

#### Scenario: Device systems survive cleanup

- **Given** paired LMS, TV, and Tasks devices created before retirement
- **When** Identity/Stytch/JWT/shared-secret code and config are removed
- **Then** each device continues to authenticate to its product-local routes
- **And** revoke/rotate/re-pair still works
- **And** cleanup has not deleted shared utility code, tables, secrets, or client configuration required only by devices.

#### Scenario: Fresh deployment and recovery are complete

- **Given** an empty supported environment and a backup/restore test environment
- **When** operators deploy/migrate from protected master and restore required product state
- **Then** Clerk application/Organizations/JWKS/session-template/M2M configuration, canonical mappings, local memberships, and device records support real smoke tests
- **And** removed Identity migrations/services are not required for startup
- **And** rollback now means forward-fix/restore under the documented post-retirement policy, not re-enabling deleted code.

#### Scenario: Wave 02 receives an exact stable handoff

- **Given** all CI and operational gates are green on protected master
- **When** this initiative closes
- **Then** the exact integrated commit, authstack version/provider, active audiences/policies, canonical-ID format, remaining local authorization/device boundaries, config names, and known residuals are published for Ultracore
- **And** no secret value or provider payload appears in the handoff.

## Implementation Instructions

1. Create a retirement checklist requiring explicit approval and evidence: per-product authstack authority, M2M authority, zero unresolved link/parity issues, legacy session/grant drain, provider webhook/revocation status, device certification, backup/restore, and rollback-window duration.
2. Disable new legacy session/grant issuance first; monitor drain and reject reactivation. Take approved backups/exports of Primer Identity mappings/audit metadata needed for provenance without exporting raw provider/session credentials into the repository.
3. Remove Primer Identity service deployment, module/workspace entries, runtime clients, broker callbacks, Stytch adapter/webhooks/cache, OAuth/JWKS/signing/service-principal jobs, config/secrets, dashboards, and CI targets. Archive historical migrations/docs according to repository policy rather than rewriting applied database history.
4. Remove superseded product code: `server/internal/identityauth`, legacy parent password/session authority if not retained as separately approved break-glass, TV shared admin key, Studio/Agents handwritten JWKS validators/custom principals, Studio service-token alias, pre-minted env token source, and obsolete browser localStorage/paste-token code.
5. Remove shared-secret config/deploy values only after all callers have authstack M2M. Ensure absence fails closed rather than triggering existing inert guards.
6. Preserve additive canonical link/local actor records required for stable foreign keys, audit, and rollback provenance. If dropping legacy columns/tables, use forward-only migrations with preconditions and counts; do not cascade-delete product data or device bindings.
7. Preserve `authutil`/hash/token utilities and schema fields still used by LMS, TV, or Tasks device credentials; split packages before deleting legacy parent-session pieces when responsibilities overlap.
8. Regenerate LMS/TV/Studio/Tasks/Agents OpenAPI and generated clients; remove obsolete security schemes/config docs. Update web bundles, Go workspace/sums, container files, compose/Stacklane/Nomad manifests, CI, Make targets, example config, architecture diagrams, and runbooks.
9. Add negative legacy-credential tests as permanent regression coverage. Keep Clerk JWKS rotation/outage, M2M machine/audience isolation, Organization/tenant isolation, logout/revocation, and device matrix in CI at appropriate deterministic/live tiers.
10. Run a clean fresh-deploy/migrate smoke test and a restore drill. Shut down/archive Identity infrastructure only after successful replacement smoke; revoke/delete provider-side Stytch applications/secrets through approved operations without printing values.
11. Open protected-master PRs in independently reversible cleanup slices. Do not force-push. Final integration occurs only after CI and independent Terra security/browser review are green.
12. Publish the wave-02 handoff artifact with exact master commit and remaining boundaries.

## End-to-End Test Plan

- Before deletion, run the full Phase 3–6 suite and capture redacted evidence plus counts needed to approve retirement.
- On a clean checkout/fresh databases, generate clients, build containers/binaries, apply all active migrations, provision/configure the Clerk development application, Organizations, session templates, origins, JWKS, and Machines through approved tooling, and start LMS, TV, Studio, Tasks, Agents, and required workers without Primer Identity.
- Use managed headless browser to log in/out and exercise tenant/membership actions on LMS, TV admin, Tasks parent, and Studio. Use real development provider; verify no network request targets Primer Identity/Stytch hosts.
- Execute real M2M TV→LMS, ingest→TV, LMS/Studio→Agents, Studio gRPC/MCP, and any integrated Tasks service calls; assert audience/permission isolation and durable retry behavior.
- Present representative old Primer Identity JWTs, old shared headers, old parent opaque sessions, old Studio alias, and wrong-provider/test-issuer credentials; assert denial.
- Keep devices paired across the cleanup deployment/restart and repeat LMS workstation, TV Android, and Tasks student browser/Android operations plus revoke/rotate/re-pair.
- Run Clerk signing-key rotation/unknown-`kid` JWKS refresh, provider outage/recovery, logout/revocation, M2M expiry/overlap rotation, Organization switch/cross-tenant mutation denial, and log redaction.
- Restore product databases from disposable backups and repeat canonical membership and device smoke tests. Primer Identity restore is tested only if retained by the approved archive policy, not required by active runtime.
- Run all root/module tests, race and coverage gates, browser/Android suites, lint/typecheck, generated drift, build/container/compose/deploy validation, secret scanners, and architecture/link checks.

No mock-only, loopback-only, or in-memory-only evidence can approve production retirement. Deterministic fixtures supplement a real development-provider and real-process/real-Postgres deployment tour.

## Anti-Cheating Audit

- Search the full repository, generated artifacts, deploy manifests, CI, docs, and images for active Primer Identity/Stytch, old JWKS, shared-key, pre-minted bearer, legacy password/session, and alias references; classify history-only docs explicitly.
- Inspect route startup with secrets absent; reject inert/open guards and default test issuers in production.
- Verify removed code is not copied into a renamed compatibility package or hidden behind an undocumented rollback flag.
- Check schema cleanup preconditions/counts and foreign keys; ensure canonical/local memberships and device rows are not cascade-deleted.
- Confirm device utility packages were split/preserved where legacy human and device tokens once shared code.
- Inspect browser bundles/storage/network for old bearer/admin-key/Identity paths.
- Verify live Clerk proof and M2M flows use production composition, not test-only authenticators or context injection.
- Inspect logs, CI artifacts, backups, migration reports, screenshots, and handoff docs for tokens, secrets, emails, provider bodies, or private keys.
- Confirm protected-master history and PR checks are green; no force push, direct unreviewed master update, skipped test, lowered coverage, or test-only production branch.
- Validate the wave-02 handoff commit exists on protected master and all stated policies are asserted by tests.

## Completion Gate

- [ ] Explicit retirement approval and observation/rollback-window evidence are recorded.
- [ ] Primer Identity/Stytch runtime/deploy/provider resources and all superseded product auth paths are removed or archived per policy.
- [ ] Old JWT/shared-secret/legacy session/test issuer credentials permanently fail on human/service routes.
- [ ] Clerk human session and JWT M2M flows, canonical mappings, local authorization, Organization/tenant isolation, JWKS/M2M rotation, outage/revocation, and redaction pass live and deterministic tests.
- [ ] Pre-existing devices survive cleanup and the full pairing/revoke/rotate/re-pair/cross-credential matrix remains green.
- [ ] Fresh deploy/migrate and backup/restore drills pass without Identity runtime.
- [ ] OpenAPI/clients, workspace/dependencies, web bundles, config, CI, containers, Compose/Stacklane/Nomad, architecture, and runbooks are reconciled.
- [ ] Full test/race/coverage/browser/Android/lint/type/build/security/deploy gates and independent Terra rerun are green.
- [ ] Cleanup landed through protected-master green PRs without force-push.
- [ ] Exact protected-master handoff tip and auth boundary summary are published for wave 02 Ultracore.
