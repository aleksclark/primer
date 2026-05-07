# Virtual TV Channel

## Purpose

A curated, scheduled video stream that the student watches linearly — like broadcast television. This is a habit-formation tool, not entertainment.

### What It Teaches

- **Patience and sustained attention.** You watch what's on. You don't skip ahead.
- **Serendipitous learning.** Content you didn't choose but learn from anyway.
- **Media discipline.** Not everything is on-demand. You receive what's given.
- **Parental editorial control.** The family, not the algorithm, decides.

### Burke Connection

This is the "little platoons" principle applied to media consumption. The child doesn't choose his own media diet any more than he chooses his own moral framework at age 12. The parent curates; the child receives. As he matures and demonstrates good judgment, he earns more autonomy.

## Architecture

```
┌─────────────────────────────────────────┐
│           CHANNEL SCHEDULER             │
│                                         │
│  Content library (local files,          │
│  downloaded videos, streaming sources)  │
│                                         │
│  Schedule: time-slotted grid            │
│  Blocks: morning, afternoon, evening    │
└────────────────┬────────────────────────┘
                 │
                 ▼
┌─────────────────────────────────────────┐
│           PLAYBACK ENGINE               │
│                                         │
│  Plays current schedule item            │
│  No skip, no fast-forward, no rewind   │
│  "What's on now" display               │
│  Channel guide for today                │
└────────────────┬────────────────────────┘
                 │
                 ▼
┌─────────────────────────────────────────┐
│       CURRICULUM INTEGRATION            │
│                                         │
│  Tutor agent knows the schedule:        │
│  - Pre-teaches vocabulary               │
│  - Assigns post-viewing responses       │
│  - Connects to current standards        │
│  - Notes vocabulary encountered         │
└─────────────────────────────────────────┘
```

## Content Categories

| Category | Examples | Purpose |
|----------|----------|---------|
| Educational documentary | Nature docs, history docs, science shows | Direct content knowledge |
| Practical skills | Woodworking, cooking, farming/homesteading shows | Modeling craftsmanship and competence |
| Quality entertainment | Classic films, well-made series (parent-selected) | Cultural literacy, narrative appreciation, rest |
| Current events | Curated news segments (not 24-hour news) | Civic awareness, geography, critical thinking |
| Music & arts | Concert recordings, art technique videos | Aesthetic formation |

## Schedule Structure

### Sample Weekday

| Time | Block | Content | Duration |
|------|-------|---------|----------|
| 8:00 AM | Morning knowledge | Documentary segment | 25 min |
| 12:30 PM | Midday practical | Woodworking/cooking show | 30 min |
| 4:00 PM | Afternoon interest | Science/nature show | 25 min |
| 7:00 PM | Evening entertainment | Classic film or quality series episode | 45-90 min |

### Rules

- **No skipping.** If you miss the scheduled time, you miss it.
- **No fast-forward.** Watch at broadcast pace.
- **No binge.** One episode, then it's done for that slot.
- **Ads removed.** Content only.
- **Parent can override.** Family movie night replaces the schedule.

## Integration with Primer

The TV channel is not separate from the curriculum. The tutor agent accesses the schedule and can:

### Pre-Viewing

- Introduce vocabulary from the upcoming documentary
- Provide historical context for a period drama
- Set up a "watch for this" question

### Post-Viewing

- Ask the student to summarize what they learned
- Connect content to current standards ("That documentary mentioned erosion — explain how that connects to our geology unit")
- Assign a written response
- Use cooking show content to launch a fractions lesson (recipe scaling)
- Discuss ethical questions from a historical documentary

### Cross-Subject Connections

- Nature documentary about bridges → physics of forces → geometry of trusses
- Cooking show → fractions and ratios → scientific method (why does yeast work?)
- History documentary → primary source analysis → geography → economics

## Implementation

### Content Sources

- **Local files**: Downloaded/ripped videos (parent owns physical media)
- **YouTube-DL archive**: Educational channels archived locally
- **Streaming services**: If available, integrated via API or simple scheduling cue

### Technical Requirements

- Video player with disabled controls (no skip/ff)
- Schedule database (content → time slot mapping)
- "What's on now" API for TUI integration
- Parent-facing schedule editor
- Content library manager (add, categorize, tag)

### MVP

Start simple:
1. A directory of video files organized by category
2. A YAML schedule mapping time slots to files
3. A player script that plays the current slot's video with controls disabled
4. The tutor agent reads the schedule YAML to know what's coming

Complexity comes later (streaming integration, content recommendation for parent, automatic scheduling based on curriculum needs).
