# Architecture — System Design

## Agent Topology

### Session Overseer

The top-level coordinator. Responsibilities:
- Plan the day's session based on mastery state, scope & sequence, and parent overrides
- Route student interactions to the appropriate specialist tutor
- Handle transitions between subjects
- Escalate to parent when needed (student stuck, behavioral issues, schedule questions)
- Ensure coverage across all domains over time

The overseer does NOT teach — it orchestrates.

### Specialist Tutors

One agent per subject domain, each with a deep, focused system prompt:

| Tutor | Domain | Key Capabilities |
|-------|--------|-----------------|
| Math | Arithmetic, algebra, geometry, statistics | Problem generation, step-by-step coaching, multiple solution paths |
| ELA | Reading, writing, grammar, vocabulary | Text analysis, writing feedback, Socratic questioning about readings |
| Science | Life, Earth, Physical science | Experiment design, observation coaching, data interpretation |
| Social Studies | History, geography, civics, economics | Primary source analysis, map work, constitutional reasoning |
| Practical | Construction, gardening, cooking, electronics | Project planning, measurement coaching, safety guidance |

Each tutor knows:
- The relevant Tennessee Academic Standards
- The student's current mastery level in their domain
- Common misconceptions for each topic
- How to scaffold (not just give answers)

### Cross-Cutting Injectors

See [injectors.md](injectors.md) for full details. Two types:
- **Guards**: Block progression until baseline standards are met
- **Reinforcement Seekers**: Surface connections to other domains

Injectors observe ALL student interactions regardless of which tutor is active.

## Data Flow

```
┌─────────────────────────────────────────────────────────┐
│                    STUDENT INPUT                         │
│  (text response, Onshape activity, project update, etc.)│
└───────────────────────────┬─────────────────────────────┘
                            │
              ┌─────────────▼─────────────┐
              │      GUARD INJECTORS      │
              │                           │
              │  Grammar? Units? Reasoning?│
              │  If fail → return to      │
              │  student for correction   │
              └─────────────┬─────────────┘
                            │ (passes)
         ┌──────────────────┼──────────────────┐
         │                  │                  │
         ▼                  ▼                  ▼
┌─────────────┐  ┌──────────────┐  ┌──────────────────┐
│  SUBJECT    │  │ REINFORCEMENT│  │    MASTERY       │
│  TUTOR      │  │   SEEKERS    │  │    TRACKER       │
│             │  │              │  │                  │
│ Teaches the │  │ "That's the  │  │ Records evidence │
│ current     │  │  same as..." │  │ of understanding │
│ topic       │  │              │  │                  │
└─────────────┘  └──────────────┘  └──────────────────┘
         │                  │                  │
         └──────────────────┼──────────────────┘
                            │
              ┌─────────────▼─────────────┐
              │     SESSION OVERSEER      │
              │                           │
              │  Next topic? Switch tutor? │
              │  Reinforcement check?      │
              │  End session?              │
              └───────────────────────────┘
```

## Session Management

A "session" is one sitting at the TUI — typically 1-3 hours.

### Session Planning

The overseer consults:
1. **Mastery state** — What standards are in progress? What's been mastered and needs reinforcement?
2. **Scope & sequence** — What's the expected progression for this point in the year?
3. **TCAP readiness** — Any gaps needing attention before the testing window?
4. **Parent overrides** — Parent can pin specific topics or projects
5. **Practical project phase** — If a build project is active, integrate it

### Context Persistence

Between sessions:
- Conversation history per tutor (recent context window)
- Mastery state per standard (permanent)
- Active project state (phase, deliverables completed)
- Reinforcement schedule (spaced repetition at the skill level)
- Cross-cutting injector hit log (what was reinforced, when)

### External Tool Sessions

When the student uses Onshape, spreadsheets, or other GUI tools:
1. The TUI launches the tool and provides instructions
2. The student works in the external tool
3. The student returns to the TUI and describes/documents what they did
4. The injector engine processes the activity for reinforcement opportunities
5. The tutor evaluates the work against project requirements

Future: direct integration via Onshape API for observing student actions in real-time.

## Technology Choices

### Why Go

- The command_teacher prototype is already in Go
- Charm/Bubble Tea ecosystem for TUI
- Fantasy SDK for agent orchestration
- Strong concurrency for parallel injector evaluation
- Single binary deployment

### Why TUI

- Forces abstract thought (text, not pictures)
- Builds typing skill through volume
- No distractions (tabs, notifications)
- Fast iteration (keyboard > mouse)
- Works over SSH if needed

### Why Not Web

- Browser is a distraction machine
- Visual UI encourages clicking over thinking
- Adds massive complexity for marginal benefit
- The student should spend significant time AWAY from screens

### Data Storage

- **Standards definitions**: YAML files (version controlled, human-editable)
- **Student state**: SQLite (fast, local, no server dependency)
- **Conversation history**: SQLite (searchable, compact)
- **Project artifacts**: Filesystem (photos, documents, drawings)
- **Configuration**: YAML (parent-editable)

## Relationship to command_teacher

The existing `command_teacher` prototype (`~/work/command_teacher`) demonstrates:
- Go + Charm TUI with three panes (terminal, instructions, chat)
- LLM instructor agent with tools (create files, set instructions, log progress)
- Sandboxed environment via bubblewrap/proot
- Student progress as markdown with session logs
- Bedrock as default LLM provider

Primer extends this from "Unix commands" to "all of middle school" by:
- Adding specialist agents (not just one instructor)
- Adding the injector layer
- Adding curriculum store with standards alignment
- Adding mastery tracking with reinforcement scheduling
- Adding external tool integration
- Adding assessment generation
