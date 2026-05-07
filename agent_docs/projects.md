# Practical Projects Framework

## Design Principles

1. **Every project maps to multiple standards** across at least 2 subjects
2. **Projects are real** — the student builds something that exists in the physical world
3. **Documentation is mandatory** — every project produces written artifacts
4. **Math is applied** — measurements, calculations, and budgets serve real purposes
5. **Science is observed** — hypotheses are tested against actual outcomes
6. **The project IS the curriculum** — not enrichment, not extra credit, not a break from "real school"

## Project Schema

```yaml
practical_project:
  id: "proj-chicken-coop"
  name: "Chicken Coop Build"
  duration_weeks: 6
  grade: 6
  standards_covered:
    math:
      - "TN.MATH.6.G.A.1"        # area of polygons
      - "TN.MATH.6.RP.A.3"       # ratio reasoning
      - "TN.MATH.6.NS.B.3"       # decimal operations (budgeting)
    science:
      - "TN.SCI.6.LS2.1"         # organisms and environment
      - "TN.SCI.6.ETS1.2"        # evaluate design solutions
    ela:
      - "TN.ELA.6.W.TTP.2"       # informative/explanatory writing
      - "TN.ELA.6.W.PDW.4"       # clear, coherent writing
    social_studies:
      - "TN.SS.6.E.1"            # economic concepts (budgeting)
  phases:
    - name: "Research & Planning"
      weeks: 1
      activities:
        - "Research poultry breeds suited to Tennessee climate"
        - "Read county zoning regulations for backyard poultry"
        - "Draft a written proposal with materials list and budget"
      deliverables:
        - type: "written"
          description: "1-page proposal with breed selection rationale and cost estimate"
      injector_opportunities:
        - "Economic reasoning: cost-benefit of different breeds"
        - "Scientific method: criteria for breed selection"
        - "Citation: sources for climate/breed data"
    - name: "Design & Measurement"
      weeks: 1
      activities:
        - "Draw floor plan to scale on graph paper"
        - "Calculate square footage per bird (4 sq ft minimum)"
        - "Design in Onshape (3D model)"
        - "Compute materials from dimensions"
      deliverables:
        - type: "artifact"
          description: "Scale drawing with dimensions + Onshape model"
        - type: "written"
          description: "Materials list with quantities derived from calculations"
      injector_opportunities:
        - "Geometry: area, scale factor, 3D visualization"
        - "Proportional reasoning: scale drawings, material ratios"
        - "Digital literacy: parametric CAD as applied geometry"
    - name: "Construction"
      weeks: 3
      activities:
        - "Frame the structure (measuring, cutting, joining)"
        - "Install hardware cloth and roofing"
        - "Build nesting boxes and roost bars"
        - "Install door with hinges and latch"
      deliverables:
        - type: "artifact"
          description: "Completed coop, photo-documented at each stage"
        - type: "written"
          description: "Daily construction journal with measurements and decisions"
      injector_opportunities:
        - "Geometry: perpendicular cuts, angle measurement"
        - "Physics: load distribution, structural integrity"
        - "Numerical precision: every measurement documented with units"
    - name: "Documentation & Analysis"
      weeks: 1
      activities:
        - "Write construction journal with daily entries"
        - "Calculate actual vs. estimated costs (% error)"
        - "Photograph and label all biological features"
        - "Present completed project to parent with oral narration"
      deliverables:
        - type: "written"
          description: "Complete construction journal, cost analysis, photo documentation"
      injector_opportunities:
        - "Scientific method: prediction vs. outcome analysis"
        - "Proportional reasoning: percent error calculation"
        - "Clear reasoning: explaining design decisions"
  assessment:
    mastery_demo: "Student can explain design decisions using math and science"
    portfolio_piece: true
    standards_assessed_via:
      - standard: "TN.MATH.6.G.A.1"
        evidence: "Correct area calculations in design phase"
      - standard: "TN.ELA.6.W.TTP.2"
        evidence: "Construction journal quality"
      - standard: "TN.SCI.6.ETS1.2"
        evidence: "Oral presentation explaining design trade-offs"
```

## Sample Year Plan (Grade 6)

| Quarter | Project | Primary Standards | Key Skills | Injector-Rich Moments |
|---------|---------|-------------------|------------|----------------------|
| Q1 | **Garden Plot** — Design, plant, maintain a 4×8 raised bed | Math: area, perimeter, ratios. Science: botany, soil. ELA: research, journaling | Carpentry, soil prep, seed starting | Geometry (bed layout), proportional reasoning (spacing, soil mix ratios), scientific method (growth predictions) |
| Q2 | **Chicken Coop** — Design and build a 4-bird coop | Math: geometry, budgeting. Science: animal biology. ELA: technical writing | Framing, measuring, hardware cloth | Geometry (Onshape design), physics (structural), economic reasoning (budget), digital literacy (CAD) |
| Q3 | **Weather Station** — Build and operate a home weather station | Math: statistics, graphing. Science: meteorology, data collection. ELA: report writing | Sensor reading, data recording, spreadsheets | Scientific method (predictions), digital literacy (spreadsheets as algebra), proportional reasoning (unit conversion) |
| Q4 | **Family Cookbook** — Research, test, document 20 family recipes | Math: fractions, unit conversion, scaling. Science: food science. ELA: procedural writing | Cooking, food safety, layout/typography | Proportional reasoning (scaling), scientific method (why bread rises), historical context (recipe origins), economic reasoning (cost per serving) |

## Integration with the System

### How Projects Generate Academic Work

Each active project generates:
- **Math problems** drawn from real measurements and calculations
- **Science investigations** using real observations and data
- **Writing assignments** documenting the process (journals, proposals, analyses)
- **Reading** related to the project domain (agriculture manuals, biology texts, etc.)
- **Vocabulary** naturally acquired through the work
- **Historical context** where relevant

The tutor agents know which project is active and weave it into lessons:
- Math tutor uses coop dimensions for geometry problems
- Science tutor uses garden observations for experiment design
- ELA tutor uses project journal for writing instruction

### Off-Screen Time

Significant project time happens AWAY from the TUI:
- Actual construction, planting, cooking
- Physical measurement and observation
- Photo documentation (returned to TUI for discussion)
- Reading physical books and manuals

The TUI plans, assesses, and discusses. The student DOES.

### Project State Tracking

```yaml
project_state:
  id: "proj-chicken-coop"
  status: "in_progress"          # planned | in_progress | completed
  current_phase: "Construction"
  phase_progress:
    "Research & Planning": "completed"
    "Design & Measurement": "completed"
    "Construction": "in_progress"
    "Documentation & Analysis": "pending"
  deliverables_completed:
    - "1-page proposal"
    - "Scale drawing"
    - "Onshape model"
    - "Materials list"
  standards_demonstrated: ["TN.MATH.6.G.A.1", "TN.ELA.6.W.TTP.2"]
  journal_entries: 12
  photos: 8
  budget_actual: 247.50
  budget_estimated: 220.00
```
