# Phase 1 — Foundation and shell

## Goal

Create the deployable public application, implement System C faithfully, and establish the shared layout/accessibility primitives used by every investor section.

## Application setup

Create `investor-web/` with:

- Vite + React + TypeScript;
- React Router routes from [../site-map.md](../site-map.md);
- import of `../design-system/generated/primer.css` through a repository-relative or copied build artifact managed by the build;
- `public/` logo assets sourced from `design-system/assets/logo/`;
- production build and lint scripts matching `web/` conventions;
- no API client until a real investor-contact/data-room endpoint exists.

Add Make targets:

- `make investor-web`;
- `make investor-web-dev` if repository conventions permit;
- include `make design-system` before production build;
- include investor-web build in CI.

## System C implementation

### Token rules

- consume semantic CSS variables only;
- dark theme default, full light parity via `data-theme="light"`;
- no hard-coded palette values in components;
- 4px spacing grid;
- zero border radius;
- 1px ruled structure;
- no shadows, gradients, glass effects, or decorative blobs;
- accent only for action/state/focus;
- attention only for risk/review/error.

### Typography

- Instrument Sans for display, headings, and body;
- IBM Plex Mono uppercase/tracked for labels, metadata, navigation, metrics, state, and controls;
- responsive investor-site display sizes may extend the token scale through semantic component classes, but preserve the System C families, weights, and tracking;
- constrain prose measure for rapid scanning.

### Shared primitives

Build:

- `SiteHeader` with anchored primary nav;
- `SiteFooter` with contact/source links;
- `SectionFrame` with ruled heading and optional index;
- `SystemLabel` for mono metadata;
- `StateBadge` for the five publication states;
- `MetricBlock` and `MetricRow`;
- `RuledCard` and `RuledGrid`;
- `SourceLink` and `CaveatNote`;
- `PrimaryButton` and `TextLink` derived from canonical System C controls;
- `ThemeToggle`;
- `SkipLink`;
- `MobileSectionNav`;
- `InvestorCTA`.

Do not create rounded marketing cards or generic dashboard widgets.

## Familiar investor shell

The primary page order remains visible in HTML and navigation:

1. thesis;
2. problem;
3. product;
4. evidence;
5. market;
6. GTM;
7. competition;
8. founder/team;
9. current state;
10. business model;
11. ask/contact.

The distinctive design comes from presentation, not from rearranging expected investor information.

## Accessibility foundation

- semantic landmarks and heading hierarchy;
- visible keyboard focus using System C token rules;
- skip navigation;
- no color-only status;
- reduced-motion media query and motion utility;
- theme preference persistence without blocking first paint;
- accessible route announcements;
- minimum 44px target size where appropriate despite compact visual style;
- chart summary slot required by every future visualization.

## Performance foundation

- self-host or efficiently preload only required font weights;
- static-first content;
- route-level code splitting for diligence/demo routes;
- no animation dependency in Phase 1;
- no heavy chart library before Phase 3 proves it necessary;
- optimized SVG logos;
- define performance budgets: <180KB initial JS gzip target, LCP <2.5s on mobile lab profile, CLS <0.1.

## Tests

- System C build tests run first;
- app typecheck, lint, and production build;
- semantic smoke tests for page landmarks and headings;
- keyboard traversal test for header/theme/navigation;
- dark/light screenshot checks at desktop and mobile widths;
- reduced-motion test;
- no raw hex/rgb values outside generated tokens/assets;
- no nonzero border radius or box shadow in authored investor styles.

## Exit criteria

- application deploys with placeholder section frames;
- dark/light themes pass visual review;
- all shared primitives match System C reference behavior;
- responsive navigation works without JavaScript-only content loss;
- CI/build commands are documented and passing.
