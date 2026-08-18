# Huma emission gap report (C6)

`cmd/openapi-gen` emits the authoring surface from `internal/api.NewWithPinger`
without binding a listener or opening PostgreSQL. The hand document
`curriculum-studio.yaml` is now a frozen compatibility baseline; generated REST
clients consume the emitted document.

## Current evidence

- Emitted paths: 38 (the baseline has 34; the emitted document includes Studio
  health/readiness/authz host hooks as well as the authoring paths).
- Emitted schemas: 75 after reference filtering.
- `contracts/.tmp/openapi.emitted.yaml` validates with
  `openapi-spec-validator` and passes `contracts/scripts/parity.sh` when passed
  as `OPENAPI_PATH`.
- The ownership scanner reports all authoring subset components and no
  `MaterializationContext`, `MaterializationBundle`, or `SessionSpec` schema.

## Known deltas carried into later compatibility work

1. C6 handlers return empty fixture values. Persistence, authorization policy,
   idempotency behavior, and domain workflow semantics remain platform/database
   work; the signatures and status/error boundary are the contract evidence.
2. Huma's standard problem envelope is augmented with the shared `ErrorCode`
   reference. Detailed problem field parity and the final generated REST client
   handoff are C7/C8 work.
3. The existing authz workspace probe is now registered at the canonical
   `/studio/v1/workspaces/{workspaceId}` path and uses the `Workspace` boundary
   shape; full workspace CRUD behavior remains S3-owned.
4. The emitted spec intentionally has no Primer-only integration payloads.
   Materialization context, bundle sessions, and event payload details remain
   protobuf-owned.

Do not replace the emitter with the hand YAML. Future baseline updates must
use the explicit bootstrap procedure and a reviewed compatibility diff.
