# Phase 9: SPA/BFF shell

**File:** `phase-09-spa-bff-shell.md`
**Depends on:** Phase 3 (auth + workspaces APIs available; later APIs stubbed as not-found until ready)
**Duration guess:** 5–7 days
**Handoff wave:** see index orchestrator map

## Goal

Stand up the Curriculum Studio authoring SPA and BFF integration boundary using the Editorial Instrument house system: dark-first tokens, generated OpenAPI TypeScript client only, host-only session cookies via BFF, accessible shell navigation, and real browser E2E proving login-session → authenticated API. UI talks only to Studio API/BFF.

## Scope

### In scope

- Scaffold `curriculum-studio/web` Vite React TS Tailwind v4
- Install house tokens (`DESIGN.md`, `house-tokens.css`, fonts Archivo/Newsreader/IBM Plex Mono)
- Accent `#3DE0F0` only
- openapi-typescript client from Studio OpenAPI (`npm run generate:client`)
- BFF routes: login entry (test mode), callback stub, logout, csrf, me
- Shell layout: nav Explore/Operate/Configure surfaces, workspace switcher names-first
- Playwright config + first E2E against `studio-server` + web preview
- `make studio-web`, `make studio-client`, `make studio-e2e`
- Optional early `cmd/openapi-gen` if Huma routes cover enough; else copy contracts YAML as baseline with drift check

### Out of scope / YAGNI

- Full planning editors (Phase 10)
- Live Google OIDC (Phase 18 BLOCKED)
- LMS web reuse

## BDD Success Criteria

#### Scenario: P9-S1 — Dark-first shell renders

- **Given** web app built with house tokens
- **When** user opens SPA root authenticated
- **Then** dark background canonical
- **And** Archivo UI font applied
- **And** single cyan accent visible on selection/primary button
- **And** no second accent hue

#### Scenario: P9-S2 — Light parity geometry

- **Given** theme toggle or prefers
- **When** switch to light
- **Then** spacing/typography geometry preserved
- **And** semantics same

#### Scenario: P9-S3 — BFF session cookie

- **Given** test auth mode stack
- **When** user completes test login from SPA
- **Then** host-only HttpOnly cookie set
- **And** JS cannot read session secret
- **And** /auth/me succeeds via cookie credentialed fetch

#### Scenario: P9-S4 — Generated client only

- **Given** SPA source tree
- **When** static scan / runtime API calls
- **Then** API calls go through openapi-fetch generated paths
- **And** no hand-rolled fetch to /workspaces bypassing client without justification allowlist

#### Scenario: P9-S5 — Accessibility shell

- **Given** SPA shell
- **When** keyboard navigate primary nav + axe scan
- **Then** focus visible
- **And** no critical axe violations on shell
- **And** icons have names

## Implementation Instructions

1. Copy house token assets into `curriculum-studio/web/src/styles/`.
2. Package scripts mirror LMS web: generate:client, build, lint (oxlint).
3. Vite proxy dev to studio-server; credentialed cookies same-site via shared localhost port strategy or BFF same origin — **prefer same-origin**: studio-server serves SPA static in prod (like LMS embed) OR reverse proxy; document choice. Default decision: **dev** Vite proxy with `credentials`; **prod** embed `web/dist` in studio-server similar to `server/internal/spa`.
4. Auth provider React context from `/auth/me`.
5. Workspace switcher lists names; IDs monospace secondary with copy.
6. Playwright: start stack script `curriculum-studio/web/e2e/global-setup.ts`.
7. Route placeholders for curricula/plans returning empty states.

## End-to-End Test Plan

#### P9-E1 — Visual shell browser

- **Setup:** Playwright + stack
- **Action:** screenshot desktop+mobile dark
- **Assert:**
  - tokens applied
  - nav usable
- **Command:** `make studio-e2e`

#### P9-E2 — Login cookie path

- **Setup:** test mode
- **Action:** login → me → logout
- **Assert:**
  - cookie set/cleared
  - post-logout 401
- **Command:** `make studio-e2e --grep auth`

#### P9-E3 — Client generation gate

- **Setup:** CI
- **Action:** npm run generate:client && git diff --exit-code schema.d.ts or build
- **Assert:**
  - client builds
  - tsc clean
- **Command:** `make studio-web`

#### P9-E4 — Axe shell

- **Setup:** Playwright axe
- **Action:** scan shell
- **Assert:**
  - 0 critical
- **Command:** `make studio-e2e --grep a11y`

## Anti-Cheating Audit

- SPA must not store JWT in localStorage
- Must not bypass BFF with implicit allow-all mock user in production builds
- House tokens not left as default shadcn zinc without replacement
- E2E not screenshot-only without auth assertion
- Generated client committed policy: commit schema.d.ts or generate in CI — pick one and enforce

## Completion Gate

- [ ] P9-S*/E* green
- [ ] DESIGN.md present in web tree
- [ ] Document same-origin/prod static strategy
- [ ] Test login path named for Phase 18 replacement


## Dependencies

- Upstream: Phase 3 (auth + workspaces APIs available; later APIs stubbed as not-found until ready)
- Sibling tracks: schema (`curriculum-studio/db`), contracts (`curriculum-studio/contracts`), identity (when leaving test auth)
- Downstream consumers: later phases listed in index

## Rollback

- Revert the phase PR(s); drop any additive migrations only via forward-fix sibling db track (never destructive rollback in prod without backup).
- Feature flags: prefer `STUDIO_*` env gates over half-applied routes.
- SPA: revert `curriculum-studio/web` routes; keep generated client in sync with OpenAPI.
