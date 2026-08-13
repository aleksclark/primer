# Phase 7: OpenAPI baseline handoff and REST clients

## Goal

Complete the authoring contract handoff: Huma-emitted OpenAPI becomes the live
generation input; the hand-authored YAML is frozen as an **immutable
compatibility baseline artifact**; TypeScript and Go REST client packages are
generated and become the exclusive authoring consumption path for Studio UI and
tests.

**Depends on:** Phase 6 (and Phase 2 parity on emitted spec)
**Duration guess:** 3–5 days

## BDD Success Criteria

#### Scenario: P7-S1 — Hand YAML demoted to compatibility baseline

- **Given** emitted OpenAPI matches baseline within agreed compatibility policy
  (no breaking deltas; additive OK)
- **When** handoff PR lands
- **Then** README states: live SoT = Huma handlers;
  `openapi/v1/curriculum-studio.yaml` retained as baseline **or** moved to
  `contracts/baselines/openapi-v1-handoff.yaml` immutably
- **And** CI breaks if someone edits baseline without explicit “bootstrap
  baseline” procedure

#### Scenario: P7-S2 — TS authoring client generated and compiles

- **Given** emitted OpenAPI
- **When** `openapi-typescript` (pin ≥ LMS) generates into
  `clients/ts-rest/generated/` (gitignored) and façade package builds
- **Then** `tsc --strict` passes
- **And** package exports `createClient` using `openapi-fetch` or equivalent

#### Scenario: P7-S3 — Go REST client generated and compiles

- **Given** emitted OpenAPI
- **When** chosen Go generator (spike-qualified) runs
- **Then** `clients/go-rest` builds against generated internals
- **And** façade does not redefine models

#### Scenario: P7-S4 — Exclusive authoring consumption in Studio tree

- **Given** sample consumer test / stub UI data layer
- **When** it lists workspaces / gets health
- **Then** it imports only `clients/ts-rest` or `clients/go-rest`
- **And** raw path strings to `/studio/v1` outside allowlist fail lint (full ban
  Phase 10; Phase 7 introduces allowlist file)

#### Scenario: P7-S5 — Client success and typed error calls hit real handlers

- **Given** loopback Huma server with harness
- **When** generated TS and Go clients call health + one authz-denied path
- **Then** success decodes typed schema; error decodes ErrorModel/ErrorCode
- **And** no snapshot of hand-written fetch wrappers remains as primary

#### Scenario: P7-S6 — Baseline freeze procedure documented

- **Given** handoff complete
- **When** operator follows README baseline section
- **Then** steps exist to publish new baseline from release emission only

## Implementation Instructions

1. **Handoff checklist:**
   - emitted vs baseline: zero breaking changes (openapi-diff / oasdiff)
   - parity green on emitted
   - all baseline operationIds present or explicitly deprecated with replacement
   - ownership scanner green

2. **Baseline storage:**
   Prefer keep path `openapi/v1/curriculum-studio.yaml` but add header comment:

   ```yaml
   # COMPATIBILITY BASELINE — do not edit as live SoT.
   # Live authoring contract: Huma handlers via cmd/openapi-gen.
   # Frozen at handoff commit <sha>.
   ```

   Or relocate to `baselines/` and leave a stub README pointer — choose one in
   implementation; update crosswalk cite if path changes (docs PR).

3. **Client generate scripts** in each client package:
   - `npm run generate` / `go generate`
   - root `make clients-generate` depends on `contracts-openapi-emit`

4. **Façades:** base URL, Bearer injection, problem+json normalize.

5. **UI interface:** document that future Studio UI plan must depend on
   `clients/ts-rest` only.

6. **Do not** commit generated TS/Go client sources.

### Focused verification

- clean generate + tsc + go test
- loopback client E2E
- baseline edit guard script

## End-to-End Test Plan

| ID | Setup | Action | Assert |
| --- | --- | --- | --- |
| E7-01 | handoff tree | README + baseline header/guard | baseline not live SoT |
| E7-02 | emit+generate | tsc + go build clients | success |
| E7-03 | loopback server | TS client getHealth | 200 typed |
| E7-04 | ownership scanner | emitted OpenAPI | non-overlap holds |
| E7-05 | Go client | denied call | ErrorCode decoded |
| E7-06 | delete generated/ | regenerate from clean | success |

## Anti-Cheating Audit

- Keeping UI on hand-written types while claiming handoff
- Committing generated client for “convenience”
- Editing baseline to match bad emission instead of fixing handlers
- Dual-running hand YAML generate path in Makefile silently
- Client tests that never start Huma

## Completion Gate

- [ ] P7-S1…P7-S6 pass
- [ ] E7-01…E7-06 evidence
- [ ] Makefile graph: emit → generate → consumer build
- [ ] Crosswalk/contracts README updated for handoff
- [ ] No tracked generated client files

## Dependencies and rollback

- **Depends on:** Phase 6
- **Unblocks:** Phases 8, 10–11
- **Rollback:** restore hand YAML as live SoT; pin clients to baseline emission
  from frozen file; mark handoff CONDITIONAL
