# Phase 3: Identity-authenticated API and generated clients

## Goal

Publish the service's authenticated durable control-plane API and generate its official clients. This phase makes runs/sessions/events controllable across processes without yet claiming worker execution. Afterward authorized callers can create queued runs, inspect status, request cancellation, and page committed events through a stable OpenAPI-derived contract.

## BDD Success Criteria

#### Scenario: Primer Identity JWT with exact agents audience is accepted

- **Given** a Primer Identity ES256 access JWT signed by a key in configured JWKS, with expected issuer, `aud=primer-agents`, bounded lifetime, public signed `client_id`, and required scope
- **When** the caller creates and reads a run through `/agents/v1`
- **Then** the request is authenticated as the copied Identity principal
- **And** the persisted owner namespace is derived from signed claims rather than request role/owner fields
- **And** the raw JWT is discarded and absent from records/logs

#### Scenario: Non-Identity and wrong-authority credentials fail closed

- **Given** a missing, malformed, expired, wrong-issuer, wrong-audience, unknown-key, overlong, or raw-Stytch-shaped bearer value
- **When** any protected endpoint is called
- **Then** it receives the same safe unauthorized response without handler/repository mutation
- **And** no fallback to LMS sessions, static API keys, `X-Service-Token`, or unverified JWT payload occurs

#### Scenario: Caller ownership and scope isolate records

- **Given** two valid principals or signed client namespaces and a run owned by the first
- **When** the second requests status, events, or cancellation by guessed ID
- **Then** it receives not-found/forbidden according to the frozen disclosure policy
- **And** no prompt/event/status detail or cancellation side effect is exposed
- **And** caller-supplied learner/workspace/role/profile fields cannot change the owner namespace

#### Scenario: Durable create/status/cancel/event-page contract is real

- **Given** an authenticated generated client and real PostgreSQL
- **When** it creates a run, repeats the idempotency key, gets status, requests cancellation, and pages events
- **Then** responses reflect the durable rows and lifecycle from Phase 2
- **And** repeated create/cancel semantics match the frozen idempotency/state rules
- **And** the API does not hard-code completion or require one HTTP request to stay open

#### Scenario: Generated clients are the contract consumers

- **Given** handler signatures and committed OpenAPI
- **When** contract/client generation runs twice
- **Then** OpenAPI and generated Go/TypeScript clients are deterministic and unchanged on the second run
- **And** both generated clients complete the same authenticated create/get/cancel/event-page flow
- **And** a drift or breaking-contract gate fails on stale generated artifacts

#### Scenario: Product authorization remains outside the service without becoming untrusted role input

- **Given** a caller-authorized opaque learner/workspace/job reference
- **When** it is attached as bounded context to a run
- **Then** `primer-agents` stores and returns it only inside the signed caller namespace
- **And** it does not query or join the caller's database
- **And** it selects execution authority from signed client/scope and server configuration, never from the opaque reference

## Implementation Instructions

- Build a fail-closed consumer validator in `primer-agents/internal/authn`, following the reviewed Studio pattern but owned by this module. Validate strict compact ES256 `typ=at+jwt`, exact configured issuer, exact frozen audience `primer-agents`, lifetime/skew, `jti`, `client_id`, scope, and JWKS key rotation bounds. Production requires HTTPS issuer/JWKS and rejects test/loopback mode.
- Identity remains the only Stytch client. Do not import a Stytch SDK, expose a Stytch token field, or call Stytch. Test tokens must be minted by Identity's production token profile or an external cryptographic fixture matching it, not by a permissive unsigned parser.
- Define server-owned scopes/profile admissions (for example run read/write/cancel, sessions, jobs, student) and caller namespace composition from validated subject class, subject, public `client_id`, and audience. Product resource authorization stays upstream; the service still blocks cross-namespace record access.
- Register Huma/chi routes under `/agents/v1` for the Phase-2 capabilities: create/get/list runs, idempotent cancel request, event page by sequence cursor, and minimal session create/get needed for later phases. Keep health/readiness unprotected.
- Use explicit safe problem/error responses for unauthorized, forbidden/not-found, conflict, invalid transition, rate/body limit, and unavailable. Provider/internal errors are not yet relevant and must not be invented.
- Emit OpenAPI offline from production handler signatures. Generate official Go and TypeScript clients in agents-owned client directories with pinned tools. Generated event/run/session types are the only consumer DTO source. If an SSE transport helper is later handwritten inside the generated package, it must use generated types and be tested against the same OpenAPI route.
- Add deterministic generation, breaking-change/baseline, no-handwritten-DTO, and raw-transport import gates. Document versioning, idempotency header, cursor, error, and auth semantics.
- Register/configure audience/client/scope in credential-free Identity fixtures for E2E. Production client registration/grant rollout is an external deployment prerequisite; do not claim live Stytch/BFF acceptance.
- A create response in this phase is honestly `queued`; no fake worker may move it to success.

## End-to-End Test Plan

- Run a real `primer-agents` process with real PostgreSQL and a real loopback JWKS endpoint. Mint a strict Identity-profile token and use the generated Go client for create/retry/get/cancel/event-page.
- Repeat the flow using the generated TypeScript client against the same process in a Node test harness.
- Test wrong issuer/audience/key/signature/lifetime/client/scope, malformed and raw-Stytch-shaped opaque bearers, and JWKS outage/rotation. Assert no rows mutate on denial and logs contain no token fragments.
- Seed two principals/client IDs, attempt IDOR status/events/cancel, and inspect the DB to prove no exposure/side effect.
- Regenerate OpenAPI and clients twice into temporary paths and compare committed artifacts; run breaking-change and no-raw-transport scans.
- Commands to add/use: module `make openapi`, `make clients`, `make contracts-check`, `make e2e`; `go test ./internal/authn/... ./internal/api/...`, then module/full repository gates.

Permitted external dependencies are real PostgreSQL and a loopback JWKS server serving real signatures. No mock bearer middleware or in-memory repository may satisfy E2E.

## Anti-Cheating Audit

- Trace Authorization extraction through signature/JWKS/issuer/audience/scope validation to repository ownership predicates. Reject decode-without-verify, payload role trust, static-secret fallback, or test auth in production.
- Search dependencies and wire types for Stytch SDKs, SessionJWT/session token names, provider tuple fields, `X-Service-Token`, or raw bearer persistence/logging.
- Inspect every get/list/events/cancel query for owner namespace predicates; client filtering is not authorization.
- Exercise generated clients in E2E; reject direct `fetch`, hand-built URLs, duplicated DTOs, or tests limited to OpenAPI text snapshots.
- Confirm create returns the durable queued record and handlers do not synthesize success/events.
- Verify product opaque refs do not become policy selectors and no caller database DSN/client is introduced.

## Completion Gate

- [ ] All BDD scenarios pass through the real process with real PostgreSQL and signed JWKS tokens.
- [ ] Exact `aud=primer-agents` and scope/namespace isolation are enforced; raw Stytch-shaped credentials fail.
- [ ] OpenAPI and generated Go/TypeScript clients are deterministic, current, and exercised end to end.
- [ ] Durable queued/status/cancel/event-page semantics match Phase 2; no fake execution exists.
- [ ] Contract-breaking, no-handwritten-DTO, and no-raw-transport gates pass.
- [ ] Auth, API, module coverage/race, and existing repository gates pass unchanged.
