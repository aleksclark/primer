# internal/api

Huma authoring edge for Curriculum Studio.

**Owns:** HTTP handler registration, request/response boundary DTOs used only at
the authoring REST edge, problem+json error mapping for browser clients.

**Does not own:** Primer integration payloads (`MaterializationContext` full
tree), gRPC services, generated client packages, DB schema.

**Import rule:** must not import `curriculum-studio/clients/*`.
