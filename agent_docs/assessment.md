# Assessment — Mastery Tracking, TCAP Prep, Portfolio

## Three Simultaneous Layers

| Layer | What | When | Purpose |
|-------|------|------|---------|
| **Continuous** | Guard injectors + tutor evaluation of every response | Every interaction | Real-time quality enforcement, immediate feedback |
| **Periodic Formal** | Generated quizzes, TCAP-format practice tests | Weekly short / monthly comprehensive | Benchmark mastery, test-format familiarity |
| **Portfolio** | Written work, project documentation, photos of builds | Ongoing, monthly review | Depth of understanding, authentic evidence |

## Continuous Assessment

The most important layer. Every student interaction is assessed:

- **Guard injectors** evaluate grammar, reasoning, precision, citation, vocabulary — constantly
- **Subject tutors** evaluate understanding of the current topic through conversation
- **Reinforcement seekers** confirm cross-domain connections when surfaced

This means assessment is not a separate activity. The student is always demonstrating (or failing to demonstrate) understanding.

### Mastery State Per Standard

```yaml
mastery_record:
  standard_id: "TN.MATH.6.RP.A.3"
  ccss_equivalent: "6.RP.A.3"
  status: "mastered"              # not_introduced | in_progress | approaching | mastered
  confidence: 0.92                # 0.0-1.0, decays over time without reinforcement
  last_assessed: "2026-10-15"
  last_reinforced: "2026-11-02"
  next_reinforcement: "2026-11-16"
  evidence:
    - type: "continuous"
      date: "2026-10-15"
      context: "Solved recipe scaling problems correctly 5/5"
    - type: "formal"
      date: "2026-10-12"
      context: "Quiz: 92% on ratio/proportion items"
    - type: "project"
      date: "2026-09-20"
      context: "Correctly calculated material ratios for garden bed soil mix"
  tcap_weight: "high"
  decay_rate: 0.02                # confidence drops per day without use
```

### Status Transitions

```
not_introduced → in_progress    (first lesson on this standard)
in_progress → approaching       (demonstrates partial understanding)
approaching → mastered          (meets all mastery criteria consistently)
mastered → approaching          (reinforcement check shows decay)
approaching → mastered          (re-demonstrates after remediation)
```

## Periodic Formal Assessment

### Weekly Quick Checks (15-20 min)

- 5-10 items covering standards worked on that week
- Mix of item types matching TCAP format
- Generated fresh each time (no repeated items)
- Immediate feedback and remediation path

### Monthly Comprehensive (45-60 min)

- Covers all standards at "mastered" or "approaching" status
- Includes items from earlier in the year (spiral review)
- TCAP format: MC, multi-select, equation editor, constructed response, editing tasks
- Scores reported per standard, not overall percentage

### TCAP Practice Tests (Quarterly)

- Full-length TCAP-format assessment
- Timed to match actual testing conditions
- Scored against TCAP performance levels (Below → Approaching → On-Track → Mastered)
- Results inform the session planner's TCAP readiness view

### Item Generation

The system generates assessment items dynamically:

```yaml
generated_item:
  standard: "TN.MATH.6.RP.A.3"
  item_type: "multi_select"       # mc | multi_select | equation_editor | constructed_response | matching
  difficulty: "on_track"          # approaching | on_track | mastered
  context: "practical"            # abstract | practical | word_problem
  stem: "A recipe calls for 2 cups flour for every 3 cups sugar. Which statements are true?"
  options:
    - text: "The ratio of flour to sugar is 2:3"
      correct: true
    - text: "For 6 cups sugar, you need 4 cups flour"
      correct: true
    - text: "The ratio of sugar to flour is 2:3"
      correct: false
    - text: "For 9 cups sugar, you need 6 cups flour"
      correct: true
  rationale: "Tests understanding of ratio as relationship, and ability to scale proportionally"
```

## Portfolio

### What Goes In

- **Written work**: Proposals, journals, reports, essays, analyses
- **Project documentation**: Plans, scale drawings, photos at each phase, cost analyses
- **Reading journals**: Responses to books, annotations, summaries
- **Best formal assessments**: Highest-scoring practice tests
- **Creative work**: Cookbook entries, presentations, diagrams

### Monthly Portfolio Review

Parent and student review together:
- What was accomplished this month?
- What shows growth from last month?
- What are the strongest pieces? Why?
- Where is improvement needed?

This is not graded — it's reflective. The student practices articulating his own growth.

### Portfolio as TCAP Evidence

If TCAP scores don't reflect the student's actual ability (testing anxiety, bad day), the portfolio provides supplementary evidence of learning.

## TCAP Preparation Strategy

TCAP prep is integrated, not crammed:

### Continuous (All Year)

- Every lesson aligned to tested standards
- Mastery tracking ensures coverage
- Guard injectors enforce the quality expected on TCAP (complete sentences, shown work, cited evidence)

### Format Familiarity (Monthly)

- TCAP-format items in monthly assessments
- Student learns the item types without anxiety
- Editing tasks (ELA), equation editor (Math), constructed response (all)

### Spring Intensive (6 Weeks Before Testing Window)

- Session planner shifts toward gap-closing based on mastery data
- Focus on standards at "approaching" that could reach "mastered"
- Increased formal assessment frequency
- Test-taking skills explicitly taught: time management, process of elimination, re-reading, checking work

### Test-Taking Skills

Taught explicitly as a meta-skill:
- **Time management**: Pace yourself, don't spend too long on one item
- **Process of elimination**: Remove obviously wrong answers first
- **Re-read the passage**: Answer is in the text, go back and find it
- **Check reasonableness**: Does this math answer make sense in context?
- **Constructed response structure**: Claim → Evidence → Reasoning
