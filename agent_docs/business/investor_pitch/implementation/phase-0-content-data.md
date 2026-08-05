# Phase 0 — Content and data contract

## Goal

Freeze the first investor narrative and encode every changing claim as structured data before building presentation components.

## Inputs

- [../narrative.md](../narrative.md)
- [../content-spec.md](../content-spec.md)
- [../../market.md](../../market.md)
- [../../long-term-market.md](../../long-term-market.md)
- [../../seed-readiness-economics.md](../../seed-readiness-economics.md)
- [../../sources.md](../../sources.md)

## Deliverables

### Content inventory

Create a section manifest with:

- stable section ID;
- navigation label;
- eyebrow, headline, body, and CTA;
- status label;
- artifact requirement;
- citations;
- inline caveat;
- publication blocker.

### Structured datasets

Define TypeScript data modules or validated JSON for:

- `productState`: live/in-development/planned capabilities;
- `pricingTiers`: Base/Core/Premier scope, ARPU, buyer, COGS state;
- `marketLayers`: population, annual ARPU, modeled ceiling, source year, overlap group;
- `seedScorecard`: floor, target, current value, definition, source;
- `researchClaims`: effect, population, study, safe claim, caveat;
- `competitorCategories`: verified category-level comparison;
- `founderProof`: timeline and resume-backed execution facts;
- `fundingPlan`: people, operating categories, contingency, total;
- `sources`: title, organization, URL, publication date, accessed date, quality tier.

### Market overlap model

Each market record must include an overlap key and an `additive: false` default. The UI must never produce an automatic total across Base/Core/Premier layers.

Example shape:

```ts
type MarketLayer = {
  id: string
  package: "base" | "core" | "premier" | "institutional"
  segment: string
  learners: number
  annualRevenuePerLearner: number
  modeledCeiling: number
  sourceYear: string
  observedOrModeled: "observed" | "modeled" | "derived"
  overlapGroup: string
  additive: false
  caveat: string
  sourceIds: string[]
}
```

### Copy approval checklist

Before Phase 1:

- confirm the $50/$100/$300 package names and scope;
- confirm $3.5M/18-month ask language;
- confirm founder timeline and eBackpack metrics;
- confirm current product-state labels;
- identify every claim still marked `EVIDENCE NEEDED`;
- decide whether the seed scorecard displays current blank values or `NOT YET MEASURED`;
- identify sources that need refreshed 2023–24/2025–26 private-school data.

## Tests

- schema validation for every dataset;
- duplicate source-ID detection;
- arithmetic tests for every market ceiling;
- no market layer marked additive;
- no `LIVE` product without an artifact link;
- no metric without a definition and source/caveat;
- no unsupported words such as `proven`, `validated`, or `traction` outside explicitly qualified contexts.

## Exit criteria

- copy and data review completed;
- all arithmetic reproducible from source fields;
- unresolved claims explicitly blocked rather than silently omitted;
- component work can proceed without hard-coding business facts into JSX.
