# Investor pitch — launch runbook

Operational checklist for shipping and maintaining the System C investor site during fundraising. Companion to [phase-5-launch.md](../agent_docs/business/investor_pitch/implementation/phase-5-launch.md) and [DEPLOY.md](./DEPLOY.md).

## Pre-launch content gate

Automated (must be green):

```bash
make design-system
cd investor-web
npm test              # data + system-c + market math + launch + a11y
npm run typecheck
npm run lint
npm run build
npm run check:bundle
```

`npm run check:launch` verifies:

- ask $3.5M / 18 months / $300k cash salaries / team of 3
- Base $50 · Core $100 · Premier $300 scope and ARPU
- seed scorecard definitions, sources, caveats, NOT_YET_MEASURED currents
- market arithmetic and non-additive overlap groups
- premium-school source year + sticker/net or vintage caveats
- no unsupported “proven / validated / traction” language
- EVIDENCE_NEEDED items retained; LIVE products have artifacts
- milestone language aligned with scorecard floors/targets

Human confirmation before broad send:

- [ ] Product state labels match launch-day reality
- [ ] Family use is discovery only — not efficacy or traction
- [ ] Every EVIDENCE NEEDED item is either resolved or still visible
- [ ] Trademark/brand diligence wording is accurate
- [ ] Contact email is the real inbox (`VITE_CONTACT_EMAIL`)
- [ ] Meeting deck numbers match site data modules

## Quality gates

### Functional smoke

- [ ] All routes: `/` `/demo` `/market` `/evidence` `/schools` `/company` `/diligence`
- [ ] Home section anchors and mobile nav
- [ ] Theme toggle persists across reload
- [ ] Market explorer adjust / reset / copy equation
- [ ] Package ladder selection
- [ ] Demo step through to final step
- [ ] Contact CTA (mailto + `#contact`)
- [ ] Source links open in a new tab
- [ ] Staging: `robots` noindex; production: index only with flags

### Accessibility

Automated: `npm run check:a11y` (landmarks, skip link, focus, reduced motion, route titles).

Manual (required before broad release):

- [ ] Keyboard-only full flow (header, theme, explorers, contact)
- [ ] NVDA or VoiceOver pass on home + one diligence route
- [ ] Heading/landmark audit on each route
- [ ] Color contrast in dark and light themes
- [ ] Visible focus and skip link
- [ ] Chart/table text equivalence for explorers
- [ ] Zoom/reflow at 200% and 400%
- [ ] Print stylesheet on diligence pages (chrome hidden)

### Performance

- [ ] `npm run check:bundle` green (≈180KB gzip initial JS budget)
- [ ] Lighthouse mobile lab: performance ≥90, a11y ≥95, BP ≥95, SEO ≥95
- [ ] LCP <2.5s, CLS <0.1, INP <200ms in lab where available
- [ ] No third-party scripts blocking render

### Visual / System C

- [ ] Dark and light parity
- [ ] No radius, shadow, gradient, or glass drift (`npm run check:system-c`)
- [ ] Monospace reserved for system information
- [ ] Accent/attention semantics respected
- [ ] Feels like a structured record, not an admin dashboard

### Security / privacy

- [ ] Headers from `public/_headers` (or CDN equivalent)
- [ ] No student data; synthetic demo only
- [ ] Analytics transport is opt-in / no-op by default
- [ ] Dependencies scanned (`npm audit`)
- [ ] No source maps or private env values in `dist/`

## Investor review loop (human)

Do **not** invent results. Record real feedback only.

1. Send the static site to 3–5 founders who have raised pre-seed/seed.
2. Send to 2–3 education investors or operators.
3. Ask each the three-minute comprehension questions below.
4. Log every point of confusion in a shared note.
5. Revise content before adding visual complexity.
6. Repeat with the meeting deck so site and live pitch match.

### Three-minute comprehension test

After three minutes on the primary page, a reviewer should answer without prompting:

1. **Company** — What does Primer do in one sentence?
2. **Buyer** — Who pays first, and who pays later?
3. **Moat** — What is hard to copy if the product works?
4. **Market expansion** — How does the market grow without illegal additive TAM math?
5. **Current proof** — What is live vs hypothesis vs evidence needed?
6. **Ask** — How much is raised, for how long, and what must be true at month 18?

Pass criterion: five representative reviewers answer consistently; residual confusion is tracked and fixed.

## Analytics

Instrumented events (privacy-safe, no-op default):

| Event | When |
| --- | --- |
| `cta_click` / `contact_intent` | Header, footer, section, and block CTAs |
| `demo_start` / `demo_step` / `demo_complete` | Demo explorer lifecycle |
| `market_explorer_use` | Adjust, reset, copy equation |
| `package_compare` | Package ladder tier select |
| `diligence_route_view` | Visits to diligence routes |
| `source_click` | Citation link clicks |

Core metric: **qualified investor conversations generated**, not raw page views.

Wire a collector by assigning `window.__PRIMER_ANALYTICS__` — never commit vendor keys.

## Post-launch cadence

### Weekly during fundraising

- [ ] Update product state and measurable milestones in data modules
- [ ] Verify scheduling/contact flow (mailto + any form)
- [ ] Review qualified engagement events
- [ ] Fix confusing copy quickly
- [ ] Keep current state candid (no polish over unknown)

### Monthly

- [ ] Update seed scorecard currents when measured
- [ ] Refresh market/source access dates where changed
- [ ] Review inference economics and package experiments
- [ ] Add validated artifacts; archive stale claims visibly
- [ ] `npm audit` and dependency bumps as needed

### Quarterly

- [ ] Full source audit (`sources.ts` + citations)
- [ ] Dependency/security review
- [ ] System C regression review (`check:system-c` + visual pass)
- [ ] Accessibility retest (automated + manual)
- [ ] Market and competitor refresh
- [ ] Investor feedback synthesis → content revision backlog

## Exit criteria (phase 5)

- [ ] All automated launch gates pass
- [ ] Manual a11y and functional smoke complete
- [ ] At least five representative investor/operator reviews completed (human)
- [ ] Three-minute comprehension test succeeds consistently
- [ ] Deployment and update workflow documented (`DEPLOY.md`)
- [ ] Public site, meeting deck, and diligence narrative share canonical data modules
- [ ] Site is ready to support a time-boxed fundraising process
