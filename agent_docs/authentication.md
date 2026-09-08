# Current authentication boundaries

This is a **source-level inventory**, reconciled against `80238144` (2026-09-08),
not live deployment certification or a declaration that every migration gate is
complete. When changing a boundary, update this reference with the implementation
and its tests. Never copy credentials between products.

## Human/admin and agent boundaries

| Boundary | Implemented credential path | Authorization / limits | Source |
| --- | --- | --- | --- |
| LMS parent-guarded routes | Opaque LMS parent Bearer session from `/auth/login`, or Primer Identity access JWT when its verifier is configured | Both resolve an LMS educator and require local `parent`/`admin`. Current guard is single-family: authorized parents/admins can manage all students. This is not a guarantee that every generic CRUD route uses that guard. | [auth_parent.go](../server/internal/api/auth_parent.go), [API registration](../server/internal/api/api.go) |
| TV admin | Primer Identity JWT Bearer, or `X-Admin-Key` / opaque Bearer shared key | A correctly scoped-to-audience Identity JWT is accepted without a TV-local human role lookup. This is a current limitation, not completed multi-tenant admin authorization. | [TV auth.go](../server/internal/tv/api/auth.go) |
| Studio REST / BFF / MCP | Primer Identity JWT/JWKS; product BFF bridges its session to the authoring API | Studio-local memberships/roles and tool policy decide access. Studio is not a token issuer. Test fixtures do not establish production BFF readiness; BFF storage and full product cutover remain separate acceptance concerns. | [REST auth](../curriculum-studio/internal/api/auth.go), [BFF](../curriculum-studio/internal/bff/), [MCP](../curriculum-studio/internal/mcp/) |
| Studio gRPC | JWT metadata interceptor in the gRPC adapter | Do not infer complete full-method authorization or LMS production integration from the adapter/harness alone. | [gRPC server](../curriculum-studio/internal/grpcapi/server.go) |
| Primer Agents | Primer Identity JWT/JWKS | Route scopes, owner namespace, and profile admission are separate checks. | [authn](../primer-agents/internal/authn/), [API](../primer-agents/internal/api/), [profiles](../primer-agents/internal/profile/) |
| Tasks parent web / native Control | Clerk session JWT verified through authstack; production requires `TASKS_AUTH_MODE=clerk` | Exact issuer/subject maps to active Tasks-local parent identity/admin membership and local session revocation. Household is not taken from Clerk organization, role, email, or request data. | [config](../primer-tasks/internal/config/config.go), [Clerk adapter](../primer-tasks/internal/parentauth/clerk.go), [parent guard](../primer-tasks/internal/api/clerk.go) |

The LMS SPA currently stores its Bearer in browser localStorage; the TV SPA
stores the admin key there. Those legacy browser patterns are **not** the desired
BFF end state. See [LMS browser auth](../web/README.md#parent-session-student-client-section)
and [TV browser auth](../tv-web/README.md#admin-authentication).

Tasks' Clerk path is not the earlier Identity authorization-code/BFF design.
Legacy Tasks `test` / `oidc` modes remain development/test compatibility only.
The real Clerk web and native SDK flows require their own acceptance evidence;
a local test issuer is not Clerk.

### Tasks authorized-party behavior

`TASKS_PUBLIC_ORIGIN` remains required. `TASKS_CLERK_AUTHORIZED_PARTIES` adds
allowed origins/native application IDs rather than replacing the web origin.
The current adapter keeps that allowlist when a nonempty `azp` is present and
omits the party check when it is absent/empty (native Clerk session fetches may
omit it). Signature, issuer, expiry, human-session credential, session ID, and
local membership checks still apply. A present mismatched party is rejected.
`TASKS_CLERK_AUDIENCE` is optional and must match a real emitted audience if set.
Do not describe every native session as carrying an Android package `azp`.

## Service-to-service credentials

| Direction | Implemented configuration / presentation | Important distinction |
| --- | --- | --- |
| TV reporter → LMS instruction ingest | LMS `SERVICE_TOKEN` = TV `TV_PRIMER_SERVICE_TOKEN`; `X-Service-Token` | LMS ingest rejects all requests when its secret is unset. |
| LMS / content-ingest → TV admin | TV `TV_ADMIN_API_KEY`; `X-Admin-Key` (opaque Bearer is also accepted) | Separate from the TV→LMS secret; not a device token. |
| LMS → Primer Agents | `PRIMER_AGENTS_ENABLED`, base URL, and a short-lived Identity JWT selected by `PRIMER_AGENTS_TOKEN_ENV_VAR` | No automatic M2M acquisition or cross-provider migration is implied; see the [cutover runbook](runbooks/phase7-cutover.md). |
| Studio REST migration alias | JWT in `X-Service-Token`, only when explicitly enabled | Despite the header name, this is a JWT alias, not the LMS ingest shared secret. |

## Empty configuration is boundary-specific

- Generic [`SharedSecretGuard`](../server/internal/api/secret.go) remains inert
  when its secret is empty. **Do not generalize that behavior to every route.**
- LMS [`POST /instruction-logs/ingest`](../server/internal/api/instruction_logs.go)
  uses `FailClosedSharedSecretGuard`: no `SERVICE_TOKEN` means **401**, not open
  local ingest. OpenAPI generation does not execute the middleware and needs no
  live secret.
- TV admin is inert only when **both** its Identity verifier and admin key are
  absent. With either path configured, invalid/anonymous credentials are denied;
  an invalid JWT cannot fall back to shared-key success. Configure authentication
  before exposing TV admin; do not rely on the startup warning as protection.
- An unconfigured LMS Identity verifier rejects the JWT path; it does not disable
  the independent local parent-session guard or login path.

## Device and student credentials

LMS workstation, TV, and Tasks each own their pairing codes, opaque credentials,
student/device bindings, revocation, and re-pairing. They do not share identity
stores or inherit authorization from a parent JWT. The workstation broker holds
the LMS device token outside the unprivileged TUI.

Tasks keeps student browser cookies, `/device/` native Bearers, and
`/management-device/` management credentials on their respective boundaries;
none is a Clerk parent credential. The native Student application is documented
in the [Android guide](../android/README.md). Device isolation must be tested
independently of a human-auth migration.

## Migration and evidence precedence

The [authstack migration plan](plans/authstack-migration/index.md) remains a
future ecosystem migration with dual-run, linking, and rollback obligations.
Its pre-Tasks inventory is historical. The [Tasks early-release amendment](plans/tasks-early-release-clerk.md)
records the narrower implemented cutover; its original P1/P2 milestone wording
is also not a current feature inventory.

Primer Identity remains implemented and consumed by LMS, TV, Studio, and Agents.
Keep its [broker contract](plans/stytch-identity-ib0/index.md), state, and recovery
obligations until consumer cutover and rollback-window closure are actually
verified. Selecting Clerk does not authorize deleting Identity/Stytch, accepting
raw Stytch tokens in products, or changing product-local device auth.

For any acceptance claim, record the exact source SHA, credential class,
environment, command/journey, and result. A test called “live,” a mounted route,
or a merged phase title is not itself provider/browser/production evidence.
