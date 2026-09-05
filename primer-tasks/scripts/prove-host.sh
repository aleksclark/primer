#!/usr/bin/env bash
# Two disposable git worktrees: distinct source, mutation, reload, restore, teardown.
set -Eeuo pipefail

ROOT=$(export CDPATH=; cd -- "$(dirname -- "$0")/.." && pwd)
REPO_ROOT=$(git -C "$ROOT" rev-parse --show-toplevel)
HEAD_SHA=$(git -C "$REPO_ROOT" rev-parse HEAD)
PROBE_TAG="host-$(date +%s)-$$"
mkdir -p "${REPO_ROOT}/.worktrees"
A=$(mktemp -d "${REPO_ROOT}/.worktrees/tasks-p1-host-a.XXXXXX")
B=$(mktemp -d "${REPO_ROOT}/.worktrees/tasks-p1-host-b.XXXXXX")
rmdir "$A" "$B"
git -C "$REPO_ROOT" worktree add --detach "$A" "$HEAD_SHA" >/dev/null
git -C "$REPO_ROOT" worktree add --detach "$B" "$HEAD_SHA" >/dev/null
chmod +x "$A/primer-tasks/scripts/host-stack" "$B/primer-tasks/scripts/host-stack"

cleanup() {
  TASKS_HOST_NAME="${PROBE_TAG}-a" TASKS_HOST_STATE_DIR="$A/primer-tasks/tmp/host-stack" "$A/primer-tasks/scripts/host-stack" down || true
  TASKS_HOST_NAME="${PROBE_TAG}-b" TASKS_HOST_STATE_DIR="$B/primer-tasks/tmp/host-stack" "$B/primer-tasks/scripts/host-stack" down || true
  git -C "$REPO_ROOT" worktree remove --force "$A" >/dev/null 2>&1 || rm -rf -- "$A"
  git -C "$REPO_ROOT" worktree remove --force "$B" >/dev/null 2>&1 || rm -rf -- "$B"
}
trap cleanup EXIT

[[ "$(git -C "$A" rev-parse --show-toplevel)" == "$A" ]]
[[ "$(git -C "$B" rev-parse --show-toplevel)" == "$B" ]]

TASKS_HOST_NAME="${PROBE_TAG}-a" TASKS_HOST_STATE_DIR="$A/primer-tasks/tmp/host-stack" "$A/primer-tasks/scripts/host-stack" up
TASKS_HOST_NAME="${PROBE_TAG}-b" TASKS_HOST_STATE_DIR="$B/primer-tasks/tmp/host-stack" "$B/primer-tasks/scripts/host-stack" up
# shellcheck disable=SC1091
source "$A/primer-tasks/tmp/host-stack/endpoints.env"
A_WEB=$PRIMER_TASKS_BASE_URL
A_API=$PRIMER_TASKS_API_URL
# shellcheck disable=SC1091
source "$B/primer-tasks/tmp/host-stack/endpoints.env"
B_WEB=$PRIMER_TASKS_BASE_URL
B_API=$PRIMER_TASKS_API_URL
[[ "$A_WEB" != "$B_WEB" && "$A_API" != "$B_API" ]] || { echo "prove-host: FAIL shared endpoints" >&2; exit 1; }

before_a=$(curl -fsS "${A_API}health")
before_b=$(curl -fsS "${B_API}health")
python3 - "$A/primer-tasks/internal/api/openapi.go" <<'PY'
from pathlib import Path
import sys
path = Path(sys.argv[1])
raw = path.read_bytes()
old = b'Health{Status: "ok"}'
new = b'Health{Status: "host-a"}'
if raw.count(old) != 1:
    raise SystemExit("backend mutation anchor missing")
path.write_bytes(raw.replace(old, new))
PY
changed=0
for _ in $(seq 1 90); do
  if curl -fsS "${A_API}health" | grep -Fq host-a; then
    changed=1
    break
  fi
  sleep 1
done
[[ "$changed" == 1 ]] || { echo "prove-host: FAIL A did not reload mutated Go" >&2; exit 1; }
[[ "$(curl -fsS "${B_API}health")" == "$before_b" ]] || { echo "prove-host: FAIL B changed after A mutation" >&2; exit 1; }
git -C "$A" checkout -- primer-tasks/internal/api/openapi.go
restored=0
for _ in $(seq 1 90); do
  if [[ "$(curl -fsS "${A_API}health")" == "$before_a" ]]; then
    restored=1
    break
  fi
  sleep 1
done
[[ "$restored" == 1 ]] || { echo "prove-host: FAIL A did not restore" >&2; exit 1; }
[[ -z "$(git -C "$A" status --porcelain)" ]] || { echo "prove-host: FAIL A dirty after restore" >&2; exit 1; }

python3 - "$B/primer-tasks/web/src/App.tsx" <<'PY'
from pathlib import Path
import sys
path = Path(sys.argv[1])
raw = path.read_bytes()
old = b"System C \xc2\xb7 HMR baseline"
new = b"System C \xc2\xb7 host-b"
if raw.count(old) != 1:
    raise SystemExit("frontend mutation anchor missing")
path.write_bytes(raw.replace(old, new))
PY
hmr=0
for _ in $(seq 1 90); do
  if curl -fsS "$B_WEB" | grep -Fq "host-b"; then
    hmr=1
    break
  fi
  # Vite serves index.html; the marker is in the JS module. Probe the source file via the Vite transform.
  if curl -fsS "${B_WEB}src/App.tsx" | grep -Fq "host-b"; then
    hmr=1
    break
  fi
  sleep 1
done
[[ "$hmr" == 1 ]] || { echo "prove-host: FAIL B React source did not update" >&2; exit 1; }
if curl -fsS "${A_WEB}src/App.tsx" | grep -Fq "host-b"; then
  echo "prove-host: FAIL A saw B mutation" >&2
  exit 1
fi
git -C "$B" checkout -- primer-tasks/web/src/App.tsx
[[ -z "$(git -C "$B" status --porcelain)" ]] || { echo "prove-host: FAIL B dirty after restore" >&2; exit 1; }

TASKS_HOST_NAME="${PROBE_TAG}-a" TASKS_HOST_STATE_DIR="$A/primer-tasks/tmp/host-stack" "$A/primer-tasks/scripts/host-stack" down
sleep 1
if curl -fsS --max-time 1 "${A_API}health" >/dev/null 2>&1; then
  echo "prove-host: FAIL A still answering after down" >&2
  exit 1
fi
curl -fsS "${B_API}health" >/dev/null || { echo "prove-host: FAIL B died when A stopped" >&2; exit 1; }
echo "prove-host: PASS two disposable git worktrees"
