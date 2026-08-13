# Phase 4: Protobuf Buf generation and client packages

## Goal

Make protobuf generation deterministic and package-ready: `buf lint/build/generate`
produces uncommitted Go (and optional TS) stubs; a dedicated Go gRPC **client
package** façade consumes them; clean checkout works offline aside from pinned
plugin fetch policy documented for CI.

**Depends on:** Phases 1–3 (Phase 3 PROCEED for gRPC stacks)
**Duration guess:** 2–3 days

## BDD Success Criteria

#### Scenario: P4-S1 — Buf pipeline validates and builds images

- **Given** `curriculum-studio/contracts` module
- **When** `./scripts/validate.sh` and `buf build` run
- **Then** lint/format/build succeed and descriptor/image land under `.tmp/`
  (gitignored)

#### Scenario: P4-S2 — Go gRPC client package builds from generated stubs only

- **Given** `buf generate` output under `contracts/gen/go` (or client-local
  generated path)
- **When** `clients/go-grpc` builds
- **Then** public API imports generated types/services without copying messages
- **And** façade may set dial options, auth metadata injector, and timeouts only

#### Scenario: P4-S3 — Generation is deterministic

- **Given** clean gen directories
- **When** generate runs twice with same pins
- **Then** normalized digests match (timestamps stripped if any)

#### Scenario: P4-S4 — Generated sources remain untracked

- **Given** a successful generate
- **When** `git status` / tracked-gen scanner runs
- **Then** no `*.pb.go` or gen paths are staged/tracked

#### Scenario: P4-S5 — Integration service descriptors include all RPCs

- **Given** `CurriculumIntegrationService` in integration.proto
- **When** generated service client is inspected
- **Then** Materialize, GetMaterialization, GetMaterializationBundle,
  GetPublishedRevision, GetPlanGraph, ValidateRevision, standards/resource
  reads, ListMaterializedItems, PullEvents, AcknowledgeEvent are present

## Implementation Instructions

1. **Confirm pins** in `buf.gen.yaml` (already: protoc-gen-go v1.36.11,
   grpc-go v1.5.1; Buf CLI 1.72.x). Prefer remote plugins in CI; document local
   `protoc-gen-go` fallback already proven in README comments.

2. **Output layout options (pick one, document):**
   - A: `contracts/gen/go/...` gitignored; client façade in
     `clients/go-grpc` imports that path via go.mod replace
   - B: generate directly into `clients/go-grpc/generated/` gitignored
   Prefer A to match proto `go_package`.

3. **Client façade** `clients/go-grpc`:
   - `NewClient(conn) *Client`
   - methods wrapping integration RPCs
   - `WithBearerToken(token string)` unary interceptor helper
   - no hand-written message structs

4. **Optional TS gRPC:** only if spike PROCEED; otherwise document deferred.

5. **Makefile:** `contracts-buf-generate`, `clients-go-grpc-build`.

6. **CI job** (or extend existing): validate + generate + `go test` client
   package compile.

7. Do not implement server yet (Phase 5).

### Focused verification

- `buf generate` twice + digest
- `go test ./clients/go-grpc/...`
- `git status --porcelain` clean of gen if gen deleted after compile (or ignore)

## End-to-End Test Plan

| ID | Setup | Action | Assert |
| --- | --- | --- | --- |
| E4-01 | contracts tree | validate.sh + buf generate | exit 0 |
| E4-02 | generated stubs | build go-grpc client | success; symbols exist for Materialize |
| E4-03 | two generates | digest compare | equal |
| E4-04 | after generate | tracked file scan | no gen tracked |
| E4-05 | proto RPC list | reflection or generated interface | all RPCs present |

## Anti-Cheating Audit

- Committing pb.go “temporarily”
- Façade redefining messages
- Generate from non-module copy of protos
- Skipping determinism because remote plugins differ — pin digests
- Claiming client works without compilation

## Completion Gate

- [ ] P4-S1…P4-S5 pass
- [ ] E4-01…E4-05 evidence
- [ ] Pins locked; README generate instructions updated
- [ ] Client package importable by future LMS adapter tests

## Dependencies and rollback

- **Depends on:** Phase 1–3
- **Unblocks:** Phase 5, 10
- **Rollback:** delete client package; keep protos; clean gen/
