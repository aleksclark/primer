#!/usr/bin/env bash
# Real Compose hot-reload and two-instance isolation proof.
#
# This script is intentionally fail-closed: it starts two disposable Compose
# projects, checks their mounts/labels/ports, mutates exact source bytes, waits
# for observed HTTP/CDP changes, restores those bytes, and only then cleans up
# the projects it created. It never uses the ambient STACKLANE_INSTANCE.
set -Eeuo pipefail

ROOT=$(export CDPATH=; cd -- "$(dirname -- "$0")/.." && pwd)
DEV="$ROOT/scripts/dev"
COMPOSE_FILE="$ROOT/compose.yaml"
PROJECT_SLUG="primer-tasks"
PROBE_TAG="slc-hr-$(date +%s)-$$-$(od -An -N3 -tx1 /dev/urandom | tr -d ' \n')"
INSTANCE_A="${PROBE_TAG}-a"
INSTANCE_B="${PROBE_TAG}-b"
PROJECT_A="${PROJECT_SLUG}-${INSTANCE_A}"
PROJECT_B="${PROJECT_SLUG}-${INSTANCE_B}"
BASE_DOMAIN="${STACKLANE_BASE_DOMAIN:-test}"
TMPDIR_PROOF=""
ROOT_A=""
ROOT_B=""
BACKEND_BACKUP=""
FRONTEND_BACKUP=""
BACKEND_RESTORED=0
FRONTEND_RESTORED=0
CREATED_A=0
CREATED_B=0
HMR_PID=""
HMR_READY=""
CHROME_PID=""
CHROME_DIR=""
EXIT_STATUS=0

compose_for() {
  local instance=$1
  shift
  local project_dir=$ROOT
  if [[ "$instance" == "$INSTANCE_A" && -n "$ROOT_A" ]]; then
    project_dir=$ROOT_A
  elif [[ "$instance" == "$INSTANCE_B" && -n "$ROOT_B" ]]; then
    project_dir=$ROOT_B
  fi
  docker compose -p "${PROJECT_SLUG}-${instance}" -f "$project_dir/compose.yaml" \
    --project-directory "$project_dir" "$@"
}

fail() {
  echo "prove-dev: FAIL $*" >&2
  exit 1
}

cleanup() {
  local status=$?
  set +e
  if [[ -n "$HMR_PID" ]]; then
    kill "$HMR_PID" 2>/dev/null || true
    wait "$HMR_PID" 2>/dev/null || true
  fi
  if [[ -n "$CHROME_PID" ]]; then
    kill "$CHROME_PID" 2>/dev/null || true
    wait "$CHROME_PID" 2>/dev/null || true
  fi

  # Restoration precedes teardown so a watcher can observe the original bytes.
  if [[ -n "$BACKEND_BACKUP" && -n "$ROOT_A" ]]; then
    cp -- "$BACKEND_BACKUP" "$ROOT_A/internal/api/openapi.go"
    if cmp -s "$BACKEND_BACKUP" "$ROOT_A/internal/api/openapi.go"; then
      BACKEND_RESTORED=1
    else
      echo "prove-dev: FAIL backend source was not restored exactly" >&2
      status=1
    fi
  fi
  if [[ -n "$FRONTEND_BACKUP" && -n "$ROOT_B" ]]; then
    cp -- "$FRONTEND_BACKUP" "$ROOT_B/web/src/App.tsx"
    if cmp -s "$FRONTEND_BACKUP" "$ROOT_B/web/src/App.tsx"; then
      FRONTEND_RESTORED=1
    else
      echo "prove-dev: FAIL frontend source was not restored exactly" >&2
      status=1
    fi
  fi

  if [[ "$status" -ne 0 ]]; then
    [[ "$CREATED_A" == 1 ]] && compose_for "$INSTANCE_A" logs --no-color --tail=80 api web >&2 || true
    [[ "$CREATED_B" == 1 ]] && compose_for "$INSTANCE_B" logs --no-color --tail=80 api web >&2 || true
  fi

  if [[ "$CREATED_A" == 1 ]]; then
    CONFIRM="${PROJECT_A}-destroy" STACKLANE_INSTANCE="$INSTANCE_A" "$DEV" destroy >/dev/null 2>&1 || status=1
  fi
  if [[ "$CREATED_B" == 1 ]]; then
    CONFIRM="${PROJECT_B}-destroy" STACKLANE_INSTANCE="$INSTANCE_B" "$DEV" destroy >/dev/null 2>&1 || status=1
  fi
  if [[ -n "$CHROME_DIR" ]]; then
    rm -rf -- "$CHROME_DIR"
  fi
  if [[ -n "$TMPDIR_PROOF" ]]; then
    rm -rf -- "$TMPDIR_PROOF"
  fi
  if [[ "$status" -eq 0 && "$BACKEND_RESTORED" == 1 && "$FRONTEND_RESTORED" == 1 ]]; then
    echo "prove-dev: PASS sources restored and probe projects cleaned"
  fi
  exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT TERM

command -v docker >/dev/null || fail "docker is required"
command -v curl >/dev/null || fail "curl is required"
command -v google-chrome >/dev/null || fail "google-chrome is required for the real HMR proof"
python3 -c 'import websocket' >/dev/null 2>&1 || fail "python websocket-client is required for the real HMR proof"

# Refuse to touch an existing project or volume, even if its name resembles a
# probe. This protects an operator's stack from accidental destroy.
assert_absent() {
  local project=$1
  local containers volumes
  containers=$(docker ps -aq --filter "label=com.docker.compose.project=${project}")
  volumes=$(docker volume ls -q --filter "label=com.docker.compose.project=${project}")
  [[ -z "$containers" ]] || fail "compose project already exists: $project"
  [[ -z "$volumes" ]] || fail "compose volumes already exist: $project"
}
assert_absent "$PROJECT_A"
assert_absent "$PROJECT_B"

TMPDIR_PROOF=$(mktemp -d)
chmod 700 "$TMPDIR_PROOF"
ROOT_A="$TMPDIR_PROOF/src-a"
ROOT_B="$TMPDIR_PROOF/src-b"
mkdir -p "$ROOT_A" "$ROOT_B"
rsync -a --exclude node_modules --exclude tmp --exclude build --exclude dist --exclude android "$ROOT/" "$ROOT_A/"
rsync -a --exclude node_modules --exclude tmp --exclude build --exclude dist --exclude android "$ROOT/" "$ROOT_B/"
BACKEND_BACKUP="$TMPDIR_PROOF/openapi.go"
FRONTEND_BACKUP="$TMPDIR_PROOF/App.tsx"
cp -- "$ROOT_A/internal/api/openapi.go" "$BACKEND_BACKUP"
cp -- "$ROOT_B/web/src/App.tsx" "$FRONTEND_BACKUP"
chmod 600 "$BACKEND_BACKUP" "$FRONTEND_BACKUP"

# Use the product lifecycle for both instances; explicitly overriding the
# instance is what makes this proof independent of the operator's worktree.
start_instance() {
  local instance=$1
  local project="${PROJECT_SLUG}-${instance}"
  local project_dir=$ROOT
  if [[ "$instance" == "$INSTANCE_A" ]]; then project_dir=$ROOT_A; else project_dir=$ROOT_B; fi
  if STACKLANE_INSTANCE="$instance" "$project_dir/scripts/dev" up; then
    return 0
  fi
  # `docker compose up` can create a dependency before a later service fails;
  # record the project if any labeled resource exists so EXIT cleanup remains
  # responsible for only this probe.
  if [[ -n "$(docker ps -aq --filter "label=com.docker.compose.project=${project}")" || \
        -n "$(docker volume ls -q --filter "label=com.docker.compose.project=${project}")" ]]; then
    if [[ "$instance" == "$INSTANCE_A" ]]; then CREATED_A=1; else CREATED_B=1; fi
  fi
  return 1
}
start_instance "$INSTANCE_A"
CREATED_A=1
start_instance "$INSTANCE_B"
CREATED_B=1

port_for() {
  local instance=$1 service=$2 target=$3
  compose_for "$instance" port "$service" "$target" 2>/dev/null \
    | head -n 1 | sed -E 's/.*:([0-9]+)$/\1/'
}

wait_http() {
  local url=$1 expected=${2:-200} deadline=$((SECONDS + 90)) body status
  while (( SECONDS < deadline )); do
    status=$(curl -sS -o /tmp/prove-dev-response -w '%{http_code}' "$url" 2>/dev/null || true)
    if [[ "$status" == "$expected" ]]; then
      cat /tmp/prove-dev-response
      return 0
    fi
    sleep 1
  done
  fail "timed out waiting for $url (expected HTTP $expected)"
}

API_A_PORT=$(port_for "$INSTANCE_A" api 8080)
API_B_PORT=$(port_for "$INSTANCE_B" api 8080)
WEB_A_PORT=$(port_for "$INSTANCE_A" web 5173)
WEB_B_PORT=$(port_for "$INSTANCE_B" web 5173)
[[ "$API_A_PORT" =~ ^[0-9]+$ && "$API_B_PORT" =~ ^[0-9]+$ ]] || fail "missing ephemeral API ports"
[[ "$WEB_A_PORT" =~ ^[0-9]+$ && "$WEB_B_PORT" =~ ^[0-9]+$ ]] || fail "missing ephemeral web ports"
[[ "$API_A_PORT" != "$API_B_PORT" ]] || fail "API instances share a host port"
[[ "$WEB_A_PORT" != "$WEB_B_PORT" ]] || fail "web instances share a host port"

wait_http "http://127.0.0.1:${API_A_PORT}/health" >/dev/null
wait_http "http://127.0.0.1:${API_B_PORT}/health" >/dev/null
wait_http "http://127.0.0.1:${WEB_A_PORT}/" >/dev/null
wait_http "http://127.0.0.1:${WEB_B_PORT}/" >/dev/null
API_A_CONTAINER=$(compose_for "$INSTANCE_A" ps -q api)
VOLUMES_A=$(docker volume ls -q --filter "label=com.docker.compose.project=${PROJECT_A}" | sort)
VOLUMES_B=$(docker volume ls -q --filter "label=com.docker.compose.project=${PROJECT_B}" | sort)
[[ -n "$API_A_CONTAINER" ]] || fail "missing API container identity"
[[ -n "$VOLUMES_A" && -n "$VOLUMES_B" ]] || fail "probe instances have no named volumes"
[[ "$(comm -12 <(printf '%s\\n' "$VOLUMES_A") <(printf '%s\\n' "$VOLUMES_B"))" == "" ]] || fail "probe instances share a volume"

for instance in "$INSTANCE_A" "$INSTANCE_B"; do
  api_container=$(compose_for "$instance" ps -q api)
  web_container=$(compose_for "$instance" ps -q web)
  [[ -n "$api_container" && -n "$web_container" ]] || fail "missing public containers for $instance"
  for container in "$api_container" "$web_container"; do
    source_mount=$(docker inspect -f '{{range .Mounts}}{{if eq .Destination "/src"}}{{.Source}}{{end}}{{end}}' "$container")
    expected_root=$ROOT_A
    [[ "$instance" == "$INSTANCE_B" ]] && expected_root=$ROOT_B
    [[ "$(realpath "$source_mount")" == "$(realpath "$expected_root")" ]] || fail "source mount for $instance is not its isolated root"
    label_instance=$(docker inspect -f '{{index .Config.Labels "stacklane.instance"}}' "$container")
    label_project=$(docker inspect -f '{{index .Config.Labels "stacklane.project"}}' "$container")
    label_endpoint=$(docker inspect -f '{{index .Config.Labels "stacklane.endpoint"}}' "$container")
    [[ "$label_instance" == "$instance" ]] || fail "Stacklane instance label mismatch for $instance"
    [[ "$label_project" == "$PROJECT_SLUG" ]] || fail "Stacklane project label mismatch for $instance"
    [[ "$label_endpoint" == "api" || "$label_endpoint" == "web" ]] || fail "Stacklane endpoint label missing for $instance"
  done
  expected_web="web.${instance}.${PROJECT_SLUG}.${BASE_DOMAIN}"
  expected_api="api.${instance}.${PROJECT_SLUG}.${BASE_DOMAIN}"
  if [[ "$instance" == "$INSTANCE_A" ]]; then
    instance_web_port=$WEB_A_PORT
    instance_api_port=$API_A_PORT
  else
    instance_web_port=$WEB_B_PORT
    instance_api_port=$API_B_PORT
  fi
  echo "prove-dev: $instance web=$expected_web:${instance_web_port} api=$expected_api:${instance_api_port}"
done

# Backend Air proof: mutate bytes that are on the inspected /src bind mount,
# observe HTTP output, then restore and require the original response again.
BACKEND_NONCE="reload-${PROBE_TAG}"
base_before_mutation=$(curl -fsS "http://127.0.0.1:${API_A_PORT}/health")
# Let Air finish its initial watcher snapshot before the first mutation. The
# health endpoint can answer as soon as the binary starts, before fsnotify has
# completed its baseline scan; mutating in that window can be silently missed.
sleep 3
python3 - "$ROOT_A/internal/api/openapi.go" "$BACKEND_NONCE" <<'PY'
from pathlib import Path
import sys
path = Path(sys.argv[1])
nonce = sys.argv[2]
raw = path.read_bytes()
old = b'Health{Status: "ok"}'
new = ('Health{Status: "' + nonce + '"}').encode()
if raw.count(old) != 1:
    raise SystemExit("backend mutation anchor was not unique")
path.write_bytes(raw.replace(old, new))
PY
backend_restore_pending=1
for _ in $(seq 1 90); do
  if curl -fsS "http://127.0.0.1:${API_A_PORT}/health" | grep -Fq "$BACKEND_NONCE"; then
    [[ "$(compose_for "$INSTANCE_A" ps -q api)" == "$API_A_CONTAINER" ]] || fail "Go response change required an API container restart"
    echo "prove-dev: PASS Go response reloaded without container restart"
    backend_restore_pending=0
    # Air may still be coalescing the mutation event when the new process first
    # answers. Let that build settle before restoring the exact original bytes;
    # otherwise the restore event can be lost and the proof reports a false
    # failure while leaving the watcher on the nonce build.
    sleep 2
    cp -- "$BACKEND_BACKUP" "$ROOT_A/internal/api/openapi.go"
    cmp -s "$BACKEND_BACKUP" "$ROOT_A/internal/api/openapi.go" || fail "backend restore checksum mismatch"
    BACKEND_RESTORED=1
    break
  fi
  sleep 1
done
[[ "$backend_restore_pending" == 0 ]] || fail "Go watcher did not expose mutated response"
for _ in $(seq 1 90); do
  restored=$(curl -fsS "http://127.0.0.1:${API_A_PORT}/health" || true)
  [[ "$restored" == "$base_before_mutation" ]] && break
  sleep 1
done
[[ "${restored:-}" == "$base_before_mutation" ]] || fail "Go response did not return after exact restore"

# Frontend proof: Chrome observes the real DOM marker and rejects a full
# Page.frameNavigated event after baseline. The source is restored by EXIT too.
FRONTEND_NONCE="HMR-${PROBE_TAG}"
HMR_READY="$TMPDIR_PROOF/hmr.ready"
CHROME_DIR="$TMPDIR_PROOF/chrome"
mkdir -p "$CHROME_DIR"
CHROME_PORT=$(python3 - <<'PY'
import socket
with socket.socket() as sock:
    sock.bind(("127.0.0.1", 0))
    print(sock.getsockname()[1])
PY
)
google-chrome --headless=new --no-sandbox --disable-gpu --disable-dev-shm-usage \
  --remote-allow-origins=* --user-data-dir="$CHROME_DIR" \
  --remote-debugging-address=127.0.0.1 \
  --remote-debugging-port="$CHROME_PORT" about:blank >/dev/null 2>&1 &
CHROME_PID=$!
for _ in $(seq 1 30); do
  curl -fsS "http://127.0.0.1:${CHROME_PORT}/json/version" >/dev/null 2>&1 && break
  sleep 1
done
curl -fsS "http://127.0.0.1:${CHROME_PORT}/json/version" >/dev/null || fail "Chrome DevTools endpoint did not start"
python3 "$ROOT/scripts/prove-hmr.py" "$CHROME_PORT" "http://127.0.0.1:${WEB_B_PORT}/student/pair" \
  "System C · HMR baseline" "System C · $FRONTEND_NONCE" "$HMR_READY" >"$TMPDIR_PROOF/hmr.log" 2>&1 &
HMR_PID=$!
for _ in $(seq 1 60); do
  [[ -f "$HMR_READY" ]] && break
  kill -0 "$HMR_PID" 2>/dev/null || { cat "$TMPDIR_PROOF/hmr.log" >&2; fail "HMR browser probe exited before baseline"; }
  sleep 0.5
done
[[ -f "$HMR_READY" ]] || fail "HMR browser probe did not establish baseline"
python3 - "$ROOT_B/web/src/App.tsx" "$FRONTEND_NONCE" <<'PY'
from pathlib import Path
import sys
path = Path(sys.argv[1])
nonce = sys.argv[2]
raw = path.read_bytes()
# The source is UTF-8; spell the anchor as code points to avoid shell byte loss.
old = "System C · HMR baseline".encode()
new = ("System C · " + nonce).encode()
if raw.count(old) != 1:
    raise SystemExit("frontend mutation anchor was not unique")
path.write_bytes(raw.replace(old, new))
PY
for _ in $(seq 1 90); do
  if ! kill -0 "$HMR_PID" 2>/dev/null; then break; fi
  sleep 1
done
wait "$HMR_PID" || { cat "$TMPDIR_PROOF/hmr.log" >&2; fail "Vite HMR proof failed"; }
cat "$TMPDIR_PROOF/hmr.log"
cp -- "$FRONTEND_BACKUP" "$ROOT_B/web/src/App.tsx"
cmp -s "$FRONTEND_BACKUP" "$ROOT_B/web/src/App.tsx" || fail "frontend restore checksum mismatch"
FRONTEND_RESTORED=1

# Stop only A and prove B remains healthy. Both projects and their volumes are
# named independently; cleanup later uses each exact destroy confirmation.
compose_for "$INSTANCE_A" stop >/dev/null
wait_http "http://127.0.0.1:${API_B_PORT}/health" >/dev/null
echo "prove-dev: PASS stopping $PROJECT_A left $PROJECT_B healthy"
