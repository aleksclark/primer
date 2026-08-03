# Primer — AI-Powered Homeschool Tutoring System

## Overview

Primer is an AI tutoring system for middle school homeschooling (grades 6-8). It provides mastery-based, standards-aligned instruction through a TUI-primary interface with specialist AI tutor agents, cross-cutting skill reinforcement, and deep integration with hands-on practical projects.

The system is named after the Young Lady's Illustrated Primer from Neal Stephenson's *The Diamond Age* — an adaptive, comprehensive, deeply personal educational device. Unlike the novel's primer, this one is grounded in a specific pedagogical tradition: Burkean conservatism, virtue formation through habit and discipline, and the primacy of the family as educator.

## Core Principles

1. **The parent is the primary educator.** The AI is a tool in the parent's hands — infinitely patient, adaptive, and tireless — but the parent sets direction, curates content, and provides irreplaceable human relationship.
2. **Virtue is formed through habit, not instruction.** The system doesn't teach "character" as a subject — it forms character through the *way* every subject is taught: requiring precision, enforcing standards, demanding craftsmanship.
3. **Every activity is a learning surface.** The cross-cutting injector system recognizes when any activity — CAD design, cooking, construction, terminal usage — touches standards from other domains and requires the student to articulate the connection.
4. **Mastery, not coverage.** The student advances when he demonstrates understanding, not on a calendar. Mastered skills stay in active use through spiral reinforcement.
5. **TUI-primary for a reason.** Text-based interaction forces abstract thought and builds typing skill. GUI tools (Onshape, spreadsheets) are escape hatches for work that requires them, not the default.

## Architecture

```
┌────────────────────────────────────────────────────────────┐
│                      SESSION OVERSEER                       │
│  Plans the day, routes to specialists, tracks big picture   │
└────┬──────────┬──────────┬──────────┬──────────────────────┘
     │          │          │          │
     ▼          ▼          ▼          ▼
┌────────┐ ┌────────┐ ┌────────┐ ┌────────┐
│  Math  │ │  ELA   │ │Science │ │  Soc.  │  ...
│ Tutor  │ │ Tutor  │ │ Tutor  │ │Studies │
└───┬────┘ └───┬────┘ └───┬────┘ └───┬────┘
    └──────────┴──────────┴──────────┘
               │
    ┌──────────▼──────────────────────┐
    │    CROSS-CUTTING INJECTORS      │
    │                                 │
    │  Guards:                        │
    │  • Grammar/Mechanics            │
    │  • Clear Reasoning              │
    │  • Numerical Precision          │
    │  • Citation & Evidence          │
    │                                 │
    │  Reinforcement Seekers:         │
    │  • Geometry (Onshape, building) │
    │  • Proportional Reasoning       │
    │  • Scientific Method            │
    │  • Economic Reasoning           │
    │  • Physics/Engineering          │
    │  • Digital Literacy             │
    │  • Historical Context           │
    └─────────────────────────────────┘
```

## Tech Stack

- **Language**: Go (extending the command_teacher prototype)
- **UI Framework**: Charm stack (Bubble Tea v2, Lip Gloss v2)
- **Agent Framework**: Fantasy SDK (Charmbracelet)
- **LLM Provider**: AWS Bedrock (primary), OpenRouter/OpenAI (fallback)
- **Curriculum Store**: YAML/JSON standards definitions, SQLite for student state
- **External Tools**: Onshape (CAD), spreadsheets, document generation (markdown → PDF)

## Project Structure

```
primer/
├── AGENTS.md              ← you are here
├── agent_docs/            ← detailed documentation by topic
│   ├── pedagogy.md        ← philosophical foundations and educational approach
│   ├── architecture.md    ← system architecture and agent design
│   ├── curriculum.md      ← standards, subjects, scope & sequence
│   ├── injectors.md       ← cross-cutting injector system design
│   ├── projects.md        ← practical projects framework
│   ├── assessment.md      ← assessment model and mastery tracking
│   ├── tv-channel.md      ← virtual TV channel system
│   └── tools.md           ← external tool integration (Onshape, etc.)
├── cmd/
│   └── primer/
│       └── main.go        ← entry point
├── internal/
│   ├── overseer/          ← session overseer agent
│   ├── tutor/             ← specialist tutor agents
│   ├── injector/          ← cross-cutting injector engine
│   ├── curriculum/        ← curriculum store and standards
│   ├── student/           ← student state and mastery tracking
│   ├── assessment/        ← assessment generation and evaluation
│   └── tui/               ← terminal UI components
├── curriculum/            ← curriculum data files
│   ├── standards/         ← TN + CCSS standard definitions (YAML)
│   ├── reading-lists/     ← curated book lists by grade/subject
│   └── projects/          ← practical project definitions
├── go.mod
└── go.sum
```

## Development Environment

```bash
# Build
go build ./cmd/primer/

# Run
go run ./cmd/primer/

# Test
go test ./...
```

## LMS Server (`server/`) and Admin SPA (`web/`)

The LMS backend is a Go HTTP API (Huma v2 + chi + pgx/PostgreSQL) that manages
students, curricula, standards, mastery tracking, and assessments. The admin
SPA is a Vite + React + Tailwind (shadcn-style) app with a generated TypeScript
client.

```bash
make build     # build server binaries
make test      # full test suite (integration tests use a PostgreSQL testcontainer)
make cover     # coverage with 85% minimum gate
make openapi   # regenerate web/openapi.yaml from API type signatures
make client    # regenerate the TS client (openapi-typescript)
make web       # build the SPA (regenerates client first)
make dev-db    # start a local PostgreSQL in docker
make migrate   # apply migrations (goose, embedded in the binary)
```

Key architecture points:

- **OpenAPI from code**: Huma generates the OpenAPI 3.1 spec from Go handler
  type signatures (`server/cmd/openapi-gen`). The TS client is generated from
  that spec at build time (`npm run generate:client`).
- **Generic CRUD+list**: every resource gets list/get/create/update/delete via
  `api.RegisterCRUD` with pagination, free-text search (`q`), whitelisted
  sorting (`sort`/`dir`), and exact-match filters (`filter=col:value`) built in
  (`server/internal/repo/list.go`, `server/internal/api/crud.go`).
- **Integration testing**: tests run against a real PostgreSQL testcontainer;
  each test gets a transaction (rolled back on cleanup) wrapped in per-statement
  savepoints so expected constraint violations don't abort it
  (`server/internal/testutil/`). FactoryBot-style factories live in
  `server/internal/testutil/factory/`.
- **Schema**: educators, students, subjects, standards (hierarchical,
  multi-source, prerequisite graph), curricula (multiple approaches),
  curriculum_standards (sequencing), enrollments, mastery_records + evidence,
  assessments (6 kinds), items, options, attempts, item_responses,
  instruction_logs (instructional time reported from outside the LMS).
  Migrations in `server/internal/db/migrations/`.

## TV Server (`server/cmd/tv-server`) and TV Admin SPA (`tv-web/`)

The virtual TV channel runs as a second binary against a **separate PostgreSQL
database**, reusing the same Huma/chi/pgx stack, the same generic CRUD and list
machinery (`internal/repo`, `internal/api`), and the same testcontainer harness.
See [the implementation plan](agent_docs/plans/video-as-instruction.md) for the
full design.

```bash
make tv-build    # build the TV server binary
make tv-test     # TV test suite
make openapi-tv  # regenerate tv-web/openapi.yaml
make tv-client   # regenerate the TV TS client
make tv-web      # build the TV admin SPA
make dev-db-tv   # create the TV database in the local PostgreSQL
make migrate-tv  # apply TV migrations
```

Configuration is read from the environment with a `TV_` prefix
(`server/internal/tv/config`) so both binaries can run side by side.

## Content ingest (`server/cmd/content-ingest`)

Manifest-driven reconciler that converges Radarr/Sonarr/yt-dlp, Jellyfin, and the
TV server toward `curriculum/content-manifest.yaml`. No LLM in the loop; ambiguous
title matches land in `curriculum/content-review.yaml` for a human pick. See
[content-ingest plan](agent_docs/plans/content-ingest.md).

```bash
make ingest-build
make ingest-plan    # diff + review candidates + report
make ingest-apply   # resolve → acquire → sync → import → report
```

Config uses the `INGEST_` env prefix. Scheduling stays out of scope — this tool
stops at classified `media_items` in the TV server.

### Service boundary and credentials

The two services never share a database. They talk over HTTP, and each
direction carries its own shared secret, so a credential that leaks in one
direction cannot be replayed in the other:

| Direction | Credential | Header | Purpose |
|-----------|-----------|--------|---------|
| TV → LMS | `SERVICE_TOKEN` (on the LMS) as `TV_PRIMER_SERVICE_TOKEN` (on the TV server) | `X-Service-Token` | Push finished educational viewings to `POST /instruction-logs/ingest` |
| LMS → TV | `TV_ADMIN_API_KEY` | `X-Admin-Key` | Read the grid, place availability windows, programme slots — the same key the TV admin SPA uses |
| Device → TV | Per-device token from `POST /devices/pair` | `Authorization: Bearer` | Catalog, `/now`, play grants, heartbeats |

Every shared-secret guard is **inert when its secret is unset**
(`api.SharedSecretGuard`). That keeps OpenAPI generation and a bare local
checkout working without ceremony; both binaries log a warning at startup when a
secret is missing, so an unguarded deployment is loud rather than silent.

### Instructional time

Watching a tagged programme becomes instructional time in the LMS:

- The TV reporter (`server/internal/tv/primer`) polls for finished
  `playback_sessions` of `educational`/`mixed` media items that have no
  `primer_reports` row, posts each to the LMS ingest, and records the LMS's log
  ID in that ledger. Configured by `TV_PRIMER_BASE_URL`,
  `TV_PRIMER_SERVICE_TOKEN`, `TV_PRIMER_REPORT_INTERVAL`,
  `TV_PRIMER_REPORT_BATCH_SIZE`, `TV_PRIMER_TIMEOUT`; with no base URL set the
  reporter is not started at all.
- **Entertainment is never instructional time.** The LMS refuses it (enum plus a
  table `CHECK`) because the definition belongs to the system of record; the TV
  reporter also excludes it from its candidate query so those sessions never
  enter the backlog.
- **Reporting is at-least-once and idempotent on both sides**: the TV ledger is
  unique per playback session, and the LMS ingest is unique on
  `(source, source_ref)`. Retrying is always safe; an LMS that is down leaves
  sessions queued rather than losing them.
- The parent sees the result in two places: **Instruction Logs** in the LMS SPA
  (what was counted, by day and subject) and **Primer Reports** in the TV SPA
  (what was exported, with a manual "report now" pass).

## Key Design Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Narrative | None | Real projects provide context; fiction risks feeling forced |
| Agent arch | Specialists + overseer | Deep domain prompts, clean separation |
| Platform | TUI-primary | Forces abstract thought, builds typing |
| Sequencing | Mastery-based + reinforcement | No gaps; mastered skills stay active |
| Assessment | Continuous + formal + portfolio | Three layers simultaneously |
| Standards | TN primary, CCSS secondary | TCAP is the accountability measure |
| Content | Curated + generated assessment | Parent controls diet; AI generates practice |

## Detailed Documentation

See `agent_docs/` for comprehensive documentation on each aspect of the system:

- **[Pedagogy](agent_docs/pedagogy.md)** — Burke, virtue formation, what this is and isn't
- **[Architecture](agent_docs/architecture.md)** — Agent system design, data flow, session management
- **[Curriculum](agent_docs/curriculum.md)** — Standards alignment, subjects, sequencing
- **[Injectors](agent_docs/injectors.md)** — Cross-cutting reinforcement system
- **[Projects](agent_docs/projects.md)** — Hands-on practical project framework
- **[Assessment](agent_docs/assessment.md)** — Mastery tracking, TCAP prep, portfolio
- **[TV Channel](agent_docs/tv-channel.md)** — Virtual linear broadcast system
- **[Tools](agent_docs/tools.md)** — External tool integration (Onshape, spreadsheets, etc.)
- **[Code improvements](agent_docs/code-improvements.md)** — DRY / SoC / naming backlog for the LMS stack
- **[Video as instruction](agent_docs/plans/video-as-instruction.md)** — TV server, Android client, and Primer integration plan
