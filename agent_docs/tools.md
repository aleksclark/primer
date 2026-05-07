# External Tool Integration

## Philosophy

The TUI is the primary learning conversation. External tools are launched when the work genuinely requires them, then the student returns to the TUI to discuss, document, and demonstrate understanding.

External tools are not "fun breaks" from text work. They are different surfaces for the same learning. The injector system ensures that time spent in Onshape is still geometry time, and time spent in spreadsheets is still algebra time.

## Onshape (Parametric CAD)

### Why Onshape

- Professional-grade parametric CAD (not a toy)
- Browser-based (no install, runs on any machine)
- Free educational accounts
- Parametric modeling teaches mathematical thinking (constraints as equations)

### Integration Points

| Onshape Activity | Academic Connection | Injector Fires |
|-----------------|--------------------|----|
| Sketch constraints (perpendicular, parallel, tangent) | Geometry vocabulary and properties | Geometry seeker |
| Dimensioning | Measurement, units, precision | Numerical precision guard |
| Fillet/chamfer radius | Arc length, circle properties | Geometry seeker |
| Extrude (depth × area) | Volume calculation | Geometry seeker |
| Revolve | Solids of revolution (advanced) | Geometry seeker |
| Assembly mates | Spatial reasoning, coordinate systems | Geometry seeker |
| Parametric relationships | Functions, variables, dependencies | Digital literacy seeker |
| Bill of Materials | Budgeting, unit cost, totals | Economic reasoning seeker |
| Scale drawings / section views | Ratio, proportion, 2D representation of 3D | Proportional reasoning seeker |

### Workflow

1. **TUI assigns Onshape task** — "Design the nesting box. Requirements: 12"×12"×12" interior, removable top, 4" diameter entry hole."
2. **Student opens Onshape** and designs
3. **Student returns to TUI** and describes what they built
4. **Tutor evaluates** against requirements and asks questions
5. **Injectors fire** on the described activity — geometry, proportional reasoning, etc.

### Future: Direct API Integration

Onshape has a REST API. Future integration could:
- Read the student's model dimensions automatically
- Verify constraints match requirements
- Detect which geometric operations were used
- Provide real-time guidance in a side panel

## Spreadsheets

### Why Spreadsheets

- Formulas are algebraic expressions in a concrete context
- Data collection → analysis → visualization pipeline
- Every practical project generates data worth tracking

### Integration Points

| Spreadsheet Activity | Academic Connection | Injector Fires |
|---------------------|--------------------|----|
| Writing formulas | Algebraic expressions, order of operations | Digital literacy seeker |
| Cell references | Variables, function notation | Digital literacy seeker |
| SUM, AVERAGE, COUNT | Statistical measures | Math tutor content |
| Charts/graphs | Data visualization, choosing appropriate representation | Scientific method seeker |
| Budget tracking | Addition, subtraction, estimation | Economic reasoning seeker |
| Unit conversion columns | Ratio, multiplication | Proportional reasoning seeker |
| Conditional formatting | Logic, comparisons | Digital literacy seeker |

### Project Data Examples

| Project | Data Tracked | Spreadsheet Skills |
|---------|-------------|-------------------|
| Garden | Daily growth measurements, soil pH, water amount | Line charts, averages, data entry |
| Weather Station | Temperature, humidity, pressure, wind | Multi-series charts, statistical summaries |
| Chicken Coop | Budget (estimated vs. actual), egg production | Budget formulas, percentage calculations |
| Cookbook | Recipe costs, scaling factors, nutrition | Unit rates, multiplication formulas |

## Terminal / Command Line

### Why Terminal

- The TUI itself builds terminal comfort
- File management, text processing, basic scripting
- Automation concepts (do X for every file in this directory)
- Professional tool — not going away

### Integration Points

| Terminal Activity | Academic Connection | Injector Fires |
|-----------------|--------------------|----|
| File organization | Hierarchical thinking, categorization | Digital literacy seeker |
| Text search (grep) | Pattern recognition | Digital literacy seeker |
| Basic scripting | Algorithmic thinking, sequences | Digital literacy seeker |
| Pipes and composition | Function composition, data flow | Digital literacy seeker |
| Version control (git) | History, change tracking, responsibility | Digital literacy seeker |

### The command_teacher Connection

The original `command_teacher` prototype teaches terminal skills in a sandboxed environment. This becomes one module within Primer's broader curriculum — the "digital literacy" strand has a terminal/command-line track.

## Document Generation

### Markdown → PDF Pipeline

The student writes in markdown (via the TUI or a text editor). The system renders to PDF for:
- Formal assignment submissions
- Project documentation with photos
- Portfolio pieces
- Reading response journals

### Tools

- **Pandoc**: Markdown → PDF/HTML conversion
- **Typst** or **LaTeX**: For math-heavy documents
- **Graph rendering**: Matplotlib or similar for charts from data

## Physical Tools (Non-Digital)

These are integrated through documentation, not through software:

| Tool | Academic Role | Documentation Requirement |
|------|--------------|--------------------------|
| Tape measure | Geometry, measurement, precision | Record all measurements with units |
| Level | Perpendicular/parallel concepts | Note where level was used and why |
| Square | Right angles, perpendicularity | Identify which joints require squareness |
| Kitchen scale | Ratio, unit conversion | Record weights, calculate per-unit costs |
| Thermometer | Data collection, calibration | Log temperatures for weather/cooking projects |
| Multimeter | Electricity concepts (V, I, R) | Record measurements, calculate with Ohm's law |

The student uses these tools physically and documents the results in the TUI. The act of documentation — with units, precision, and reasoning — is where the academic learning is reinforced.
