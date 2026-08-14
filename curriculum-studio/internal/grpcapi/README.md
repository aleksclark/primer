# internal/grpcapi

gRPC integration edge for `CurriculumIntegrationService`.

**Owns:** service registration, adapter stubs/harnesses for contract E2E,
metadata auth attachment points.

**Does not own:** full materializer agents, LMS runtime, authoring draft graph UI.

**Import rule:** must not import `curriculum-studio/clients/*`. Generated gRPC
stubs live under build-only `contracts/gen/` and are consumed here at compile time.
