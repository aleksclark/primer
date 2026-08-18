# internal/api

Huma authoring edge for Curriculum Studio.

**Owns:** HTTP handler registration, request/response boundary DTOs used only at
the authoring REST edge, problem+json error mapping for browser clients.

**S1/S2 surface:** `GET /studio/v1/health` (liveness),
`GET /studio/v1/ready` (real DB ping), `GET /metrics` and
`GET /studio/v1/metrics` (request counter), request-id middleware, structured
access logs (method/path/status/duration_ms/request_id only — no
query/body/auth/DSN), and signed Bearer JWT validation plus local membership
RBAC for `/studio/v1/auth/me` and authorization probes.

Protected requests accept only `Authorization: Bearer <Primer JWT>` with
`aud=curriculum-studio`; the validator requires `at+jwt` ES256, configured
issuer, bounded `iat`/`nbf`/`exp`, known JWKS `kid`, and a signed public
`client_id`. `azp`, provider credentials, and caller identity headers are not
accepted. Studio never issues tokens or sessions.

BFF login, session, cookie, and CSRF routes are intentionally absent from this
package; that boundary belongs to I7.

**Does not own:** Primer integration payloads (`MaterializationContext` full
tree), gRPC services, generated client packages, DB schema, domain CRUD, or
BFF session/login.

**Import rule:** must not import `curriculum-studio/clients/*`.
