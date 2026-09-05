# Primer Tasks — standalone task and agentic-verification plan

## Outcome

Deliver **Primer Tasks**, a standalone Primer-adjacent product that gives each
student a scheduled checklist and completes each occurrence only after its
configured verification requirements succeed. Parents administer students,
tasks, schedules, browser sessions, and exceptions through a browser SPA or a
WebSocket-streamed Fantasy agent. Students use the same SPA; conversational,
artifact, human, and external verification methods share one durable
attempt/decision model. Native pairing/checklist baselines exist only on the
preserved donor `impl/tasks-p6-external` (not on `origin/master`); those
baselines and all native work formerly attached to Phases 4–7 now live in the
separate [Primer Tasks Android continuation plan](../primer-tasks-android/index.md).

The completed standalone loop is:

```text
parent login → student + task → schedule → student checklist
    → verification attempt (chat, media, parent, or external)
    → durable decision → checked occurrence → parent audit
```

Primer Tasks is a separate deployable and database. Future Primer integration
uses authenticated APIs and durable events, never a shared database.

## Current-state summary

Repository evidence at the planning base (`2e55dd3`):

| Area | Current state | Evidence |
|---|---|---|
| LMS and TV service stack | Huma v2 + chi + pgx/PostgreSQL, offline OpenAPI emission, React/Vite SPAs, testcontainers | `server/`, `web/`, `tv-web/`, root `Makefile` |
| Student/device pairing | One-use hashed codes and student-bound opaque device tokens exist in LMS and TV, but not for this product or QR pairing | `server/internal/repo/student_device.go`, `server/internal/api/student_api.go`, `server/internal/tv/api/device.go` |
| Android | Existing `android/` app is the TV client; it is not a task client and must not share package identity or state | `android/README.md` |
| Agent runtime | LMS has a process-local MAF runtime; the requested standalone Fantasy runtime does not exist | `server/internal/agent/`, `agent_docs/plans/primer-maf-runtime/` |
| Fantasy | Not a production dependency. Local module cache and upstream release expose streaming text/reasoning/tool callbacks and typed tools at `charm.land/fantasy v0.41.1` | inspected Fantasy `README.md`, `agent.go`, `tool.go` |
| Identity | Primer Identity is a separate issuer/broker and remains mid-delivery; product BFF/live cutover cannot be assumed complete | `primer-identity/README.md`, `agent_docs/plans/primer-identity-service/index.md` |
| Design system | Primer System C is the stronger local visual authority; dark is primary, square/ruled, generated tokens, no chat bubbles | `design-system/README.md`, `design-system/generated/` |
| Compose/Stacklane | Host Make is the default non-Docker path. An opt-in LMS/TV Compose stack exists and must not become the Tasks default; Tasks still needs its own additive opt-in vector if Compose is used at all | `AGENTS.md`, `docs/dev-compose.md` |
| Task verification product | No standalone task/schedule/occurrence/verification service, SPA, or Android app exists | repository inventory |

This plan therefore creates new product code under `primer-tasks/`; it does not
repurpose LMS assignment/mastery tables, the TV Android app, or the LMS MAF
runtime.

## Architecture and ownership

```text
                     Primer Identity (future/live)
                         or dev test issuer
                                 │ OAuth/OIDC
                                 ▼
┌──────────────────────── Primer Tasks ─────────────────────────┐
│ Go modular monolith (`tasks-server`)                          │
│  BFF/auth · students/devices · task revisions · schedules    │
│  occurrence engine · verification registry · Fantasy runtime │
│  jobs · WebSocket hub · outbox · generated REST/WS contracts │
│             │                         │                        │
│             ▼                         ▼                        │
│       PostgreSQL                 object storage               │
│  source of truth + jobs       media/artifact bytes            │
└──────────────────────────┬────────────────────────────────────┘
                           │ same-origin REST/WS
                           ▼
                 React parent/student SPA

Native clients consume the same service contracts on an independent delivery
track; they do not gate Phases 4–7 of this plan.
```

### Planned product tree

```text
primer-tasks/
  go.mod
  cmd/{tasks-server,tasks-migrate,openapi-gen}/
  internal/{api,app,auth,bff,config,db,domain,repo,schedule,
            verification,agent,jobs,artifacts,outbox,observability,testutil}/
  clients/{typescript,kotlin}/        # committed façade/config only
  web/                                # parent + student route groups
  android/                            # dedicated student application
  compose.yaml                        # opt-in Stacklane-compatible Compose; not the default host path
  scripts/dev                         # opt-in Compose/Stacklane lifecycle vector only
  test-artifacts/                     # text manifests/reports, not secrets/media
```

Generated OpenAPI, JSON Schema, TypeScript, and Kotlin source live only in
ignored build directories. Production route registration and Go boundary types
are the REST contract source. WebSocket message Go types constrain runtime
encoding and drive offline JSON Schema/type generation; the small client
transport façades may open sockets but may not duplicate message DTOs.

## Core domain decisions

### Tenancy and actors

- `tenants` represent a family/household from phase 1, even while onboarding is
  single-household.
- Parent identity is a Primer Identity `sub` mapped through
  `parent_memberships`. The only initial parent role is `admin`; role vocabulary
  may expand later without granting implicit authority.
- Every student, task, schedule, occurrence, attempt, artifact, agent run,
  outbox row, and audit row is tenant-owned. Composite foreign keys and
  tenant-scoped repositories prevent cross-tenant attachment and IDOR.
- Student browser sessions and Android device credentials are separate from
  parent sessions. A student credential is permanently bound to one student;
  changing students requires revoke and re-pair.

### Task and schedule model

- `task_templates` have immutable `task_revisions`. Parent updates publish a new
  revision; existing occurrences retain the revision and verification snapshot
  they were created with.
- `task_schedules` target one student and a task revision. They support one-off
  dates and a bounded RFC 5545 RRULE subset with an explicit IANA timezone,
  start/end bounds, due offset, and enabled state.
- A materializer creates `task_occurrences` with uniqueness on
  `(schedule_id, nominal_at)`. Retries and restarts cannot duplicate work.
- Occurrence states are server-owned: `pending`, `in_progress`,
  `awaiting_verification`, `completed`, `excused`, and `canceled`. Client code
  cannot write `completed` directly.

### Verification abstraction

Each task revision contains one or more immutable
`verification_requirements`; initial completion policy is **all requirements**.
A registry manifest identifies:

- stable `kind` and config schema version;
- interaction capability: `parent_action`, `chat`, `artifact`, `form`, or
  `external`;
- executor: `human`, `deterministic`, `fantasy`, or `external`;
- configuration and submission JSON Schemas;
- retry, timeout, and evidence-retention policy.

The generic engine owns `verification_attempts`, ordered submissions/messages,
evaluations, and immutable accept/reject decisions. Drivers may propose a
transition but never update occurrence completion directly. The engine validates
requirements, uses compare-and-set transitions, records evidence and provenance,
and completes the occurrence only after policy is satisfied.

Initial drivers are:

1. `parent_approval` — a tenant admin explicitly approves or rejects.
2. `agent_dialogue` — a narrowly tooled Fantasy loop conducts a question-based
   verification such as three chapter questions.
3. `agent_artifact_rubric` — an asynchronous Fantasy/multimodal evaluation of a
   submitted artifact against a snapshotted rubric.
4. `external_callback` — a signed, idempotent request/result protocol for an
   allowlisted external verifier.

Unknown kinds or unsupported schema versions fail closed and remain visibly
blocked; they never degrade to self-check or automatic completion.

### Agent and streaming model

- Pin `charm.land/fantasy v0.41.1`; Bedrock is the primary configured provider
  and OpenRouter/OpenAI-compatible is an optional fallback. Deterministic
  scripted `LanguageModel` fixtures drive default tests.
- Parent agent tools manage students, task drafts/revisions, schedules, and
  occurrence queries. Destructive or broad actions require a preview and an
  explicit confirmation handle. Tenant/actor scope comes from server context,
  never model arguments.
- Student verification agents receive only the requirement-specific prompt and
  tools. They cannot CRUD tasks, schedules, students, or completion state.
- Agent runs are durable jobs detached from a socket. Conversation, tool-result,
  status, and final-message records survive reconnect/restart; transient text
  deltas may be coalesced while final messages remain durable.
- WebSockets stream text deltas and safe progress (`thinking`, named safe tool
  activity, retry, evaluating, complete). Raw hidden chain-of-thought/reasoning
  deltas are **not** shown or stored; Fantasy reasoning callbacks drive only a
  generic indicator and optional short post-run rationale.
- A bounded subscriber queue may disconnect a slow client, but cannot cancel or
  fail the underlying run. Authorized clients reconnect with a durable cursor.

## Scope boundaries

### In scope

- Standalone Go service, own PostgreSQL, own object store, own migrations and
  configuration namespace (`TASKS_`).
- Parent BFF login shell, tenant membership, multiple parent principals with the
  single initial `admin` role, student CRUD/archive, and tenant isolation.
- Parent and student SPA route groups using Primer System C.
- Versioned task definitions, schedules, occurrences, manual approval, Fantasy
  chat and non-chat verification, media evidence, external verification.
- WebSocket streaming, progress indicators, reconnect, durable jobs, audit and
  observability.
- Handler-derived REST contracts and generated TypeScript/Kotlin clients.
- Host Make / non-Docker development as the primary path (`make tasks-*`,
  local Postgres via existing host targets/testcontainers). An opt-in
  Stacklane-compatible Compose vector with hot reload for Go and Vite, direct
  loopback fallback, real PostgreSQL/object storage, and worktree isolation is
  additive only; do not treat it as the default host command. Phases that add
  or change that vector must still test it.
- Mandatory independent exploratory E2E and promoted automation in every phase.

### Out of scope

- Sharing the LMS, TV, Identity, or Studio databases; cross-database joins/FKs.
- Importing LMS assignments/mastery or claiming task completion is academic
  mastery.
- Production Primer embedding or bidirectional synchronization. The final phase
  proves an integration seam only.
- Parent roles other than `admin`, billing, district/school administration, or
  public self-service tenant signup.
- General workflow graphs or arbitrary code loaded from task configuration.
- Showing model chain-of-thought.
- Offline completion guarantees; clients may cache read state, but server
  verification remains authoritative.
- Treating scripted models as proof of pedagogical or multimodal model quality.
- Native-client work after the donor-only Phase 1–2 baseline on
  `impl/tasks-p6-external` (not on `origin/master`); dialogue, media,
  external-verifier, and release continuation are owned by
  [`../primer-tasks-android/`](../primer-tasks-android/index.md) and do not block
  Phases 4–7 here.

## Global constraints

1. **SOA isolation:** Primer Tasks owns its database, object namespace,
   credentials, migrations, and deploy lifecycle. All future Primer traffic is
   authenticated HTTP/events.
2. **Tenant scope from day one:** no temporary global CRUD or client-only
   filtering. Every public query/mutation and agent tool enforces tenant scope
   server-side; two-tenant negatives begin in phase 1.
3. **Auth split:** live parent auth targets Primer Identity OAuth/OIDC and a
   host-only BFF session. A protocol-compatible local test issuer is allowed in
   host and opt-in Compose dev/test only; production must reject it before
   migration/listen.
4. **Device custody:** pairing codes are random, hashed, single-use, short-lived,
   and shown once. Device tokens are stored only as hashes server-side and in an
   Android Keystore-encrypted local store; never in QR URLs, logs, or backups.
5. **Typed API ownership:** Huma handler signatures and explicit boundary types
   are the REST source of truth. Offline emission requires no DB/provider. Web
   calls use the separately generated TypeScript client/façade; raw transport is
   allowlisted only for health, binary upload, and WebSocket machinery. Native
   generated-client policy is owned by the separate platform plan.
6. **Durable decisions:** agent text is not completion. Only a committed,
   immutable verification decision through the domain service can satisfy a
   requirement and complete an occurrence.
7. **Idempotency:** schedule generation, pairing claim, message submission,
   artifact finalization, job execution, tool mutations, decisions, and external
   callbacks have stable idempotency keys and replay tests.
8. **Bounded inference:** finite steps/tokens/time/retries, bounded prompt and
   artifact sizes, cancellation, cost/usage records, and no provider credentials
   in clients/logs.
9. **Safe progress:** stream text and progress; do not expose hidden reasoning.
10. **System C:** consume `design-system/generated/primer.css`; dark primary/
    light parity, square ruled components, names before IDs, no shadows/gradients/
    chat bubbles, keyboard/a11y/mobile-browser evidence.
11. **Server-owned collections:** parent lists use bounded server-side search,
    filter, sort, and pagination with URL state; no bulk-fetch/client filtering.
12. **Opt-in Stacklane/Compose contract:** when the additive Compose vector is
    used, publishing services use required
    `stacklane.enable/project/instance/endpoint/port` labels and
    `127.0.0.1::<containerPort>` publishes. That vector's commands use the same
    ordered Compose invocation and `-p primer-tasks-$INSTANCE`; no `.local`,
    fixed host port, wildcard publish, or host network. Host Make targets remain
    the default and are not routed through Compose.
13. **Hot reload proof:** Go watcher and Vite HMR must be proven by source
    mutation/restore; Vite HMR must update without full reload. Source mounts are
    worktree-local; caches/state are named volumes.
14. **No weakened gates:** add product-local build/test/coverage/lint/codegen/E2E
    targets without lowering Primer's existing 85% Go coverage expectation.
15. **Forward migrations:** published data is preserved; destructive rollback is
    not the normal phase rollback.

## Mandatory per-phase E2E promotion protocol

Every remaining phase exposes a user-visible browser flow. Native emulator flows
are specified and gated independently in
[`../primer-tasks-android/`](../primer-tasks-android/index.md).

1. **Implementation agent** completes the vertical slice and focused tests.
2. A **dedicated exploratory E2E agent** (not the implementer) starts the real
   Tasks service through the default host Make/non-Docker path and exercises
   the phase through browser public boundaries. It records actions,
   desktop/mobile screenshots, network/console failures, and database-visible
   outcomes. When the phase adds or changes the opt-in Compose/Stacklane
   vector, the same agent also starts that additive stack and proves
   isolation/hot-reload against it. Compose is never the universal/default host
   lifecycle.
3. The implementer fixes findings and the exploratory agent re-runs until PASS.
4. Only after exploratory PASS, a **test-promotion agent** writes/extends the
   Playwright suite from those observed browser steps.
5. A **review agent** runs the phase anti-cheating audit and all completion
   commands. The phase branch is integrated only after this review.

Playwright must use the generated client through the UI, not seed final state by
calling private repositories. API setup helpers may create prerequisites through
public admin endpoints. Screenshots and reports are evidence, not replacements
for assertions.

## Phase overview

| Phase | Goal | Depends on |
|---|---|---|
| [Phase 1: Standalone identity, pairing, and client shells](./phase-01-foundation-pairing.md) | Run the default host Make path; parent logs in, creates a tenant-scoped student, displays a QR, and web/Android pair to that student. Add and test an opt-in Compose/Stacklane vector only as an additive lifecycle. | None |
| [Phase 2: Tasks, schedules, checklist, and parent approval](./phase-02-tasks-schedules-manual.md) | Parent creates/schedules versioned tasks; students see occurrences; parent approval completes the manual-verification example. | Phase 1 |
| [Phase 3: Fantasy runtime and parent command chat](./phase-03-parent-agent-chat.md) | Durable Fantasy jobs and WebSockets stream a parent agent that safely manages tasks and schedules with tools. | Phase 2 |
| [Phase 4: Student dialogue verification](./phase-04-student-dialogue-verification.md) | Student SPA completes a reading task through a streamed, requirement-scoped three-question Fantasy conversation. | Phase 3 |
| [Phase 5: Web media evidence and asynchronous rubric review](./phase-05-media-rubric.md) | Student SPA submits image/video/audio evidence; image rubric evaluation runs without chat and streams progress. | Phase 4 |
| [Phase 6: External verifier protocol](./phase-06-external-verifiers.md) | Allowlisted external tools receive signed idempotent jobs and return durable results through the same verification engine. | Phase 5 |
| [Phase 7: Multi-user operations and Primer-ready release](./phase-07-release-integration-readiness.md) | Add admin invitations, audit/retention/backup/observability, web release hardening, and authenticated API/event seams for later Primer integration. | Phase 6 |

## Requirement traceability

| Requirement | Phase success/evidence |
|---|---|
| Different verification methods per task | P2 manual driver; P4 dialogue; P5 artifact rubric; P6 external callback |
| Read chapter → three questions | P4 browser dialogue scenarios; native continuation is Android Phase 1 |
| Poem picture → rubric | P5 web image artifact + asynchronous rubric; native continuation is Android Phase 2 |
| Brush teeth → parent checkoff | P2 parent-approval occurrence scenario |
| Standalone now; Primer SOA later | P1 own DB/service; P7 authenticated integration seam and no cross-DB audit |
| Future multi-tenant/multi-user; all parents admins initially | P1 tenant/membership model + two-tenant isolation; P7 invite flow; only `admin` role throughout |
| Parent CRUD students/tasks/schedules | P1 student operations; P2 task/schedule operations; P3 equivalent agent tools |
| Abstract verification and external tools | registry in P2/P4, media in P5, signed external protocol in P6 |
| Parent WebSocket Fantasy chat | P3 |
| Student chat for relevant verification | P4 |
| Agent verification without chat | P5 |
| Streaming and thinking/progress indicators | P3–P5 WS scenarios; raw reasoning exclusion audit |
| Parent + student web SPA | P1 shells, P2 functional workflows, all later phases |
| Android QR pairing and persistent single-student auth | P1; CameraX primary, with the documented exact-image Photo Picker/SAF emulator fallback only for the recorded upstream VirtualScene blocker |
| Native camera/files for image/video/audio | Separate Android continuation plan, Phase 2 |
| Mandatory dedicated E2E then Playwright per phase | global browser promotion protocol + every phase E2E/completion gate |
| Native emulator continuation | Separate Android plan; main Phases 4–7 do not wait for it |
| Isolated opt-in Compose/Stacklane, Go/Vite hot reload | P1 additive vector; re-proved in P7 when that vector changes |

## Delivery/orchestration contract

- One phase is one reviewed branch/worktree and one coherent vertical slice.
- A phase orchestrator may create implementer, E2E, and reviewer children, but
  orchestration depth must stay within planner → orchestrator → leaf agent.
- Phases integrate in order. A later orchestrator is dispatched only from the
  reviewed tip of its dependency; it must not guess unpublished schemas.
- The phase orchestrator commits implementation and an evidence report. It does
  not push, open a PR, merge to `master`, or claim a blocked live-provider gate
  without user authorization.
- RED browser E2E, cross-tenant leakage, stale generated clients, hard-coded
  success, or a lowered gate stops integration and is reported. Native gates are
  reported and sequenced independently in the Android continuation plan.

Suggested branch sequence:

```text
impl/tasks-p1-foundation
impl/tasks-p2-checklist
impl/tasks-p3-parent-agent
impl/tasks-p4-dialogue
impl/tasks-p5-media
impl/tasks-p6-external
impl/tasks-p7-release
```

## Completion rule

Primer Tasks is complete only when every main-plan BDD scenario passes through
its real public boundary; each remaining phase has independent browser
exploratory evidence followed by promoted Playwright automation. The independent
Android continuation may remain in progress without blocking this web/server
completion claim and retains its own stricter emulator completion rule. Two
tenants remain isolated across REST, WebSocket, agent tools, object keys, jobs,
and events; default host Make/testcontainer gates are green; when the opt-in
Compose vector is in scope, Compose check/hot-reload/two-instance proofs pass;
generated clients are freshly built from offline-emitted contracts with no
tracked generated source; Fantasy
and provider failures are bounded and observable; image rubric review has an
explicit controlled live-provider qualification before any model-quality claim;
backup/restore and device revocation drills pass; all builds/lints/tests/coverage
gates are green; and the anti-cheating audits find no in-memory, mocked
first-party, hard-coded, client-only, or cross-database substitution.
