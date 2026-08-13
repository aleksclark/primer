# Phase 1: Ownership freeze and package layout

## Goal

Freeze Curriculum Studio contract ownership, repository package layout, module
paths, gitignore/generated-output policy, and the skeleton build graph so later
phases implement generation and harnesses without redesigning boundaries.

After this phase, implementers know exactly where protobuf, OpenAPI baselines,
server edge packages, client packages, and ephemeral IR live — and what other
plans must not own.

**Depends on:** None
**Duration guess:** 1–2 days

## BDD Success Criteria

#### Scenario: P1-S1 — Non-overlap ownership is documented and enforceable

- **Given** the foundation crosswalk L6 and `curriculum-studio/contracts/README.md`
- **When** a reviewer inspects the plan package map and ownership lint notes
- **Then** OpenAPI is declared browser/public authoring only and protobuf is
  declared Primer/machine integration only
- **And** a mechanical check fails if OpenAPI components reintroduce
  `MaterializationContext` or full Primer bundle field trees (allowlist: thin
  authoring subset types already present)

#### Scenario: P1-S2 — Package boundaries encode server → emit → client → consumer

- **Given** an empty or partial `curriculum-studio/` service tree
- **When** the layout skeleton and MODULE/README docs are added
- **Then** directories exist (or are reserved) for:
  - `curriculum-studio/contracts/` (IDL + baseline + validate)
  - `curriculum-studio/cmd/openapi-gen/` (offline emitter stub path)
  - `curriculum-studio/internal/api/` (Huma authoring edge)
  - `curriculum-studio/internal/grpcapi/` (gRPC integration edge)
  - `curriculum-studio/clients/ts-rest/` (TS authoring client package)
  - `curriculum-studio/clients/go-rest/` (Go authoring client package)
  - `curriculum-studio/clients/go-grpc/` (Go gRPC client package)
  - gitignored `contracts/gen/`, `contracts/.tmp/`, `clients/*/generated/`
- **And** dependency direction forbids server importing client packages

#### Scenario: P1-S3 — Generated outputs are untracked by policy

- **Given** the contracts `.gitignore` and monorepo ignore rules
- **When** `git check-ignore` / CI scan runs on `gen/`, `.tmp/`, `*.pb.go`,
  client `generated/`
- **Then** those paths are ignored
- **And** a planted tracked file under `gen/` fails the policy script (proof may
  land fully in Phase 10; Phase 1 at least documents and wires the script stub)

#### Scenario: P1-S4 — Cross-plan interface appendix is published

- **Given** Identity, DB, LMS, and platform concerns
- **When** `docs` section in this phase’s package README is written
- **Then** required JWT claims, DB enum stability, LMS client-only rule, and
  platform binary hooks are listed as interfaces without taking ownership

## Implementation Instructions

1. **Read-only freeze citation**
   Link crosswalk L1–L6, product plan boundary, identity auth table, contracts
   README ownership table into `curriculum-studio/contracts/OWNERS.md` or extend
   README with an “Implementation ownership” section pointing at this plan.

2. **Layout skeleton (docs + empty keepers only)**
   Create directory placeholders with `README.md` describing purpose — do **not**
   implement handlers yet. Suggested tree:

   ```text
   curriculum-studio/
     README.md                         # update layout section
     contracts/                        # existing
     db/                               # existing — do not redesign
     cmd/
       openapi-gen/README.md           # offline emission entry (Phase 6)
       studio-api/README.md            # future binary — platform owns fullness
     internal/
       api/README.md                   # Huma authoring edge
       grpcapi/README.md               # gRPC integration edge
       boundary/README.md              # shared wire helpers (enum map, ids) — NOT a DTO catalog
       authn/README.md                 # JWT/JWKS validate adapter interface
     clients/
       ts-rest/README.md
       go-rest/README.md
       go-grpc/README.md
     tools/
       contract-gates/README.md        # parity, tracked-gen, raw-ban scripts
   ```

3. **Go module decision (FROZEN at integration)**
   Use `curriculum-studio/go.mod` module
   `github.com/aleksclark/primer/curriculum-studio` matching proto `go_package`.
   Do not reuse the LMS `server` module. Root `go.work` inclusion is owned by delivery wave **F0**.

4. **Build graph skeleton**
   Add `curriculum-studio/Makefile` or Task targets (names stable for later phases):

   - `contracts-validate` → existing `contracts/scripts/validate.sh`
   - `contracts-buf-generate` → `buf generate` (Phase 4 fills)
   - `contracts-openapi-emit` → openapi-gen (Phase 6)
   - `clients-generate` → fan-out
   - `contracts-parity` → Phase 2
   - `contracts-gates` → Phase 10

5. **Ignore rules**
   Confirm/extend:

   ```gitignore
   curriculum-studio/contracts/gen/
   curriculum-studio/contracts/.tmp/
   curriculum-studio/clients/**/generated/
   curriculum-studio/**/openapi.emitted.yaml
   ```

6. **Requirement ID registry**
   Keep IDs from index §8 stable; add `tools/contract-gates/requirements.txt`
   listing REQ-* for the validator in Phase 11.

7. **Do not** move LMS `web/openapi.yaml` generation or edit Identity design.

### Focused verification

- `test -d` for each reserved path
- `git check-ignore -v` on sample generated paths
- `cd curriculum-studio/contracts && ./scripts/validate.sh` still passes
- markdown links from index → this file resolve

## End-to-End Test Plan

| ID | Setup | Action | Assert |
| --- | --- | --- | --- |
| E1-01 | clean tree | run ownership scanner script (may be stub returning paths + forbidden OpenAPI schema names) | fails if `MaterializationContext` schema appears in OpenAPI components; passes on current baseline |
| E1-02 | layout committed | `go list` / filesystem test of module boundary docs | server packages do not import `clients/*`; README states direction |
| E1-03 | gitignore updated | `git check-ignore` on `contracts/gen/foo.go` | ignored |
| E1-04 | contracts | `./scripts/validate.sh` | exit 0 |

## Anti-Cheating Audit

- Layout READMEs without actual directory creation or ignore rules
- Claiming Huma emission exists when only LMS openapi-gen is present
- Putting DTOs under `internal/boundary` that duplicate proto messages field-for-field as a third catalog
- “Validating” ownership only by prose without the forbidden-schema scan
- Editing DB migrations or LMS OpenAPI under the guise of layout work

## Completion Gate

- [ ] P1-S1…P1-S4 scenarios pass with evidence
- [ ] E1-01…E1-04 recorded
- [ ] Directory skeleton + Makefile targets + gitignore present
- [ ] `contracts/scripts/validate.sh` green
- [ ] No production business logic added
- [ ] Index links resolve

## Dependencies and rollback

- **Depends on:** None
- **Unblocks:** Phases 2–4, 6
- **Rollback:** delete skeleton dirs/docs; restore gitignore; no data migration
