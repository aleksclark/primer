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
