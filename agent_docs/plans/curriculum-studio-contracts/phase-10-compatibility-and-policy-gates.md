# Phase 10: Compatibility, exclusive-use, clean-checkout gates

## Goal

Install fail-closed policy gates: immutable compatibility baselines
(`buf breaking`, OpenAPI breaking), no tracked generated outputs, unauthorized
raw transport bans, deterministic double-generation, planted red-failure proofs,
and clean-checkout pipeline.

**Depends on:** Phases 4–9
**Duration guess:** 3–4 days

## BDD Success Criteria

#### Scenario: P10-S1 — buf breaking against immutable prior image

- **Given** baseline image artifact from last release/handoff under
  `contracts/baselines/` or CI artifact fetch
- **When** candidate protos break a field
- **Then** `buf breaking --against <baseline>` fails
- **And** additive optional field passes

#### Scenario: P10-S2 — OpenAPI breaking against immutable baseline

- **Given** frozen OpenAPI baseline artifact
- **When** emitted OpenAPI removes a required property or operation
- **Then** breaking gate fails
- **And** missing baseline fails closed (no silent skip) except first bootstrap
  documented once

#### Scenario: P10-S3 — No tracked generated outputs

- **Given** generate has been run
- **When** `tools/contract-gates/check_tracked_gen.sh` runs
- **Then** exit 0 iff no tracked matches under gen patterns
- **And** planting `git add -f contracts/gen/foo.go` is caught in CI simulation

#### Scenario: P10-S4 — Unauthorized raw fetch/grpc banned

- **Given** eslint/oxlint/grep/go-analyzer rules for Studio base paths and
  `grpc.Dial` to Studio without client package
- **When** a consumer file uses raw `fetch('/studio/v1/...')` or hand HTTP
- **Then** lint fails unless path is allowlisted (conformance harness only)
- **And** allowlist is machine-readable and reviewed

#### Scenario: P10-S5 — Deterministic generation

- **Given** clean output dirs
- **When** full generate graph runs twice
- **Then** normalized digests of OpenAPI emission, buf image, and client
  outputs match

#### Scenario: P10-S6 — Planted red failures for each critical gate

- **Given** scripts under `tools/contract-gates/red/`
- **When** each mutation is applied in CI job or make target `gates-red-proof`
- **Then** the corresponding gate fails, then workspace is restored green
- **And** evidence log lists gate → red → green

#### Scenario: P10-S7 — Clean-checkout gate

- **Given** a fresh worktree/clone without gen dirs
- **When** `make contracts-ci` (validate → parity → generate → test → gates)
- **Then** all succeed
- **And** final `git status` is clean (no untracked required artifacts left
  dirty beyond ignore rules)

## Implementation Instructions

1. **Baseline bootstrap:**
   - Export current `buf build -o baselines/curriculumstudio.v1.buf.binpb`
   - Freeze OpenAPI handoff YAML as baseline
   - Check in **binary/text baselines** only as immutable artifacts (allowed:
     prior contract snapshots — not generated client code)
   - Document update procedure: only on versioned release

2. **Gate scripts:**
   - `check_buf_breaking.sh`
   - `check_openapi_breaking.sh` (oasdiff/openapi-diff)
   - `check_tracked_gen.sh`
   - `check_raw_transport.sh`
   - `check_deterministic.sh`
   - `red_proof.sh`

3. **Raw transport ban patterns:**
   - `/studio/v1`
   - `curriculumstudio.v1.CurriculumIntegrationService` via raw stubs
   - Allow: `internal/**/conformance/**`, `tools/**`

4. **CI integration:** add job or document until monorepo workflow exists under
   `.github/workflows` — local `make contracts-ci` is authoritative if GHA
   absent.

5. **Planted reds (minimum):**
   - rename proto field
   - remove OpenAPI path
   - track fake pb.go
   - add raw fetch in dummy consumer file
   - nondeterministic timestamp in emission without normalize
   - break enum parity

### Focused verification

- `make contracts-ci`
- `make gates-red-proof` (or script) restores clean tree

## End-to-End Test Plan

| ID | Setup | Action | Assert |
| --- | --- | --- | --- |
| E10-01 | baseline image | buf breaking on broken proto | fail; additive pass |
| E10-02 | baseline OpenAPI | breaking on removed op | fail |
| E10-03 | force-add gen file | tracked-gen check | fail |
| E10-04 | raw fetch plant | raw ban lint | fail |
| E10-05 | double generate | digests | equal |
| E10-06 | red_proof script | all mutations | red then green restore |
| E10-07 | fresh worktree | contracts-ci | green + clean status |

## Anti-Cheating Audit

- Skipping breaking checks when baseline missing
- Allowlist `*` for raw transport
- Red proof that does not restore / hides failures
- Determinism check only on proto not OpenAPI clients
- CI not running gates locally claimed green

## Completion Gate

- [ ] P10-S1…P10-S7 pass
- [ ] E10-01…E10-07 evidence
- [ ] baselines committed or CI-artifact-fetched with fail-closed miss
- [ ] `make contracts-ci` documented as T10 entry
- [ ] red proof log retained in tools/ or CI artifacts docs

## Dependencies and rollback

- **Depends on:** Phases 4–9
- **Unblocks:** Phase 11
- **Rollback:** leave gates advisory only with explicit FAIL note — not allowed
  for plan completion
