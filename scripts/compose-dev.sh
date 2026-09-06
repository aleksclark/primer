#!/usr/bin/env bash
# Primer Stacklane compose lifecycle.
# Always uses: docker compose -p "primer-<instance>" -f "$ROOT/docker-compose.yml"
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CHECK_SCRIPT="${ROOT}/scripts/compose-dev-check.sh"
PROJECT_SLUG="primer"

die() { echo "error: $*" >&2; exit 1; }
info() { echo "primer-compose: $*" >&2; }

# Neutralize accidental ambient Compose controls, then pin this repo's file.
unset COMPOSE_FILE COMPOSE_PROFILES COMPOSE_PROJECT_NAME || true
COMPOSE_FILE="${ROOT}/docker-compose.yml"

# sanitize_instance: lowercase, non [a-z0-9-] → -, collapse dashes, trim, max 48, fallback dev
sanitize_instance() {
  local s="${1:-}"
  s="$(printf '%s' "$s" | tr '[:upper:]' '[:lower:]')"
  s="$(printf '%s' "$s" | sed -E 's/[^a-z0-9-]+/-/g; s/-+/-/g; s/^-+//; s/-+$//')"
  if [[ ${#s} -gt 48 ]]; then
    s="${s:0:48}"
    s="$(printf '%s' "$s" | sed -E 's/-+$//')"
  fi
  if [[ -z "$s" ]]; then
    s="dev"
  fi
  printf '%s' "$s"
}

derive_instance() {
  if [[ -n "${STACKLANE_INSTANCE:-}" ]]; then
    sanitize_instance "$STACKLANE_INSTANCE"
    return
  fi
  local wt
  wt="$(basename "$ROOT")"
  if [[ -n "$wt" && "$wt" != "." && "$wt" != "/" ]]; then
    sanitize_instance "$wt"
    return
  fi
  local branch=""
  if command -v git >/dev/null 2>&1; then
    branch="$(git -C "$ROOT" rev-parse --abbrev-ref HEAD 2>/dev/null || true)"
  fi
  if [[ -n "$branch" && "$branch" != "HEAD" ]]; then
    sanitize_instance "$branch"
    return
  fi
  sanitize_instance "dev"
}

detect_base_domain() {
  if [[ -n "${STACKLANE_BASE_DOMAIN:-}" ]]; then
    printf '%s' "$STACKLANE_BASE_DOMAIN"
    return
  fi
  if ! command -v stacklane >/dev/null 2>&1; then
    printf 'test'
    return
  fi
  local detected=""
  detected="$(
    timeout 3 stacklane status -o json 2>/dev/null \
      | python3 -c 'import json,sys,re
try:
    raw=json.load(sys.stdin)
except Exception:
    sys.exit(0)
val=raw.get("base_domain") if isinstance(raw, dict) else None
if isinstance(val, str) and re.fullmatch(r"[A-Za-z0-9](?:[A-Za-z0-9.-]{0,251}[A-Za-z0-9])?", val):
    print(val)
' 2>/dev/null || true
  )"
  if [[ -n "$detected" ]]; then
    printf '%s' "$detected"
    return
  fi
  printf 'test'
}

require_docker() {
  command -v docker >/dev/null 2>&1 || die "docker not found"
  docker compose version >/dev/null 2>&1 || die "docker compose not available"
  [[ -f "$COMPOSE_FILE" ]] || die "missing $COMPOSE_FILE"
}

compose() {
  docker compose -p "$COMPOSE_PROJECT" --project-directory "$ROOT" -f "$COMPOSE_FILE" "$@"
}

export_stack_env() {
  INSTANCE="$(derive_instance)"
  COMPOSE_PROJECT="${PROJECT_SLUG}-${INSTANCE}"
  export STACKLANE_INSTANCE="$INSTANCE"
  export COMPOSE_PROJECT_NAME="$COMPOSE_PROJECT"
  export STACKLANE_BASE_DOMAIN="$(detect_base_domain)"
  export LMS_CORS_ORIGINS="${LMS_CORS_ORIGINS:-http://web.${INSTANCE}.${PROJECT_SLUG}.${STACKLANE_BASE_DOMAIN}:3000}"
  export TV_CORS_ORIGINS="${TV_CORS_ORIGINS:-http://tv-web.${INSTANCE}.${PROJECT_SLUG}.${STACKLANE_BASE_DOMAIN}:3001}"
}

host_port_for() {
  local svc="$1"
  local target="$2"
  local mapping
  mapping="$(compose port "$svc" "$target" 2>/dev/null || true)"
  if [[ -z "$mapping" ]]; then
    printf ''
    return
  fi
  printf '%s' "${mapping##*:}"
}

stacklane_status_line() {
  if ! command -v stacklane >/dev/null 2>&1; then
    printf 'stacklane: BLOCKED (daemon/cli absent — direct loopback ports still work)\n'
    return
  fi
  if timeout 3 stacklane status >/dev/null 2>&1; then
    local web_fqdn="web.${INSTANCE}.${PROJECT_SLUG}.${STACKLANE_BASE_DOMAIN:-test}"
    if timeout 3 stacklane resolve "$web_fqdn" >/dev/null 2>&1; then
      printf 'stacklane: OK\n'
    else
      printf 'stacklane: degraded (daemon up; %s not resolved yet)\n' "$web_fqdn"
    fi
  else
    printf 'stacklane: BLOCKED (daemon not reachable)\n'
  fi
}

print_direct() {
  local label="$1" svc="$2" target="$3"
  local hp
  hp="$(host_port_for "$svc" "$target")"
  if [[ -n "$hp" ]]; then
    echo "direct ${label}:  http://127.0.0.1:${hp}/"
  else
    echo "direct ${label}:  (not published — stack down?)"
  fi
}

print_endpoints() {
  local base="${STACKLANE_BASE_DOMAIN:-test}"

  echo "web.${INSTANCE}.${PROJECT_SLUG}.${base}:3000  (via Stacklane VIP)"
  echo "api.${INSTANCE}.${PROJECT_SLUG}.${base}:8080"
  echo "tv-web.${INSTANCE}.${PROJECT_SLUG}.${base}:3001"
  echo "tv.${INSTANCE}.${PROJECT_SLUG}.${base}:8081"
  echo "investor.${INSTANCE}.${PROJECT_SLUG}.${base}:3002"
  echo "postgres.${INSTANCE}.${PROJECT_SLUG}.${base}:5432"
  print_direct "web" web 5173
  print_direct "api" api 8080
  print_direct "tv-web" tv-web 5173
  print_direct "tv" tv 8081
  print_direct "investor" investor 5173
  print_direct "postgres" postgres 5432
  stacklane_status_line
  echo "instance: ${INSTANCE}"
  echo "compose project: ${COMPOSE_PROJECT}"
  echo "stacklane base_domain: ${base}"
}

container_health() {
  local svc="$1"
  docker inspect -f '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}' \
    "${COMPOSE_PROJECT}-${svc}-1" 2>/dev/null || echo missing
}

wait_healthy() {
  local timeout_s="${1:-300}"
  local start now elapsed
  local services=(postgres api tv web tv-web investor)
  start="$(date +%s)"
  info "waiting for ${services[*]} healthy (timeout ${timeout_s}s)…"
  while true; do
    now="$(date +%s)"
    elapsed=$((now - start))
    if (( elapsed > timeout_s )); then
      compose ps || true
      die "services not healthy within ${timeout_s}s"
    fi
    local all_ok=1 svc h
    for svc in "${services[@]}"; do
      h="$(container_health "$svc")"
      if [[ "$h" != "healthy" ]]; then
        all_ok=0
        break
      fi
    done
    if [[ "$all_ok" -eq 1 ]]; then
      info "all services healthy"
      return 0
    fi
    sleep 2
  done
}

cmd_check() {
  require_docker
  export_stack_env
  bash "$CHECK_SCRIPT"
}

cmd_up() {
  require_docker
  export_stack_env
  bash "$CHECK_SCRIPT"
  info "building images (project=${COMPOSE_PROJECT} instance=${INSTANCE})…"
  compose build
  info "starting stack…"
  compose up -d --remove-orphans
  wait_healthy 360
  print_endpoints
}

cmd_dev() {
  # Foreground logs. Ctrl-C leaves the stack UP (detached). Use down to stop.
  cmd_up
  info "following logs (Ctrl-C leaves stack up; run: bash scripts/compose-dev.sh down)"
  compose logs -f
}

cmd_status() {
  require_docker
  export_stack_env
  compose ps
  echo
  print_endpoints
}

cmd_logs() {
  require_docker
  export_stack_env
  compose logs "$@"
}

cmd_down() {
  require_docker
  export_stack_env
  info "stopping stack (volumes preserved; never uses -v)…"
  compose down --remove-orphans
}

cmd_destroy() {
  require_docker
  export_stack_env
  local expect="${COMPOSE_PROJECT}-destroy"
  if [[ "${CONFIRM:-}" != "$expect" ]]; then
    die "refusing destroy: set CONFIRM=${expect} to remove volumes for project ${COMPOSE_PROJECT}"
  fi
  info "destroying stack AND volumes for ${COMPOSE_PROJECT}…"
  compose down -v --remove-orphans
}

cmd_endpoints() {
  require_docker
  export_stack_env
  print_endpoints
}

usage() {
  cat <<'EOF'
Usage: scripts/compose-dev.sh <command>

Commands:
  check       Fail-closed Stacklane/compose contract validation
  up          check + build + up -d + wait healthy + print endpoints
  dev         up then follow logs (Ctrl-C leaves stack running)
  status      compose ps + endpoint table
  logs        docker compose logs (extra args forwarded)
  down        compose down (never -v; volumes preserved)
  destroy     compose down -v (requires CONFIRM=<compose-project>-destroy)
  endpoints   print FQDNs + direct loopback mappings

Environment:
  STACKLANE_INSTANCE     override instance slug (else worktree dirname / branch)
  STACKLANE_BASE_DOMAIN  FQDN base (default: host daemon base_domain, else test)
  VITE_HMR_HOST / VITE_HMR_CLIENT_PORT / VITE_HMR_PROTOCOL  optional HMR overrides
  LMS_CORS_ORIGINS / TV_CORS_ORIGINS  optional CORS overrides

Notes:
  - Host `make dev-db`, `make test`, and `make investor-web-dev` are unchanged.
  - Stacklane is opt-in; this wrapper is the compose entrypoint (not host make).
  - No provider tokens in compose. Identity / Jellyfin / agents stay disabled.
  - Stacklane daemon is optional; direct 127.0.0.1 ephemeral ports always work.
  - destroy requires exact CONFIRM=$COMPOSE_PROJECT-destroy (never an alias).
  - logs / dev: Ctrl-C leaves the stack running.
EOF
}

main() {
  local cmd="${1:-}"
  shift || true
  case "$cmd" in
    check) cmd_check "$@" ;;
    up) cmd_up "$@" ;;
    dev) cmd_dev "$@" ;;
    status) cmd_status "$@" ;;
    logs) cmd_logs "$@" ;;
    down) cmd_down "$@" ;;
    destroy) cmd_destroy "$@" ;;
    endpoints) cmd_endpoints "$@" ;;
    -h|--help|help|"") usage; [[ -n "$cmd" ]] || exit 1 ;;
    *) die "unknown command: $cmd (try --help)" ;;
  esac
}

main "$@"
