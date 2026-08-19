# Stacklane-compatible Docker Compose DEV stack

Opt-in parallel-worktree local stack: Postgres (LMS + TV databases), LMS API
(Air), TV API (Air), LMS Vite, TV Vite, and investor Vite. Published only on
`127.0.0.1::<containerPort>` (ephemeral host ports) with Stacklane labels for
stable FQDNs when the Stacklane daemon is installed.

**Host paths are unchanged:** `make dev-db`, `make test`, and
`make investor-web-dev` remain the non-Docker workflow. Compose is **not** wired
into those Make targets.

**No provider credentials** in this foundation stack. Identity JWT, Jellyfin,
Bedrock, and primer-agents stay unset / disabled.

## Prerequisites

- Docker Engine + Compose v2
- Optional: Stacklane daemon for `*.test` DNS + VIP proxy
  (`web` public **3000** → Vite 5173, `api` **8080**, `tv-web` **3001**,
  `tv` **8081**, `investor` **3002**, `postgres` **5432**). Unprivileged
  `stacklane serve` cannot bind privileged port 80, so no public port is 80.
- Stacklane install/daemon is **separate**. Direct loopback ephemeral ports work
  if the daemon is absent (`stacklane: BLOCKED` in status is OK).

## Copy-paste commands

```bash
bash scripts/compose-dev.sh check
bash scripts/compose-dev.sh up
bash scripts/compose-dev.sh status
bash scripts/compose-dev.sh logs
bash scripts/compose-dev.sh down
CONFIRM=primer-<instance>-destroy bash scripts/compose-dev.sh destroy
```

Foreground logs (starts stack if needed; **Ctrl-C leaves the stack up**):

```bash
bash scripts/compose-dev.sh dev
```

Paseo workspace scripts (`paseo.json`) wrap the same commands: `check`, `up`,
`status`, `endpoints`, `logs`, `down`, `destroy`.

Override instance slug (sanitized to `[a-z0-9-]`, max 48):

```bash
STACKLANE_INSTANCE=my-feature bash scripts/compose-dev.sh up
```

Default instance is the worktree directory name (e.g. `stacklane-compose`), else
branch name, else `dev`. Compose project is always `primer-<instance>` via
`docker compose -p` (never hardcoded in the YAML).

## Endpoints

| Path | Meaning |
|------|---------|
| `http://web.<instance>.primer.test:3000` | LMS SPA via Stacklane VIP **3000** → container 5173 |
| `http://api.<instance>.primer.test:8080` | LMS API via Stacklane VIP **8080** |
| `http://tv-web.<instance>.primer.test:3001` | TV SPA via Stacklane VIP **3001** → container 5173 |
| `http://tv.<instance>.primer.test:8081` | TV API via Stacklane VIP **8081** |
| `http://investor.<instance>.primer.test:3002` | Investor site via Stacklane VIP **3002** → container 5173 |
| `postgres.<instance>.primer.test:5432` | Postgres via Stacklane VIP **5432** |
| `http://127.0.0.1:<ephemeral>/` | Direct loopback (compose published port) |

Print mappings:

```bash
bash scripts/compose-dev.sh endpoints
```

LMS `/api` is proxied by Vite to `http://api:8080` (Compose DNS). TV `/api` is
proxied to `http://tv:8081`. No CORS hairpin. CORS allow-lists default to the
external Stacklane web origins on ports 3000 / 3001.

Health:

- LMS / TV: `GET /api/v1/health`
- Vite apps: `GET /`

## Architecture

- **postgres** (`postgres:17-alpine` digest-pinned): LMS `primer` + TV
  `primer_tv` (init script). Named volume `primer_pg_data`.
- **api** (`Dockerfile.dev`): golang `1.25.7-bookworm` digest-pinned, Air
  (`github.com/air-verse/air` MIT, pinned `v1.67.4`) as PID1, repo bind-mounted
  at `/src`, named volumes for Go mod/build caches and `/src/server/tmp`.
- **tv**: same image, Air with `.air.tv.toml`, container port 8081.
- **web / tv-web / investor** (`Dockerfile.web.dev`): node `22-bookworm`
  digest-pinned, `npm ci` fail-closed into named `node_modules` volumes, Vite as
  PID1 on `0.0.0.0:5173` inside the container.
- **Publish form:** only `127.0.0.1::<containerPort>`. Never `0.0.0.0`, empty
  host IP, fixed host ports, host network, or public port 80.
- **Labels** on each externally addressable service: `stacklane.enable`,
  `stacklane.project=primer`, `stacklane.instance`, `stacklane.endpoint`,
  `stacklane.port`, and `stacklane.target_port` when public ≠ container.

## down vs destroy

| Command | Effect |
|---------|--------|
| `bash scripts/compose-dev.sh down` | `docker compose down` — **never** `-v`. DB + caches preserved. |
| `CONFIRM=primer-<instance>-destroy bash scripts/compose-dev.sh destroy` | `docker compose down -v` — deletes named volumes for this project only. |

There is no abbreviated destroy. Confirmation must be exactly
`${COMPOSE_PROJECT}-destroy` (for example `primer-stacklane-compose-destroy`).

## macOS bind-mount caveat

Docker Desktop on macOS bind-mounts are slower than Linux and may delay Air/Vite
file watchers. If reload seems sticky, prefer the host Make path. Named volumes
for `node_modules`, Go caches, and Postgres avoid putting those on the bind
mount.

## Production images

Production packaging remains root `Dockerfile` / `Dockerfile.tv` (unchanged).
Dev images are `Dockerfile.dev` / `Dockerfile.web.dev` only.

## Check harness

`scripts/compose-dev-check.sh` (also `bash scripts/compose-dev.sh check` /
preflight for `up`):

- Renders `docker compose config --format json` to a mode-0700 temp file (does
  not print full env)
- Asserts services, bind mounts, named volumes, loopback ephemeral ports,
  Stacklane labels, health paths, internal Vite proxy targets, no public :80,
  no provider token env, no `.local` domains
- Mutation probes expect failure detection for wildcard host IP, fixed host
  port, missing label, removed source mount
- Default + override instance projects must differ

## Stacklane base domain

FQDNs are `<endpoint>.<instance>.primer.<base>`.

- Docs and Compose default **base** is `test`.
- If the host Stacklane daemon uses another `--dns-base-domain`,
  `scripts/compose-dev.sh` auto-detects it via `stacklane status -o json`.
- Override explicitly: `STACKLANE_BASE_DOMAIN=test bash scripts/compose-dev.sh up`.
- Direct loopback still works when the daemon is absent or resolve lags
  (`stacklane: BLOCKED` / degraded is OK).

Set `VITE_HMR_HOST=` (empty, hyphen-form interpolation) for direct-loopback HMR
so the Vite client uses `window.location`.
