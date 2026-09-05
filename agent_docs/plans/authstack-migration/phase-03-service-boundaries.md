# Phase 3: Service boundary migration

## Goal

Replace service-to-service shared secrets, pre-minted environment JWTs, handwritten token forwarding, and custom HTTP/gRPC/MCP validation with audience-specific authstack M2M credentials. Each caller authenticates as itself to one destination audience; target services authorize narrow product permissions. Human and device authentication remain unchanged in this phase.

Service migration precedes human cutover so background work and downstream calls no longer depend on Primer Identity service principals or shared browser/admin credentials when browser sessions move.

## BDD Success Criteria

#### Scenario: TV reporter authenticates only to LMS ingest

- **Given** the TV reporter has an M2M client permitted for the LMS audience
- **When** it posts a completed educational viewing to `/instruction-logs/ingest`
- **Then** LMS accepts the authstack service principal with `instruction-logs:ingest`
- **And** idempotent `(source, source_ref)` behavior is unchanged
- **And** the same token is rejected by TV admin, Studio, Agents, human, and device routes.

#### Scenario: Content ingest has narrow TV machine authority

- **Given** content ingest has a TV-audience M2M credential
- **When** it performs an allowed catalog/import operation
- **Then** TV authorizes only its configured machine permissions
- **And** the token cannot authenticate the TV browser-admin session or paired-device routes
- **And** removal of `X-Admin-Key` does not make admin routes public.

#### Scenario: Downstream tokens replace inbound authorization

- **Given** an LMS/Studio request arrived with a human or device bearer
- **When** authorized application code calls Primer Agents, Studio, TV, or another service
- **Then** the outbound authstack transport replaces the inbound authorization with a destination-audience service token
- **And** the destination principal identifies the calling service, not the end user/device
- **And** product audit context is passed only as bounded non-credential metadata.

#### Scenario: gRPC authentication precedes complete authorization

- **Given** every Studio gRPC unary and stream method has a registered policy
- **When** a service invokes a permitted or forbidden method
- **Then** missing/invalid credentials return `Unauthenticated`
- **And** valid credentials without permission return `PermissionDenied`
- **And** missing method policy returns `Internal`
- **And** no unregistered read/ack method bypasses authorization.

#### Scenario: Credential acquisition is cached by audience and safely retried

- **Given** a service makes repeated calls to one destination and then another
- **When** authstack obtains client-credentials tokens
- **Then** valid tokens are reused only for the same audience
- **And** expiry refreshes them without forwarding old inbound credentials
- **And** provider outage yields bounded unavailable/retry behavior without logging secrets or dropping durable work.

#### Scenario: Legacy service path can roll back during the window

- **Given** a boundary is in `dual` or `authstack` with the old secret/JWT adapter retained
- **When** an operator selects the documented per-boundary rollback state
- **Then** only that boundary returns to its old credential path
- **And** queued/retried work remains idempotent
- **And** no schema or credential deletion is required.

## Implementation Instructions

1. Complete the exact caller→target inventory, including TV→LMS, content-ingest→TV, LMS→Primer Agents, Studio→Primer Agents if active, LMS→Studio gRPC/HTTP if active, MCP service clients, and any Tasks integrations added by wave 00. Do not migrate third-party API keys (Bedrock, Jellyfin, Radarr/Sonarr, yt-dlp) as though they were Primer service identity.
2. Provision one selected-provider M2M client or workload identity per caller with one or explicitly bounded audiences/scopes. Record provider object names and secret-manager references, never values. Avoid sharing one “Primer service” credential.
3. Construct `oauth.NewClientCredentialsTokenSource` at caller composition roots. Use `authhttp.TokenTransport` and `authgrpc.UnaryClientInterceptor` with explicit audiences. Replace existing authorization; never copy inbound HTTP headers or gRPC metadata.
4. Replace LMS `FailClosedSharedSecretGuard` on instruction ingest with authstack M2M authentication plus permission authorization. Keep idempotency and entertainment rejection independent of authentication.
5. Split TV's mixed `requireAdmin` behavior into human-admin and machine-admin route policies. Add local TV human membership separately; migrate content-ingest and other machine callers off `X-Admin-Key`. Do not expose pairing-code issuance to broad service permission by accident.
6. Replace `server/internal/remoteagent.EnvTokenSource` and generated-client raw bearer injection with an authstack token transport/source. Retain the invariant that LMS product authorization occurs before an Agents call and preserve no-duplicate-after-acceptance behavior.
7. Replace Studio `X-Service-Token` alias and handwritten gRPC authentication with authstack. Build complete exact full-method authorization registries for unary and stream RPCs. Review every method currently lacking an explicit scope check.
8. Migrate Primer Agents middleware/custom principal to authstack for service callers and map existing scopes/profile admission behind `auth.Authorizer` or a narrow application adapter. Preserve owner subject and idempotency semantics.
9. If Primer Tasks has service boundaries, migrate only parent/service calls; leave student browser/Android bearer, pairing, WebSocket, and device sessions untouched.
10. Implement per-boundary dual metrics and rollback flags. Legacy secret paths remain disabled for new deployments once each boundary is certified but are not deleted until Phase 7.
11. Update OpenAPI security schemes, generated clients/transports, config validation, deploy manifests, secret-manager names, and runbooks without committing credentials.

## End-to-End Test Plan

- Use real service processes and real PostgreSQL stores to run TV reporter→LMS instruction ingest. Complete/retry the same viewing and assert one instruction log and one TV reporting ledger entry. Repeat with wrong audience, human token, TV device token, and missing permission.
- Run content-ingest plan/apply against the TV test process with a selected-provider M2M fixture. Assert allowed catalog operation succeeds and browser-admin/device-only operations deny.
- Run LMS→Primer Agents through the generated client and real authstack outbound transport. Seed an inbound parent/device authorization header and capture the downstream request in a controlled TLS test target; assert it contains the service token for `primer-agents`, not inbound material.
- Start Studio gRPC and call every full method with missing, malformed, valid-permitted, valid-forbidden, wrong-audience, and human credentials. Include stream interceptors if any streams exist. Assert canonical gRPC codes and missing-policy failure.
- Exercise provider/token-endpoint outage with durable TV reporting or job rows: work remains queued/retryable, retries are bounded, and recovery sends exactly once/idempotently.
- Run concurrent calls to two audiences and verify token-source requests/caches do not cross audiences.
- Run legacy rollback for one boundary while another remains authstack, proving flags are independent.
- Execute focused service tests, `make test`, `make tv-test`, `make studio-test`, Primer Agents tests, `make tasks-test`, race tests around token cache/reporter/jobs, generated client checks, and coverage gates.

A local selected-provider token endpoint or authstack provider fixture may test deterministic acquisition/replacement. At least one real development-provider M2M flow is required before Phase 3 completion.

## Anti-Cheating Audit

- Search outgoing clients for copied `Authorization`, incoming-context token access, raw bearer constructor arguments, `EnvTokenSource`, `X-Service-Token`, and `X-Admin-Key`.
- Verify third-party credentials remain separate and are not accepted by Primer auth middleware.
- Inspect destination audience on every token transport and provider registration; reject wildcard/multi-service audience shortcuts.
- Check machine tokens cannot reach human-session endpoints or product-local device routes.
- Ensure every Studio gRPC method—not only mutation probes—has explicit authorization after authentication.
- Confirm outage/retry tests inspect durable source-of-truth rows, not only mock call counts.
- Reject tests that preinstall an auth principal in context instead of traversing real middleware/interceptors.
- Inspect logs, errors, audit metadata, and test failure output for token/client-secret/provider-body leakage.
- Verify rollback does not enable both legacy and authstack credentials on unrelated boundaries or make empty config inert.

## Completion Gate

- [ ] Every Primer caller→target edge uses a caller-specific, destination-audience authstack service token.
- [ ] Shared service headers, pre-minted env JWTs, and inbound bearer forwarding are no longer authoritative on migrated edges.
- [ ] Target permissions are least-privilege and distinct from human/device policies.
- [ ] HTTP, gRPC unary/stream, and MCP service policies have complete negative/authorization coverage.
- [ ] Real provider M2M, outage/recovery, replay/idempotency, and independent rollback evidence pass.
- [ ] Generated clients/specs, configs, deploy manifests, runbooks, focused/full/race/coverage/build tests pass.
- [ ] Legacy adapters remain available only for explicit rollback and are scheduled for Phase 7 removal.
- [ ] Anti-cheating audit confirms token replacement, audience isolation, durable retries, and no credential leaks.
