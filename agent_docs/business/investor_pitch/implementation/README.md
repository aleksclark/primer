# Investor-pitch site implementation

This directory turns the investor narrative into a phased implementation plan. The site should feel immediately familiar to investors: concise company thesis, problem, product, proof, market, model, GTM, competition, team, current state, and ask. System C — Record supplies the distinctive visual language without changing that expected information architecture.

## Implementation principles

1. **Familiar investor sequence:** no experimental navigation or hidden core claims.
2. **System C throughout:** generated semantic tokens, square ruled geometry, Instrument Sans content voice, IBM Plex Mono system voice, sparse accent, no shadows or gradients.
3. **Truthful status:** `LIVE`, `IN DEVELOPMENT`, `PLANNED`, `HYPOTHESIS`, and `EVIDENCE NEEDED` appear consistently.
4. **Evidence near claims:** source links and caveats stay adjacent to numbers.
5. **One canonical data source:** market, pricing, status, and milestones are structured content, not duplicated prose across components.
6. **Progressive enhancement:** the primary page remains readable without animation or client-only interaction.
7. **Investor speed:** the core thesis is scannable in under three minutes; deeper routes support diligence.
8. **Accessible by construction:** WCAG 2.2 AA target, reduced motion, keyboard navigation, semantic charts, and equivalent text summaries.
9. **No student data:** demos use synthetic/redacted artifacts only.
10. **No premature portal:** gated data-room integration comes after the public narrative is correct.

## Proposed location and stack

Create a dedicated `investor-web/` Vite application rather than mixing investor messaging into the admin SPA. Reuse the repository's proven stack:

- React 19;
- TypeScript;
- Vite;
- Tailwind CSS 4 where useful for layout;
- React Router;
- generated `design-system/generated/primer.css` tokens;
- Radix primitives only where interaction requires them;
- Lucide icons used sparingly.

A dedicated app keeps the public deployment, analytics, performance budget, and dependencies independent from authenticated LMS administration while sharing System C assets.

## Phases

- [Phase 0 — content and data contract](phase-0-content-data.md)
- [Phase 1 — foundation and shell](phase-1-foundation.md)
- [Phase 2 — core investor narrative](phase-2-core-narrative.md)
- [Phase 3 — market, model, and evidence interactions](phase-3-interactive-proof.md)
- [Phase 4 — diligence routes and product demo](phase-4-diligence-demo.md)
- [Phase 5 — quality, launch, and iteration](phase-5-launch.md)

## Definition of done

The initial public release is done when:

- every core investor section is implemented and sourced;
- product state is unambiguous;
- Base/Core/Premier markets cannot be accidentally summed;
- the $3.5M pre-seed ask maps to the 18-month seed-readiness scorecard;
- the founder and current-state sections contain no unsupported traction claims;
- System C review gates pass in dark and light themes;
- Lighthouse/accessibility/performance budgets pass;
- all mobile content has an equivalent linear reading order;
- analytics capture only investor-intent events;
- source and diligence links work in the production build.
