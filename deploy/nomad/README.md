# Primer Nomad deployment contract (plan-03)

Source-only project contract for the **primer** release set. This tree is the
canonical declaration for fleet pull-reconciler enrollment. It does **not**
submit jobs to Nomad from CI or `deploy/deploy.sh`.

## Layout

| Path | Role |
|---|---|
| `deployment.yaml` | Plan-03 schema v1 manifest (one release set) |
| `jobs/*.nomad.hcl` | Portable jobspecs (one top-level job each) |
| `env/home.nomadvars.hcl` | Non-secret home-fleet overlay |
| `images.lock.hcl` | Immutable `registry@sha256:` image authority |
| `tests/contract.sh` | Static contract tests (no secrets, no Nomad token) |
| `tests/expected-services.json` | Service/check expectations |

Legacy `deploy/*.nomad.hcl.tmpl` remain during dual-source migration (S0/S1) and
must not be deleted until S3. `deploy/deploy.sh` refuses production submit.

## Release set

| Job ID | Type | Notes |
|---|---|---|
| `primer` | service | LMS API + SPA |
| `primer-tv` | service | TV channel API + admin |
| `content-ingest` | periodic batch | Shared media writer; `prohibit_overlap=true` |

Rollout: **serial**. Prune: **explicit-only** (removing a manifest row never
stops a live job; retirement needs a fleet tombstone).

## Images

Production selectors are digest-only variables in `images.lock.hcl`:

- `image_primer`
- `image_primer_tv`
- `image_content_ingest`

Each value must match `^.+@sha256:[0-9a-f]{64}$`. Floating tags and `:latest`
are rejected by contract tests. Trace tags (e.g. `6cd71b3`, `f0ab751`) are
comments only.

## Secrets (Nomad Variables — key names only)

Create/update these paths out-of-band. Values never belong in git, shell
`envsubst`, plan output, or CI logs.

### `nomad/jobs/primer`

| Key | Env injected |
|---|---|
| `database_url` | `DATABASE_URL` |
| `service_token` | `SERVICE_TOKEN` |

### `nomad/jobs/primer-tv`

| Key | Env injected |
|---|---|
| `tv_database_url` | `TV_DATABASE_URL` |
| `tv_jellyfin_api_key` | `TV_JELLYFIN_API_KEY` |
| `tv_jellyfin_user_id` | `TV_JELLYFIN_USER_ID` |
| `tv_admin_api_key` | `TV_ADMIN_API_KEY` |
| `tv_primer_service_token` | `TV_PRIMER_SERVICE_TOKEN` |

### `nomad/jobs/content-ingest`

| Key | Env injected |
|---|---|
| `ingest_radarr_api_key` | `INGEST_RADARR_API_KEY` |
| `ingest_sonarr_api_key` | `INGEST_SONARR_API_KEY` |
| `ingest_jellyfin_api_key` | `INGEST_JELLYFIN_API_KEY` |
| `ingest_jellyfin_user_id` | `INGEST_JELLYFIN_USER_ID` |
| `ingest_tv_admin_key` | `INGEST_TV_ADMIN_KEY` |

## content-ingest handoff (do not skip)

`content-ingest` is a **periodic writer** against shared MooseFS media and
Radarr/Sonarr/Jellyfin/TV admin APIs.

Before S2 authority flip:

1. **Pause** the live periodic scheduler (or choose a proven quiet window).
2. Prove **no child** `content-ingest/periodic-*` is running.
3. Flip fleet registry authority to the project commit SHA.
4. Reconcile once; do **not** CI-dispatch or `nomad job dispatch` / force run.
5. Allow one controlled scheduled window; verify single child + no duplicate
   downloads/API mutations; confirm `prohibit_overlap` remains true.

This source-only PR does **not** pause live content-ingest.

## Health / routes

| Job | Service | Check | Hosts |
|---|---|---|---|
| primer | `primer` | `GET /api/v1/health` | `primer.fleet.clark.team`, `primer.clark.team` |
| primer-tv | `primer-tv` | `GET /api/v1/health` | `tv.fleet.clark.team`, `tv.clark.team` |
| content-ingest | (batch) | periodic child success | n/a |

## Local contract checks

```bash
./deploy/nomad/tests/contract.sh
```

Optional L0 when `nomad` is installed:

```bash
nomad fmt -check deploy/nomad/jobs
nomad job validate \
  -var-file=deploy/nomad/images.lock.hcl \
  -var-file=deploy/nomad/env/home.nomadvars.hcl \
  deploy/nomad/jobs/primer.nomad.hcl
# repeat for primer-tv and content-ingest
```

## Production writer

The only production writer after enrollment is the **fleet pull reconciler**.

- `deploy/deploy.sh` refuses `nomad job run` / production submit.
- CI must never dispatch `content-ingest`.
- Manual CAS is an emergency break-glass only, with reviewed plan evidence.

## Ownership

- Application + this contract: **project-owned** (`aleksclark/primer`)
- Owner identity: `aleks-clark` (manifest) / `@aleksclark` (CODEOWNERS)
- Fleet registry source id: `project-primer` (observe mode until contract lands)
