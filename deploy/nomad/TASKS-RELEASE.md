# Tasks P1/P2 production package

Tasks remains one standalone service and its own PostgreSQL database. This
package does not change LMS/TV/ingest jobs, the development/Compose path, Android,
or application authentication semantics. Application ownership is separate from
these packaging files.

## Current publication boundary

- `Dockerfile.tasks` builds the actual server, migration and operator bootstrap
  binaries plus the generated-client SPA. Context is the repository root, with
  the **Dockerfile-specific allowlist** `Dockerfile.tasks.dockerignore`.
- `.github/workflows/tasks-image.yml` is the Tasks-only GHCR build/publication
  lane. PRs build without publishing; master and an explicitly dispatched branch
  publish `ghcr.io/aleksclark/primer-tasks:sha-<full-source-SHA>`, provenance and
  SBOM. Other products' image jobs are not part of this lane.
- `jobs/primer-tasks.nomad.hcl` and `env/tasks-home.nomadvars.hcl` are **candidate
  source artifacts, not an enrolled or submitted workload**. The existing
  `deployment.yaml` and `images.lock.hcl` are deliberately unchanged. Once an
  approved build is published, add the real registry digest as
  `image_primer_tasks` in `images.tasks.lock.hcl` and enroll an isolated
  `primer-tasks` release set using only that lock and the Tasks overlay. Never
  substitute a local image ID, fabricated digest or tag for a registry digest.
- The current fleet project lane is observe-only and lacks an approved writer
  for this new job. Neither image publication nor this candidate jobspec grants
  deployment authority. Resolve that with the fleet owner, do not bypass it.
  Do not run `deploy/deploy.sh`, submit Nomad directly, change tunnel/DNS, or
  silently flip `allow_create`/ownership.

## Build contract

Requirements: Docker Buildx, approved public Clerk profile for
`https://api.primerlms.com`, and the frozen combined application/packaging commit.
Go is pinned to **1.26.6** because the approved Authstack module requires it.
`GOWORK=off` prevents unrelated local modules from entering the build;
`GOPRIVATE=git.fleet.clark.team/aleksclark/authstack` uses the application-owned,
same-version public-HTTPS transport replacement. No private build credentials,
vendor tree or test issuer are included.

| Build input | Contract |
|---|---|
| `VITE_CLERK_PUBLISHABLE_KEY` | Required approved public browser key; repository Actions **variable** of this name for CI, environment input for local builds |
| `VITE_TASKS_BASE_PATH` | `/tasks/` |
| `VITE_TASKS_API_BASE` | `/tasks/api` |
| `SOURCE_REVISION` | Full Git commit SHA, recorded as OCI revision label |

A missing publishable key is a **failed production build**, not a fallback to a
test issuer, invented key or login-disabled release. The previously discovered
Authstack profile is a test instance and is not implicitly approved for Tasks.
Never read/copy `CLERK_SECRET_KEY`: neither build nor runtime needs it. Supply only
the approved public fields without echoing environment/profile values to logs.

After the combined commit is clean and approved public key is injected:

```sh
./scripts/build-tasks-image.sh
```

This command builds/loads **locally only**, writes Buildx metadata under
`tmp/tasks-image/`, and reports an image ID (not a registry digest). It never
publishes or deploys. Public-base pulls may use an isolated empty `DOCKER_CONFIG`
if a stale local credential helper interferes; never alter global auth config.
Use the Tasks workflow for approved publication. Its artifact
`tasks-image-<full-source-SHA>/tasks-image.txt` records the source SHA, immutable
registry selector and trace tag. Check the workflow result, digest and
provenance before locking it.

## Runtime and routing contract

| Item | Contract |
|---|---|
| Public entry | `https://api.primerlms.com/tasks/` |
| Service / job | `primer-tasks` / `primer-tasks` |
| Container port | `8080`, host port dynamically allocated by Nomad |
| Service health | Unauthenticated `GET /health` on the service port |
| Public API health | `GET /tasks/api/health` |
| API mapping | `/tasks/api/students` → native `/students`; `/tasks/api/tasks` → native `/tasks`; strip exactly once **in the app** |
| SPA | `/app/web`, served by `/app/tasks-server`; `/tasks` redirects 308 to `/tasks/` |
| Router | `Host(api.primerlms.com) && (Path(/tasks) || PathPrefix(/tasks/))`, explicit priority 200 |

The tunnel preserves `Host: api.primerlms.com`. Forward paths unchanged. Do not
add strip-prefix middleware, capture `/tasks-other`, or modify existing LMS
host/catch-all or `/api/v1` routers. Do not create a separate UI service or use the
optional `app.primerlms.com` route. No WebSocket upgrade requirement exists for
this release. Parent Clerk modal return is
`https://api.primerlms.com/tasks/parent/students`; the application owns exact
origin/azp and session validation. Student opaque cookie/device credentials are
unchanged (`tasks_student` remains Path=/, Secure in production, HttpOnly,
SameSite=Lax); no cross-product/shared parent cookie is introduced.

The image is non-root UID/GID 65532, and both Nomad tasks use a read-only root
filesystem with all capabilities dropped and no-new-privileges. IANA tzdata is
included for household local-day scheduling. SIGTERM has a 10-second shutdown
budget. No data volume is required by this P1/P2 image.

### Configuration names and delivery

Non-secret runtime settings declared by the Tasks manifest:
`TASKS_ENV=production`, `TASKS_AUTH_MODE=clerk`, `TASKS_HOST=0.0.0.0`,
`TASKS_PORT=8080`, `TASKS_BASE_PATH=/tasks`, `TASKS_WEB_DIR=/app/web`, and
`TASKS_PUBLIC_ORIGIN=https://api.primerlms.com`.

Proposed Nomad Variable path: **`nomad/jobs/primer-tasks`** (provisioning must be
confirmed separately; do not infer it from these declarations).

| Nomad key | Injected environment name |
|---|---|
| `tasks_database_url` | `TASKS_DATABASE_URL` |
| `tasks_clerk_issuer` | `TASKS_CLERK_ISSUER` |
| `tasks_clerk_jwks_url` | `TASKS_CLERK_JWKS_URL` |

`TASKS_CLERK_AUDIENCE` is optional **only if the approved actual session profile
carries an aud claim**. It is intentionally not invented in the manifest. If
required, agree the profile with the application owner and add the corresponding
key/template explicitly. Exact authorized party must match the public origin.
No Clerk secret key, Authstack service token, Identity/password, parent shared
key or session secret is needed. Keep values out of Git, plan/log output and
operator screenshots.

## Migrations and first parent bootstrap

Production server startup does not migrate or seed. A non-sidecar **prestart**
Nomad task runs the same image with entrypoint `/app/tasks-migrate`, **no args**,
and **only `TASKS_DATABASE_URL`** injected. The server cannot start when migration
fails. The explicit `entrypoint` override is required: Nomad Docker `command`
would append to the image's server ENTRYPOINT, not replace it.

An approved operator can run the equivalent container command after securely
injecting the Tasks-only DSN and choosing a verified published digest:

```sh
# Documentation only: production execution requires the approved writer/operator.
docker run --rm --read-only --user 65532:65532 --cap-drop ALL \
  --security-opt no-new-privileges --env TASKS_DATABASE_URL \
  --entrypoint /app/tasks-migrate "$IMAGE"
```

Never point this at an LMS/TV database. Apply migrations before bootstrap.
Bootstrap is operator-only, no public endpoint or automatic enrollment:

```text
/app/tasks-bootstrap --issuer <approved-exact-issuer> \
  --subject <approved-Clerk-user-subject> \
  --tenant-id <existing-local-household-UUID> --actor-ref <existing-subject_ref>
```

It uses `TASKS_DATABASE_URL` only. Creating a new household additionally requires
`--create-household --household-name <name>` and explicit UUID/actor-ref. Same
mapping is idempotent; rebinding/reactivating revoked identities is refused.
Bootstrap authority and subject/household mapping must be supplied by the owner,
not inferred from an email or a test fixture. Do not place subject/identity
bootstrap flags in the routine service jobspec.

## Checks and handoff

```sh
python3 deploy/nomad/tests/tasks-contract.py
bash -n scripts/build-tasks-image.sh
nomad fmt -check deploy/nomad/jobs/primer-tasks.nomad.hcl
# Only after the real registry lock exists:
nomad job validate \
  -var-file=deploy/nomad/images.tasks.lock.hcl \
  -var-file=deploy/nomad/env/tasks-home.nomadvars.hcl \
  deploy/nomad/jobs/primer-tasks.nomad.hcl
```

Do not report static checks or an intermediate builder stage as a completed
production image. Before enrollment, prove the combined immutable image with a
disposable Tasks PostgreSQL: migration/re-run, non-root/read-only server health,
real SPA/assets under `/tasks/`, API 401 without parent auth, and clean SIGTERM.
Then the approved operator verifies the real public login/bootstrap, student
pairing and one manual task completion. Existing internal LMS health is the
non-regression control; public `/api/v1/health` is not a Tasks endpoint and may
still 404. No fixture issuer may be called public release evidence.

Rollback the service to a prior **compatible immutable digest** via the approved
writer. Image auto-revert does not roll back migrations or parent mappings. Keep
a pre-migration Tasks database backup and review schema compatibility; if schema
rollback is required, use an explicit owner-approved Tasks-only restore rather
than an improvised down migration. There is no claim of a prior production Tasks
release until one actually exists.
