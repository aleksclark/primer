# Curriculum Studio LikeC4 architecture

Maintainable multi-file [LikeC4](https://likec4.dev) workspace for
**Curriculum Studio** as a standalone product and independently
deployable modular service.

Workspace path: `architecture/curriculum-studio/`

## Authoritative sources

| Document | Role |
| --- | --- |
| `agent_docs/plans/primer-curriculum-studio-product-plan.md` | Product boundary, modules, Primer contract, events |
| `AGENTS.md` | Existing LMS/TV independently deployed services, separate PostgreSQL, HTTP + directional secrets, no shared DB |

LMS/TV code was inspected only to name real interface shapes already
in the repo (LMS `POST /auth/login` bearer sessions; LMS
`POST /curriculum/import/{plan,apply}`; TV→LMS
`POST /instruction-logs/ingest` with `X-Service-Token`). Those shapes
inform the **integration boundary**, not Studio internals.

Do not treat this model as an auth design decision. The proposed
identity/auth service is modeled as an **external/adjacent** system
with `#uncertainty` on every auth edge.

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
  README.md
```

LikeC4 recursively merges `*.c4` in this directory.

## Decisions recorded in the model

1. **Studio is its own product.** Primer is a consumer, not the owner
   of authoring. Studio has a complete
   `plan → validate → materialize → edit → publish → export` loop.
2. **One deployable service + modules.** Internal plan / standards /
   resources / validation / materialization / agent runner / export /
   integration-gateway modules are components, not a service fleet.
3. **Browser UI is owned by Studio** but executes outside the Go
   process. It is not a peer `softwareSystem`.
4. **Separate PostgreSQL and artifact/object store.** No direct
   cross-database access in either direction.
5. **Sync + async integration.** Primer → Studio HTTPS materialization
   API, and Studio → Primer domain events/webhooks. Both are required
   by the plan; events are optional for Primer.
6. **Proposed identity/auth service is adjacent and unsettled.**
   Current LMS authenticates parents in-process. Studio standalone use
   cannot require live LMS sessions. Protocol, account store, and LMS
   migration are open.
7. **Existing LMS import is not silently the Phase 3 adapter.**
   `POST /curriculum/import/{plan,apply}` is parent-session guarded
   today. A Studio→LMS bundle push is tagged `#uncertainty`.

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
| `studio_system_context` | Authors, Studio, proposed IdP, model providers, Primer |
| `studio_containers` | Studio UI, service modules, DB, artifact store |
| `primer_integration` | Sync materialization API + async events |
| `data_ownership_deployment` | Separate DBs, no cross-DB edges |
| `auth_trust` | Proposed identity service and trust boundaries |

LikeC4 may also emit a generated `index` view.

## Correctness-critical edges to re-check after edits

After `export json`, inspect the named view's `edges` (source includes
are not proof):

- `primer_lms` → `curriculum_studio` (context)
- `primer_lms.lms_api` → `curriculum_studio.studio` (sync API)
- `curriculum_studio.studio` → `primer_lms.lms_api` (events/callbacks)
- `curriculum_studio.studio` → `curriculum_studio.postgres`
- **Absence** of `curriculum_studio.*` → `primer_lms.lms_postgres`
- **Absence** of `primer_lms.*` → `curriculum_studio.postgres`

## Out of scope

- Primer TV channel, content-ingest, and student TUI internals
- Splitting Studio modules into separately deployed services
- Choosing OIDC vs session tokens vs LMS-owned auth
- Committing generated LikeC4 site/JSON
