#!/usr/bin/env bash
# Automated student-client acceptance smoke against a running LMS API.
#
# Prerequisites:
#   - LMS API reachable (default http://127.0.0.1:8080/api/v1)
#   - Parent educator credentials that can login
#   - curriculum activities published (make activity-publish) OR this script
#     will attempt assign-next after login
#   - Go toolchain (builds primer-student-harness as a test-only runner)
#
# Physical workstation steps are listed at the end and are NOT executed here.
#
# Usage:
#   export PRIMER_API=http://127.0.0.1:8080/api/v1
#   export PRIMER_PARENT_EMAIL=parent@example.com
#   export PRIMER_PARENT_PASSWORD=secret
#   export PRIMER_STUDENT_ID=<uuid>          # optional; created if empty
#   ./scripts/student-acceptance.sh

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
API="${PRIMER_API:-http://127.0.0.1:8080/api/v1}"
EMAIL="${PRIMER_PARENT_EMAIL:-}"
PASSWORD="${PRIMER_PARENT_PASSWORD:-}"
STUDENT_ID="${PRIMER_STUDENT_ID:-}"
SLUG="${PRIMER_ACTIVITY_SLUG:-basic-navigation}"
WORKDIR="${TMPDIR:-/tmp}/primer-student-acceptance-$$"
HARNESS_BIN="${PRIMER_HARNESS_BIN:-}"

log() { printf '==> %s\n' "$*"; }
die() { printf 'ERROR: %s\n' "$*" >&2; exit 1; }

need() { command -v "$1" >/dev/null 2>&1 || die "missing dependency: $1"; }

json_field() {
  # json_field <json> <key>
  python3 -c 'import json,sys; d=json.loads(sys.argv[1]); k=sys.argv[2];
print(d[k] if k in d else d.get("body",{}).get(k,""))' "$1" "$2" 2>/dev/null \
    || python3 -c 'import json,sys; print(json.loads(sys.argv[1])[sys.argv[2]])' "$1" "$2"
}

api() {
  local method="$1" path="$2"
  shift 2
  local args=(-sS -X "$method" "${API}${path}" -H "Content-Type: application/json")
  if [[ -n "${TOKEN:-}" ]]; then
    args+=(-H "Authorization: Bearer ${TOKEN}")
  fi
  if [[ $# -gt 0 ]]; then
    args+=(-d "$1")
  fi
  curl "${args[@]}"
}

need curl
need python3
need go

[[ -n "$EMAIL" && -n "$PASSWORD" ]] || die "set PRIMER_PARENT_EMAIL and PRIMER_PARENT_PASSWORD"

mkdir -p "$WORKDIR"
trap 'rm -rf "$WORKDIR"' EXIT

log "Health check ${API%/api/v1}/health or ${API}/../health"
if ! curl -sS -f "${API}/health" >/dev/null 2>&1; then
  # health is registered at /health on the mux; some proxies expose under /api/v1
  curl -sS -f "${API%/api/v1}/health" >/dev/null || die "API health check failed"
fi

log "Parent login"
LOGIN_JSON="$(api POST /auth/login "{\"email\":\"${EMAIL}\",\"password\":\"${PASSWORD}\"}")"
TOKEN="$(python3 -c 'import json,sys; print(json.loads(sys.argv[1])["token"])' "$LOGIN_JSON")"
[[ -n "$TOKEN" ]] || die "login did not return token: $LOGIN_JSON"

if [[ -z "$STUDENT_ID" ]]; then
  log "Create student"
  STU_JSON="$(api POST /students '{"firstName":"Accept","lastName":"Test","gradeLevel":7}')"
  STUDENT_ID="$(python3 -c 'import json,sys; print(json.loads(sys.argv[1])["id"])' "$STU_JSON")"
fi
log "Student ${STUDENT_ID}"

log "Issue pairing code"
PC_JSON="$(api POST /pairing-codes "{\"studentId\":\"${STUDENT_ID}\"}")"
CODE="$(python3 -c 'import json,sys; print(json.loads(sys.argv[1])["code"])' "$PC_JSON")"
[[ -n "$CODE" ]] || die "no pairing code: $PC_JSON"

log "Pair device"
PAIR_JSON="$(curl -sS -X POST "${API}/student-devices/pair" \
  -H "Content-Type: application/json" \
  -d "{\"code\":\"${CODE}\",\"deviceName\":\"acceptance-ws\"}")"
DEVICE_TOKEN="$(python3 -c 'import json,sys; print(json.loads(sys.argv[1])["token"])' "$PAIR_JSON")"
DEVICE_ID="$(python3 -c 'import json,sys; print(json.loads(sys.argv[1])["deviceId"])' "$PAIR_JSON")"
[[ -n "$DEVICE_TOKEN" ]] || die "pair failed: $PAIR_JSON"
log "Device ${DEVICE_ID}"

log "Assign activity slug=${SLUG}"
ASSIGN_JSON="$(api POST "/students/${STUDENT_ID}/assign-next" "{\"slug\":\"${SLUG}\"}")"
ASSIGN_ID="$(python3 -c 'import json,sys; d=json.loads(sys.argv[1]); print(d.get("assignment",{}).get("id",""))' "$ASSIGN_JSON")"
[[ -n "$ASSIGN_ID" ]] || die "assign-next failed: $ASSIGN_JSON"
log "Assignment ${ASSIGN_ID}"

log "Build test-only harness (if needed)"
if [[ -z "$HARNESS_BIN" ]]; then
  HARNESS_BIN="${WORKDIR}/primer-student-harness"
  (cd "${ROOT}/server" && go build -o "$HARNESS_BIN" ./cmd/primer-student-harness)
fi

log "Run headless engine for ${SLUG}"
DB_PATH="${WORKDIR}/state.db"
# harness is intentionally test-only; allow unsandboxed for CI/dev hosts
set +e
HARNESS_OUT="$("$HARNESS_BIN" \
  -base-url "$API" \
  -token "$DEVICE_TOKEN" \
  -db "$DB_PATH" \
  -slug "$SLUG" \
  -assignment "$ASSIGN_ID" 2>&1)"
HARNESS_RC=$?
set -e
printf '%s\n' "$HARNESS_OUT"
[[ $HARNESS_RC -eq 0 ]] || die "harness failed rc=${HARNESS_RC}"

log "Verify learning overview / mastery"
OV_JSON="$(api GET "/students/${STUDENT_ID}/learning-overview")"
printf '%s' "$OV_JSON" > "${WORKDIR}/overview.json"
export WORKDIR
python3 - <<'PY'
import json, os, pathlib
ov = json.loads(pathlib.Path(os.environ["WORKDIR"], "overview.json").read_text())
print("open_assignments", len(ov.get("openAssignments") or []))
print("sessions", len(ov.get("recentSessions") or []))
print("mastery", len(ov.get("masterySummary") or []))
assert any(s.get("state") == "completed" for s in (ov.get("recentSessions") or [])), "expected a completed session"
assert "evidenceStatuses" in ov, "learning overview must expose evidenceStatuses"
statuses = ov.get("evidenceStatuses") or []
print("evidence_statuses", len(statuses))
# Phase 0 honesty: procedural evidence must not claim formal mastery alone.
for st in statuses:
    assert st.get("formalMastery") is not True or st.get("evidenceStatus") == "formal_mastery"
    if st.get("proceduralAccepted") and st.get("missingEvidenceClasses"):
        assert st.get("additionalEvidenceRequired") is True or st.get("evidenceStatus") == "additional_evidence_required"
        assert "mastered" not in (st.get("masteryStatus") or "")
        print("truthful_status", st.get("standardCode"), st.get("evidenceStatus"), "missing", st.get("missingEvidenceClasses"))
if not ov.get("masterySummary"):
    print("WARN: no mastery records yet (publish standards + activity links)")
else:
    print("mastery_ok")
    assert statuses, "expected evidenceStatuses when masterySummary is non-empty"
PY

log "Metrics"
MET_JSON="$(api GET /ops/student-metrics)"
printf '%s' "$MET_JSON" > "${WORKDIR}/metrics.json"
python3 - <<'PY'
import json, os, pathlib
m = json.loads(pathlib.Path(os.environ["WORKDIR"], "metrics.json").read_text())
print(m)
assert m.get("devicesActive", 0) >= 1
PY

log "Automated acceptance PASSED"
cat <<'EOF'

Manual physical checklist (not run by this script):
  [ ] Fresh pair on Lenovo workstation image
  [ ] Online completion in TUI
  [ ] Offline completion + sync
  [ ] Reboot resume (terminal + typing)
  [ ] Server outage recovery
  [ ] Revoke / re-pair on hardware
  [ ] Deploy rollback via workstation/deploy.sh
  [ ] Parent SPA review of session + mastery

See agent_docs/runbooks/student-client-ops.md
EOF
