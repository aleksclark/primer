# Phase 3 — Market, model, and evidence interactions

## Goal

Add interactions that help an investor test assumptions without turning the pitch into a dashboard or obscuring the core narrative.

## Market expansion explorer

### Required behavior

- display Base/Core/Premier as separate vertical expansion layers;
- let the user change learners and annual ARPU;
- recalculate each layer from structured fields;
- show source year and observed/modeled/derived state;
- keep `additive: false` visible;
- disable combined totals for overlapping populations;
- provide a copyable equation;
- reset to sourced defaults.

### Institutional explorer

Separate from family expansion:

- public intervention: targeted learners × $1,200;
- elite learning support: supported learners × selected annual price;
- microschool academic core: enrolled learners × institutional ARPU;
- international school: enrolled learners × annual license.

Do not combine these with family subscriptions unless the contracting party and deduplication rule are explicit.

## Package ladder interaction

Allow an investor to compare:

- buyer;
- scope;
- monthly/annual ARPU;
- included subjects;
- planning/assessment responsibility;
- projects/content/reporting;
- support level;
- COGS target/current state;
- expansion validation status.

The default view is a simple three-column ruled comparison. Avoid consumer-pricing-page visual language such as a highlighted “most popular” card.

## Seed scorecard

Implement floor/target/current columns for:

- learners;
- ARR run rate;
- 12-week retention;
- gross margin;
- COGS;
- CAC/payback;
- activation;
- learning outcome;
- mastery scoring;
- school pilots;
- Core/Premier expansion tests.

Current unknown values display `NOT YET MEASURED`, never zero.

## Evidence explorer

Each research claim expands to show:

- paper and year;
- study population;
- design;
- effect;
- safe claim;
- limitation;
- direct URL.

Keep Primer's company-evidence panel visually distinct from prior research.

## Product-state explorer

Add optional filtering by:

- learner experience;
- parent/educator experience;
- LMS core;
- media/projects;
- school integration;
- compliance/research.

State filters cannot remove the legend or make planned work appear live.

## Motion and interaction design

System C motion is functional:

- 120ms hover/focus contrast;
- 200ms disclosure and tab transitions;
- 320ms maximum for diagram state changes;
- no parallax, scroll hijacking, animated counters, background particles, or decorative number rolling;
- reduced-motion mode removes nonessential transitions.

Use accent to show the active assumption or selected tier. Use attention only for overlap warnings, evidence gaps, and risks.

## Technical approach

- prefer native CSS grid, details/summary, and small React state machines;
- use URL query parameters for shareable market assumptions only if stable;
- avoid a chart library unless accessible semantic output cannot be achieved with CSS/SVG;
- if using SVG, include titles/descriptions and an adjacent table;
- persist no sensitive data;
- calculations use tested pure functions.

## Tests

- property tests for market arithmetic and formatting;
- overlap prevention test;
- floor/target/current state rendering;
- keyboard interaction for tabs/disclosures/sliders;
- screen-reader labels for every control and result;
- reduced-motion behavior;
- URL-state roundtrip if enabled;
- no interaction required to access source/caveat text;
- visual tests for long labels and large numbers.

## Exit criteria

- interactions improve comprehension in user review;
- no interaction changes the canonical sourced defaults silently;
- all calculations match business-document tests;
- accessibility and performance budgets still pass;
- static fallback remains complete.
