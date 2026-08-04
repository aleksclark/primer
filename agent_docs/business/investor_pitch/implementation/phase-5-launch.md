# Phase 5 — Quality, launch, and iteration

## Goal

Ship a fast, accessible, source-complete investor site with controlled analytics and a repeatable content-update process.

## Pre-launch content gate

Confirm:

- ask, runway, salaries, roles, and milestones match the funding plan;
- Base/Core/Premier scope and pricing match all business documents;
- seed-readiness scorecard definitions are consistent;
- market arithmetic is tested;
- premium-school data includes source year and sticker/net caveats;
- all current product state is accurate on launch day;
- no family use is described as efficacy or traction;
- every `EVIDENCE NEEDED` item is either resolved or visibly retained;
- trademark/brand diligence status is accurately described where relevant.

## Quality gates

### Functional

- all routes and anchors;
- theme selection;
- market calculations/reset/share;
- disclosures/tabs;
- contact CTA;
- source and download links;
- staging noindex and production indexing.

### Accessibility

- automated WCAG checks;
- keyboard-only full flow;
- NVDA or VoiceOver manual pass;
- visible focus and skip link;
- heading/landmark audit;
- color contrast in both themes;
- reduced motion;
- chart/table equivalence;
- zoom and reflow at 200%/400%.

### Performance

- Lighthouse mobile targets: performance ≥90, accessibility ≥95, best practices ≥95, SEO ≥95;
- LCP <2.5s, CLS <0.1, INP <200ms in lab/field where available;
- initial JavaScript budget from Phase 1;
- image and font budgets;
- route-level bundle report;
- no third-party scripts blocking render.

### Visual/System C

- dark and light parity;
- no radius, shadow, gradient, glass, or decorative color drift;
- all components consume semantic tokens;
- monospace voice restricted to system information;
- accent/attention semantics respected;
- mobile ruled layouts remain legible;
- presentation feels like a structured record, not an admin dashboard.

### Security/privacy

- static hosting where possible;
- CSP, HSTS, referrer policy, permissions policy;
- no student data;
- minimal analytics with IP/privacy controls;
- spam-resistant contact flow;
- dependencies scanned;
- no source maps or private environment values exposed unintentionally.

## Analytics

Track:

- primary CTA click and completed contact;
- demo start/completion;
- market explorer use;
- package comparison interaction;
- evidence/diligence route visit;
- source click;
- section depth and return visits where privacy-safe.

Do not optimize for raw page views. The core metric is qualified investor conversation generated.

## Investor review loop

Before broad release:

1. send static site to 3–5 founders who have raised pre-seed/seed;
2. send to 2–3 education investors/operators;
3. ask each to state the company, buyer, moat, market expansion, current proof, and ask after a three-minute review;
4. record every point of confusion;
5. revise content before adding visual complexity;
6. repeat with the meeting deck to ensure site and live pitch match.

## Deployment

- production domain and canonical metadata;
- preview deployments for every pull request;
- immutable static assets with caching;
- design-system generation before build;
- CI: design-system tests → investor data tests → typecheck/lint → build → accessibility/smoke → deploy preview;
- rollback documented;
- uptime monitoring for production URL and contact endpoint.

## Post-launch cadence

### Weekly during fundraising

- update product state and measurable milestones;
- verify scheduling/contact flow;
- review qualified engagement;
- fix confusing copy quickly;
- keep current state candid.

### Monthly

- update seed scorecard;
- refresh market/source access dates where changed;
- review inference economics and package experiments;
- add validated artifacts;
- archive stale claims rather than editing history invisibly.

### Quarterly

- full source audit;
- dependency/security review;
- System C regression review;
- accessibility retest;
- market and competitor refresh;
- investor feedback synthesis.

## Exit criteria

- all launch gates pass;
- at least five representative investor/operator reviews completed;
- the three-minute comprehension test succeeds consistently;
- deployment and update workflow is documented;
- public site, meeting deck, and diligence narrative use the same canonical data;
- site is ready to support a time-boxed fundraising process.
