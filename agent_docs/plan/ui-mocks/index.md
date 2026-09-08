# Cross-platform UI mocks

## Outcome

Deliver a code-first mock workflow in which a local coding agent composes Primer Tasks web and Android screens from production components, a reviewer comments on exact rendered UI in Chromatic, and approved screens become executable visual baselines and implementation starting points.

## Current state

Primer System C already generates web and Compose tokens from `design-system/tokens.json`. Tasks has production React screens, a shared Android `core-ui`, Compose student screens, Playwright acceptance, and Compose instrumentation tests. It lacks a component workbench, deterministic screen fixtures, cross-platform review catalog, visual baselines, and a documented review-to-implementation loop.

## Scope

In scope:

- a pilot spanning pairing, checklist, task start, submission, rejection/retry, and completion;
- Storybook stories built from production React components;
- Compose previews built from production Android composables;
- Chromatic-compatible web and exported Android review surfaces;
- deterministic web screenshot verification and Android preview/build verification;
- an executable local runbook.

Out of scope:

- replacing the real Tasks API, auth, Playwright acceptance, or device tests;
- persisting review comments in Primer databases;
- treating mocks as proof of server behavior, pairing security, camera behavior, or physical-device behavior;
- migrating every Primer UI during the pilot.

## Global constraints

- System C and generated tokens remain authoritative.
- Production containers own API, auth, navigation, and mutations; mock fixtures enter only at stateless screen boundaries.
- Mock cases are deterministic and contain no credentials, cookies, bearer tokens, or live household data.
- Human-readable names lead UI identity.
- Android and web represent the same workflow states without pretending to share generated transport DTOs.
- Visual approval complements behavioral acceptance and never replaces it.
- Baseline updates are explicit review actions, never automatic test repairs.

## Phases

| Phase | Goal | Depends on |
|---|---|---|
| [Phase 1: Reusable screen contracts](./phase-01-screen-contracts.md) | Extract production-ready web and Android screen boundaries with deterministic workflow states. | None |
| [Phase 2: Review catalog](./phase-02-review-catalog.md) | Present web stories and Android previews through a reviewable catalog with component-level targets. | Phase 1 |
| [Phase 3: Visual verification](./phase-03-visual-verification.md) | Establish reproducible screenshot and build gates tied to approved mock states. | Phase 2 |
| [Phase 4: Pilot runbook](./phase-04-pilot-runbook.md) | Document and execute the complete Tasks mock workflow. | Phase 3 |

## Completion rule

The plan is complete only when production and mock surfaces use the same screen components, all pilot states are selectable without a backend, Storybook and Android previews build, web visual comparisons pass, the real Tasks Playwright pilot remains green, the runbook is executable from a clean checkout, and no mock-only branch enters production runtime code.
