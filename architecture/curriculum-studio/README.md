# Curriculum Studio LikeC4 architecture

Maintainable multi-file [LikeC4](https://likec4.dev) workspace for
**Curriculum Studio** as a standalone product and independently
deployable modular service.

Workspace path: `architecture/curriculum-studio/`

## Authoritative sources

| Document | Role |
| --- | --- |
| `agent_docs/plans/primer-curriculum-studio-product-plan.md` | Product boundary, modules, Primer contract, events |
| `agent_docs/plans/curriculum-studio-foundation-crosswalk.md` | **Authoritative** vocabulary, ownership, auth, enum crosswalk |
| `agent_docs/plans/primer-identity-service-design.md` | Decided Primer Identity boundaries and JWT/BFF mechanics |
| `agent_docs/plans/curriculum-studio-mcp-design.md` | Streamable HTTP MCP endpoint decisions |
| `curriculum-studio/contracts/` | OpenAPI + protobuf wire contracts (+ MCP third-surface policy) |
| `curriculum-studio/db/` | Standalone Studio PostgreSQL schema |
| `AGENTS.md` | Existing LMS/TV independently deployed services, separate PostgreSQL |

LMS/TV code was inspected only to name real interface shapes already
in the repo (LMS `POST /auth/login` bearer sessions; LMS
`POST /curriculum/import/{plan,apply}`; TV→LMS
`POST /instruction-logs/ingest` with `X-Service-Token`). Those shapes
inform the **integration boundary**, not Studio internals.

## Layout

```text
architecture/curriculum-studio/
  specification.c4
  landscape.c4
  infrastructure.c4
  services/studio.c4
  services/primer.c4
  relationships/actors-auth.c4
  relationships/studio-internals.c4
  relationships/primer-integration.c4
  views/system-context.c4
  views/studio-containers.c4
  views/primer-integration.c4
  views/data-ownership.c4
  views/auth-trust.c4
  views/studio-mcp.c4
  README.md
```

LikeC4 recursively merges `*.c4` in this directory.

## Decisions recorded in the model

1. **Studio is its own product.** Primer is a consumer, not the owner
   of authoring. Studio has a complete
   `plan → validate → materialize → edit → publish → export` loop.
2. **One deployable service + modules.** Internal plan / standards /
   resources / validation / materialization / agent runner / export /
   integration-gateway / **MCP adapter** modules are components, not a service fleet.
3. **Browser UI is owned by Studio** but executes outside the Go
   process. It is not a peer `softwareSystem`.
4. **Separate PostgreSQL and artifact/object store.** No direct
   cross-database access in either direction.
5. **Sync + async integration.** Primer → Studio HTTPS/gRPC
   materialization API, and Studio → Primer domain events/webhooks.
   Both are required by the plan; events are optional for Primer.
6. **Primer Identity is decided and adjacent.** Google OIDC, host-only
   BFF cookies, single-audience JWTs + JWKS. Studio API validates JWKS
   only; product authorization stays in Studio. See crosswalk L3–L4.
7. **Existing LMS import is not silently the Phase 3 adapter.**
   `POST /curriculum/import/{plan,apply}` is parent-session guarded
   today. A Studio→LMS bundle push remains `#uncertainty` / deferred.
8. **Contracts live under the Studio service tree** at
   `curriculum-studio/contracts/` (OpenAPI authoring vs protobuf
   integration ownership split).
9. **MCP Streamable HTTP** is a same-deployable agent surface at `/mcp`
   (crosswalk L7). External curriculum-planning agents obtain tokens
   from Identity and call `mcp_adapter`; tools use domain modules;
   Studio Postgres only; no third service/DB.

## Pinned CLI

Use **LikeC4 CLI `1.46.0`** for validate / build / export (repo-proven
pin from other Nous architecture workspaces). Do not add a package
manifest solely to run these commands.

```bash
# from repository root
npx --yes likec4@1.46.0 validate architecture/curriculum-studio
npx --yes likec4@1.46.0 build architecture/curriculum-studio -o /tmp/curriculum-studio-site
npx --yes likec4@1.46.0 export json architecture/curriculum-studio -o /tmp/curriculum-studio-model.json
```

Generated `dist/`, static sites, and JSON exports stay **untracked**.
Write them under `/tmp` (as above) or a local ignore; do not commit
them.

## Authored views

| View ID | Concern |
| --- | --- |
| `studio_system_context` | Authors, Studio, Identity, model providers, Primer, planning agent |
| `studio_containers` | Studio UI, service modules (incl. MCP adapter), DB, artifact store |
| `primer_integration` | Sync materialization API + async events |
| `data_ownership_deployment` | Separate DBs, no cross-DB edges |
| `auth_trust` | Decided Identity trust boundaries (JWKS, BFF, MCP, no shared DB) |
| `studio_mcp` | Focused MCP agent → Identity → mcp_adapter → domain → Studio DB |

LikeC4 may also emit a generated `index` view.

## Correctness-critical edges to re-check after edits

After `export json`, inspect the named view's `edges` (source includes
are not proof):

- `primer_lms` → `curriculum_studio` (context)
- `primer_lms.lms_api` → `curriculum_studio.studio` (sync API)
- `curriculum_studio.studio` → `primer_lms.lms_api` (events/callbacks)
- `curriculum_studio.studio` → `curriculum_studio.postgres`
- `curriculum_studio.studio.curriculum_api` → `identity_service` (JWKS)
- `curriculum_studio.studio.mcp_adapter` → `identity_service` (JWKS)
- `curriculum_planning_agent` → `curriculum_studio.studio.mcp_adapter`
- `curriculum_studio.studio_ui` → `identity_service` (OIDC/BFF)
- **Absence** of `curriculum_studio.*` → `primer_lms.lms_postgres`
- **Absence** of `primer_lms.*` → `curriculum_studio.postgres`
- **Absence** of MCP path → LMS Postgres (positive isolation)

## Out of scope

- Primer TV channel, content-ingest, and student TUI internals
- Splitting Studio modules into separately deployed services
- Committing generated LikeC4 site/JSON
- Implementing Identity or Studio service code
