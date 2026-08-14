# internal/api

Huma authoring edge for Curriculum Studio.

**Owns:** HTTP handler registration, request/response boundary DTOs used only at
the authoring REST edge, problem+json error mapping for browser clients.

**S1 surface:** `GET /studio/v1/health` (liveness), `GET /studio/v1/ready`
(real DB ping), `GET /metrics` and `GET /studio/v1/metrics` (request counter),
request-id middleware, structured access logs (method/path/status/duration_ms/
request_id only — no query/body/auth/DSN).

**Does not own:** Primer integration payloads (`MaterializationContext` full
tree), gRPC services, generated client packages, DB schema, domain CRUD, authz.

**Import rule:** must not import `curriculum-studio/clients/*`.
