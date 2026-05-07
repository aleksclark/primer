# Cross-Cutting Injectors

## Concept

Injectors are the general mechanism for reinforcing learning across domain boundaries. They observe every student interaction — text output, tool activity, project work, conversation — and operate in two modes:

1. **Guards** block progression until baseline standards are met
2. **Reinforcement Seekers** surface connections to other domains without blocking

The key insight: every activity is a potential reinforcement surface. Onshape isn't just "CAD" — it's geometry class happening through a real tool. Cooking isn't just "life skills" — it's proportional reasoning with real stakes.

## Guards

Guards enforce non-negotiable quality standards. If a guard fires, the student must correct the issue before the subject tutor evaluates the content.

### Grammar & Mechanics Guard

- **Scope**: All student-produced text
- **Checks**: Spelling, punctuation, subject-verb agreement, complete sentences
- **Standards**: TN.ELA.6.L.CSE.1, TN.ELA.6.L.CSE.2
- **Example**: Student writes "their are 5 chickens" in a math answer → corrected before math is evaluated
- **Philosophy**: Writing is not a separate subject. It is a skill practiced hundreds of times daily across every domain.

### Clear Reasoning Guard

- **Scope**: All answers and explanations
- **Checks**: Answer explains *why*, not just *what*. Shows work. Identifies assumptions.
- **Example**: "42" is not an acceptable math answer. "42 sq ft because 6 × 7 = 42, and the bed is 6 feet long by 7 feet wide" is.
- **Philosophy**: Unexplained answers are worthless. The reasoning IS the learning.

### Numerical Precision Guard

- **Scope**: Math, Science, Practical Projects
- **Checks**: Units always present. Estimates before calculations. Reasonableness check. Appropriate significant figures.
- **Standards**: TN.MATH.6.RP.A.3, TN.SCI engineering practices
- **Example**: "The board is 6" → "6 what? Inches? Feet? Meters?"
- **Philosophy**: Naked numbers are meaningless. Real-world quantities always have units.

### Citation & Evidence Guard

- **Scope**: ELA, Social Studies, Science
- **Checks**: Claims backed by specific sources — the book, the measurement, the document, the observation
- **Example**: "It was hot that year" → "According to what source? What data?"
- **Philosophy**: Unsupported claims are not knowledge. Evidence is non-negotiable.

### Vocabulary Precision Guard

- **Scope**: All domains
- **Checks**: Uses domain-appropriate terms when known. Defines new terms in context.
- **Example**: "the plant food thing" → must use "photosynthesis" if the term has been taught
- **Philosophy**: Precise language enables precise thought.

## Reinforcement Seekers

Seekers don't block. They recognize when the current activity connects to standards from another domain and ask the student to articulate the connection. This turns every activity into multi-domain learning.

### Geometry Reinforcement Seeker

**Watches for**: Spatial/dimensional concepts in any context

| Context | Trigger | Connection Surfaced |
|---------|---------|-------------------|
| Onshape CAD | Perpendicular/parallel constraints | "What does perpendicular mean formally? How do you prove lines are perpendicular?" |
| Onshape CAD | Fillet/chamfer radius | "That radius is an arc. What fraction of a circle? How would you calculate the arc length?" |
| Onshape CAD | Extrude operation | "You just created a 3D solid. Volume = base area × height. Calculate it." |
| Construction | Framing angles | "What angle is that joint? How do you measure it? What's complementary/supplementary?" |
| Construction | Roof pitch | "Pitch is rise over run — that's slope. Write it as a fraction and a decimal." |
| Garden layout | Bed dimensions | "Calculate the area. Now calculate how much soil you need at 6 inches deep — that's volume." |

### Proportional Reasoning Seeker

**Watches for**: Quantities with proportional relationships

| Context | Trigger | Connection Surfaced |
|---------|---------|-------------------|
| Cooking | Scaling a recipe | "You're making 1.5x. Write out the fraction multiplication for each ingredient." |
| Cooking | Cost per serving | "If the whole recipe costs $12 and serves 6, what's the unit rate?" |
| Construction | Scale drawings | "The drawing is 1:12 scale. If the wall is 3 inches on paper, how tall is it really?" |
| Construction | Material estimation | "You need 4 boards per 8 feet of fence. How many for 32 feet? Write the proportion." |
| Science | Concentration | "5 mL fertilizer per gallon. You have 3 gallons. Set up the proportion." |

### Scientific Method Seeker

**Watches for**: Claims, predictions, observations in any domain

| Context | Trigger | Connection Surfaced |
|---------|---------|-------------------|
| Garden | "I think this soil is better" | "That's a hypothesis. How would you test it? What's your control? What do you measure?" |
| Garden | Recording growth | "You're collecting data. What type of graph shows change over time?" |
| Construction | "This joint is stronger" | "How would you test that claim? What variables would you control?" |
| Cooking | "It tastes better with more salt" | "That's subjective. How would you design a blind taste test?" |

### Economic Reasoning Seeker

**Watches for**: Cost/benefit, scarcity, trade-offs

| Context | Trigger | Connection Surfaced |
|---------|---------|-------------------|
| Projects | Budget decisions | "You chose the cheaper wood. What's the trade-off? Does it last as long?" |
| Projects | Time allocation | "Spending 2 hours sanding vs. 30 minutes. What's the opportunity cost?" |
| Cooking | Ingredient choices | "Organic costs 2x. Is the benefit proportional to the cost?" |

### Physics/Engineering Seeker

**Watches for**: Hands-on building that touches physical science

| Context | Trigger | Connection Surfaced |
|---------|---------|-------------------|
| Wiring | Weather station sensors | "Voltage, current, resistance. State Ohm's law. Calculate the current." |
| Construction | Load-bearing design | "Where are the forces? What happens if you remove this support?" |
| Garden | Water flow/irrigation | "Gravity pulls water downhill. What's the slope needed for flow?" |

### Historical Context Seeker

**Watches for**: Current activities with historical roots

| Context | Trigger | Connection Surfaced |
|---------|---------|-------------------|
| Construction | Building techniques | "Timber framing is ancient. How did medieval builders solve this without power tools?" |
| Garden | Crop selection | "Tennessee's agricultural history — what was grown here 200 years ago? Why?" |
| Cooking | Recipe origins | "Where does this dish come from? What does it tell us about that culture?" |

### Digital Literacy Seeker

**Watches for**: Tool usage that touches CS/math concepts

| Context | Trigger | Connection Surfaced |
|---------|---------|-------------------|
| Spreadsheet | Writing a formula | "That formula is an algebraic expression. Write it in mathematical notation." |
| Terminal | Using pipes/filters | "You just composed two functions. What's the mathematical term for that?" |
| Onshape | Parametric constraints | "You made the height depend on the width. That's a function. What's the domain?" |

## Implementation Notes

### Injector Interface

```go
type Injector interface {
    // Type returns "guard" or "reinforcement"
    Type() InjectorType
    
    // Evaluate checks the student's activity against this injector's domain
    // Returns nil if no action needed
    Evaluate(ctx context.Context, activity StudentActivity) (*InjectorResult, error)
    
    // StandardsLinked returns the standards this injector reinforces
    StandardsLinked() []StandardID
}

type InjectorResult struct {
    Type        InjectorType    // guard | reinforcement
    Message     string          // what to say to the student
    Standard    StandardID      // which standard is being reinforced
    Blocking    bool            // if true, student must respond before proceeding
    Prompt      string          // the question/correction to pose
}

type StudentActivity struct {
    Type        ActivityType    // text_response | tool_action | project_update | conversation
    Content     string          // the text or description of action
    Tool        string          // "onshape" | "spreadsheet" | "terminal" | ""
    Context     string          // what subject/project is active
    Metadata    map[string]any  // tool-specific data
}
```

### Frequency Control

Reinforcement seekers should not fire on every possible opportunity — that would be exhausting. Controls:

- **Cooldown per seeker**: Don't fire the same seeker more than once per 30 minutes
- **Max reinforcements per session**: Cap at 5-8 per 2-hour session
- **Priority**: Fire on the strongest connections, not every tenuous one
- **Student mastery context**: Don't reinforce standards already at "Mastered" unless in decay check
- **Novelty preference**: Prefer new connections over ones already surfaced this week
