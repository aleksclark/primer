# Primer

Primer is a family-directed homeschooling system for mastery-based instruction,
practice, practical projects, and parent-reviewed evidence. Its name comes from
the Young Lady's Illustrated Primer in Neal Stephenson's *The Diamond Age*.
The parent remains the primary educator; AI is an adaptive tool, not a substitute
for parental judgment or human relationship.

## Repository map

This is a multi-module Go workspace plus web, Android, and NixOS applications.
There is **no root `go.mod` or `cmd/primer`**. [go.work](go.work) lists the five
service modules and the separately versioned Agents Go client module.

| Component | Source / entry point | Responsibility |
| --- | --- | --- |
| LMS | `server/cmd/primer-server`, [web/](web/README.md) | Students, standards, curricula, assignments, evidence, mastery, instructional time, parent administration |
| TV | `server/cmd/tv-server`, [tv-web/](tv-web/README.md) | Catalog, availability, programmed channel, grants, watch-once accounting, instructional-time reporting |
| Content ingest | `server/cmd/content-ingest` | Manifest-driven Radarr/Sonarr/yt-dlp/Jellyfin/TV reconciliation |
| Curriculum Studio | [curriculum-studio/](curriculum-studio/README.md) | Independent curriculum authoring, validation, publication, materialization, exports, projects, collaboration, REST/gRPC/MCP surfaces |
| Primer Identity | [primer-identity/](primer-identity/README.md) | Stytch-backed human-auth broker, Primer OAuth grants/JWTs/JWKS, service principals, revocation |
| Primer Agents | [primer-agents/](primer-agents/), `cmd/primer-agents` | Standalone MAF runtime with PostgreSQL run/session/event records and replayable SSE |
| Primer Tasks | [primer-tasks/](primer-tasks/), `cmd/tasks-server`, `web/` | Separate task/checklist service, local household authorization, parent agent chat, student verification |
| Android | [android/](android/README.md) | TV (`:app`), Student (`:app-student`), Control (`:app-control`), shared libraries; consumes generated Tasks client |
| Student workstation | `server/cmd/primer-student`, [workstation/](workstation/README.md) | Broker/TUI, local activity execution, evidence outbox, NixOS packaging |
| Investor site | [investor-web/](investor-web/LAUNCH.md) | Public investor narrative and diligence; independent from authenticated administration |
| Shared browser design | [design-system/](design-system/README.md) | Generated visual tokens and assets |

The older standalone Android client under `primer-tasks/android/` is still part
of the Tasks build/test surface; it is not the Student/Control platform root.
The `spikes/` tree contains qualification evidence, not a production service.

**Presence is not acceptance.** This map describes checked-in implementation,
not a declaration that every roadmap phase, browser journey, deployment, or
physical-device gate is complete. Studio's [S17 browser guide](curriculum-studio/web/e2e/README.md)
records later credential-free browser proof than its original handoffs, but not
whole-phase/live acceptance. Android's [completion audit](agent_docs/plans/primer-android-platform/completion-audit.md)
also has device/native acceptance obligations. Older plan cursors and findings
must be reconciled against reachable source and exact-tip evidence before reuse.

## Technology and ownership

- **Backend:** Go, Huma/chi HTTP APIs, pgx/PostgreSQL, goose migrations.
- **Browser:** Vite, React, TypeScript, generated API clients; shared house tokens.
- **Android:** Kotlin/Compose, shared security/device/update libraries, Media3 TV playback.
- **Workstation:** Go/Bubble Tea TUI and a privileged broker, packaged with NixOS.
- **Agents:** pinned Microsoft Agent Framework Go in `primer-agents/runtime` and
  the LMS compatibility path; Fantasy in Tasks. Provider enablement is explicit
  and component-specific, not a single global Bedrock/OpenRouter default.
- **State:** PostgreSQL is authoritative for each service. The workstation keeps
  local cached/durable state; SQLite is not the central academic database.

LMS, TV, Studio, Identity, Agents, and Tasks own **separate databases**, even where
binaries share a Go module. They integrate through APIs/events, not cross-database
queries or foreign keys. Product-local authorization remains separate from
identity-provider claims. See the [current authentication matrix](agent_docs/authentication.md)
before configuring any credential or migration.

### LMS conventions

Huma derives the LMS/TV OpenAPI specifications from Go handler signatures via
`server/cmd/openapi-gen`; browser clients are regenerated from those specifications.
Generic CRUD/list machinery lives in `server/internal/api/crud.go` and
`server/internal/repo/list.go`, with pagination, `q`, whitelisted sorting, and
exact-match filters. Integration tests use PostgreSQL testcontainers and
transaction/savepoint helpers under `server/internal/testutil/`.

### Instructional time

The TV reporter (`server/internal/tv/primer`) sends finished educational/mixed
playback sessions to LMS instruction ingest and records exported log IDs in
`primer_reports`. `TV_PRIMER_BASE_URL` enables the reporter; the
`TV_PRIMER_SERVICE_TOKEN` must match LMS `SERVICE_TOKEN`. Both directions retain
idempotency: the TV ledger is unique per playback session and the LMS ingest is
unique on `(source, source_ref)`.

**Entertainment is never instructional time.** The LMS enforces that boundary
with its API enum and database constraint; the TV reporter also excludes it.
Parents inspect counted time in LMS Instruction Logs and exports in TV Primer
Reports. Reporting retries do not justify duplicate academic evidence.

## Development

Use the Go versions pinned in each module / `go.work`, Node and npm for the web
apps, and Docker for testcontainers. Android additionally requires JDK 17 and
its documented SDK/toolchain. Studio requires contract generation before a
clean-checkout module test; see its [guide](curriculum-studio/README.md).

Commands below run **from the repository root** unless stated otherwise.

```sh
# LMS/server module: includes TV, ingest, and workstation Go packages
make build
make test
make cover

# Local LMS backend (after creating/migrating a disposable development DB)
(cd server && go run ./cmd/primer-server)

# LMS / TV browser clients and builds (install each app's npm dependencies first)
make client                 # LMS spec + TS client
make web
make tv-client              # TV spec + TS client
make tv-web

# Individual components
make tv-build tv-test
make ingest-build
make student-build
make identity-build identity-test
make agents-build agents-test
make tasks-build tasks-test

# Studio: generate ignored client packages before build/test
make -C curriculum-studio clients-generate
make studio-build studio-test

# Android platform: separate from make tasks-android
(cd android && ./gradlew test assembleDebug)
```

`make test`, `make build`, and `make lint` operate on **`server/` only**.
`make all` is also not a workspace-wide acceptance gate. Use affected component
commands and their workflows in [.github/workflows/](.github/workflows/); no one
root command certifies the entire product.

| Coverage command | Package scope | Required floor |
| --- | --- | --- |
| `make cover` | `server/internal/...` | 85% |
| `make studio-cover` | `curriculum-studio/internal/...` | 85% |
| `make identity-cover` | `primer-identity/internal/...` | 80% |
| `make agents-cover` | `primer-agents/internal/...` | 85% |
| `make tasks-cover` | `primer-tasks/internal/...`, with same-source child-process collection | 85% |

These are **requirements, not current measurements**. [Makefile](Makefile) and
[scripts/enforce-module-cover.sh](scripts/enforce-module-cover.sh) implement the
gates. The [Agents coverage history](agent_docs/runbooks/coverage-blockers.md)
is not permission to lower its floor. Record measurement SHA, command, and result
rather than carrying percentages forward to a new head.

### Local services and generated clients

Host Make targets stay the default non-Compose workflow: `make dev-db` starts
local PostgreSQL, `make dev-db-tv` creates its TV database, and `make migrate` /
`make migrate-tv` apply the selected migrations. Check the destination DSN first;
never run these against a shared/production database by accident.

The opt-in [Compose stack](docs/dev-compose.md) is managed through
`scripts/compose-dev.sh` / `paseo.json`, not substituted for the host commands.
`make dev-db-studio` and `make dev-db-identity` still deliberately exit as
unsupported; use disposable component databases and their documented namespaced
DSNs. That does not imply their build, test, or migration targets are stubs.

Tasks has its own development commands (`make tasks-check`, `make tasks-up`,
`make tasks-endpoints`, `make tasks-down`) and host-stack proof targets in
[its scripts](primer-tasks/scripts/). These operate a separate local stack; they
are not production deployment commands.

Keep codegen policies component-specific. LMS/TV specs and Identity's spec/client
baselines are tracked; Studio's generated clients remain ignored. Never solve
missing Studio generated imports by checking the output into Git.

## Operations and design references

- **Workstation pairing, assignments, backup, rollout:**
  [student-client operations](agent_docs/runbooks/student-client-ops.md).
  Prototype retirement still requires recorded physical acceptance; stub/harness
  commands remain test-only.
- **Agents:** [service operations](agent_docs/runbooks/agents-operations.md),
  [LMS remote rollout](agent_docs/runbooks/phase7-cutover.md), and
  [local MAF preview](agent_docs/runbooks/maf-go-runtime.md). The remote and local
  flags are independent and default off; neither implies distributed model recovery.
- **Content ingest:** [YouTube storage, Collection, and recovery](agent_docs/runbooks/youtube-shows.md).
  `curriculum/content-manifest.yaml` is desired state; human title selections live
  in `curriculum/content-review.yaml`. Ingest stops at catalog population, not
  scheduling. Its `plan` command can write review/report/catalog state; do not
  assume it is an offline read-only operation.
- **Deployment:** [Nomad operations](deploy/nomad/README.md),
  [investor deployment](investor-web/DEPLOY.md), and [Android guide](android/README.md).
  Builds/tests are not authorization for live publication or device changes.
- **Educational intent:** [pedagogy](agent_docs/pedagogy.md),
  [target tutoring model](agent_docs/architecture.md),
  [curriculum](agent_docs/curriculum.md), [assessment](agent_docs/assessment.md),
  [injectors](agent_docs/injectors.md), and [projects](agent_docs/projects.md).

Historical plans retain requirements and design evidence, but old branch names,
`/tmp` receipts, and “next phase” text are not current runtime instructions.
Unrun live-provider and physical-device evidence must stay explicit.
