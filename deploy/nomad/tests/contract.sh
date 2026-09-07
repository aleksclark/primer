#!/usr/bin/env bash
# Repository-local plan-03 contract tests for the primer release set.
# No secrets, no Nomad token, no cluster mutation.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
REPO_ROOT="$(cd "${ROOT}/../.." && pwd)"
cd "${REPO_ROOT}"

fail() { echo "FAIL: $*" >&2; exit 1; }
pass() { echo "OK: $*"; }

DIGEST_RE='@sha256:[0-9a-f]{64}$'
SECRET_VALUE_RE='(postgres://[^[:space:]]+:[^@[:space:]]+@|password[[:space:]]*=[[:space:]]*[^[:space:]]|TOKEN[[:space:]]*=[[:space:]]*[^$\{][^[:space:]]{8,}|API_KEY[[:space:]]*=[[:space:]]*[^$\{][^[:space:]]{8,})'

echo "==> plan-03 primer contract @ ${ROOT}"

# ── layout ──────────────────────────────────────────────────────────────────
for p in \
  deployment.yaml \
  images.lock.hcl \
  env/home.nomadvars.hcl \
  jobs/primer.nomad.hcl \
  jobs/primer-tv.nomad.hcl \
  jobs/content-ingest.nomad.hcl \
  README.md \
  tests/contract.sh \
  tests/expected-services.json
do
  [[ -f "${ROOT}/${p}" ]] || fail "missing ${p}"
done
pass "layout"

# ── CODEOWNERS ──────────────────────────────────────────────────────────────
CO="${REPO_ROOT}/.github/CODEOWNERS"
[[ -f "${CO}" ]] || fail "missing .github/CODEOWNERS"
grep -qE '^/deploy/nomad/[[:space:]]+@aleksclark' "${CO}" \
  || fail "CODEOWNERS must own /deploy/nomad/ @aleksclark"
pass "CODEOWNERS"

# ── deployment.yaml schema (stdlib python; no pyyaml required) ──────────────
python3 - <<'PY' "${ROOT}/deployment.yaml" || fail "deployment.yaml schema"
import json, re, sys
from pathlib import Path

path = Path(sys.argv[1])
text = path.read_text()
# Minimal YAML subset reader for this strict manifest (no nested complexity beyond lists/maps we emit).
try:
    import yaml  # type: ignore
    data = yaml.safe_load(text)
except Exception:
    # Fallback: require JSON-compatible YAML (already is for our file) via a tiny parser.
    # Convert simple YAML to JSON-ish: only supports our emitted shape.
    import ast
    # Prefer json if someone converted; else use ruamel-free approach with regex checks.
    data = None

if data is None:
    # Structural assertions without full parse
    need = [
        r'(?m)^schema_version:\s*1\s*$',
        r'(?m)^project:\s*primer\s*$',
        r'(?m)^owner:\s*aleks-clark\s*$',
        r'(?m)^repository:\s*https://github\.com/aleksclark/primer\s*$',
        r'(?m)^ref_policy:\s*signed-default-branch-commit\s*$',
        r'(?m)^namespace:\s*default\s*$',
        r'(?m)^datacenters:\s*\[home\]\s*$',
        r'(?m)^\s*name:\s*primer\s*$',
        r'(?m)^\s*id:\s*primer\s*$',
        r'(?m)^\s*id:\s*primer-tv\s*$',
        r'(?m)^\s*id:\s*content-ingest\s*$',
        r'(?m)^\s*spec:\s*jobs/primer\.nomad\.hcl\s*$',
        r'(?m)^\s*spec:\s*jobs/primer-tv\.nomad\.hcl\s*$',
        r'(?m)^\s*spec:\s*jobs/content-ingest\.nomad\.hcl\s*$',
        r'(?m)^\s*env:\s*env/home\.nomadvars\.hcl\s*$',
        r'(?m)^\s*images:\s*images\.lock\.hcl\s*$',
        r'(?m)^\s*-\s*nomad/jobs/primer\s*$',
        r'(?m)^\s*-\s*nomad/jobs/primer-tv\s*$',
        r'(?m)^\s*-\s*nomad/jobs/content-ingest\s*$',
        r'(?m)^\s*rollout:\s*serial\s*$',
        r'(?m)^\s*prune:\s*explicit-only\s*$',
    ]
    for pat in need:
        if not re.search(pat, text):
            raise SystemExit(f'manifest missing pattern: {pat}')
    # forbid unknown dangerous keys / absolute paths
    if re.search(r'(?m)^\s*spec:\s*/', text):
        raise SystemExit('absolute spec path forbidden')
    if '..' in text:
        raise SystemExit('path traversal forbidden in deployment.yaml')
    print('deployment.yaml structural OK')
    raise SystemExit(0)

# Full parse path
assert data.get('schema_version') == 1
assert data.get('project') == 'primer'
assert data.get('owner') == 'aleks-clark'
assert data.get('repository') == 'https://github.com/aleksclark/primer'
assert data.get('ref_policy') == 'signed-default-branch-commit'
assert data.get('namespace') == 'default'
assert data.get('datacenters') == ['home']
rss = data.get('release_sets') or []
assert len(rss) == 1
rs = rss[0]
assert rs.get('name') == 'primer'
assert rs.get('env') == 'env/home.nomadvars.hcl'
assert rs.get('images') == 'images.lock.hcl'
assert rs.get('rollout') == 'serial'
assert rs.get('prune') == 'explicit-only'
assert rs.get('variable_paths') == [
    'nomad/jobs/primer',
    'nomad/jobs/primer-tv',
    'nomad/jobs/content-ingest',
]
jobs = rs.get('jobs') or []
assert [(j['id'], j['spec']) for j in jobs] == [
    ('primer', 'jobs/primer.nomad.hcl'),
    ('primer-tv', 'jobs/primer-tv.nomad.hcl'),
    ('content-ingest', 'jobs/content-ingest.nomad.hcl'),
]
print('deployment.yaml parse OK')
PY
pass "deployment.yaml"

# ── images.lock digest form ─────────────────────────────────────────────────
LOCK="${ROOT}/images.lock.hcl"
for var in image_primer image_primer_tv image_content_ingest; do
  line="$(grep -E "^${var}[[:space:]]*=" "${LOCK}" || true)"
  [[ -n "${line}" ]] || fail "images.lock missing ${var}"
  val="$(sed -E 's/^[^=]+=[[:space:]]*"([^"]+)".*/\1/' <<<"${line}")"
  [[ "${val}" =~ ${DIGEST_RE} ]] || fail "${var} not digest form: ${val}"
  # reject latest / bare tags as authority
  [[ "${val}" != *":latest"* ]] || fail "${var} uses :latest"
  [[ "${val}" != *"@sha256:PLACEHOLDER"* ]] || fail "${var} is placeholder"
done
pass "images.lock digests"

# ── jobspecs: identity, meta, nomadVar, no shell secrets, digest vars ───────
check_job() {
  local id="$1" path="$2" image_var="$3" nomad_var="$4"
  local f="${ROOT}/${path}"
  [[ -f "${f}" ]] || fail "missing job ${path}"
  grep -qE "^job \"${id}\" \{" "${f}" || fail "${path}: job id mismatch"
  grep -q 'managed_by[[:space:]]*=[[:space:]]*"fleet-pull-reconciler"' "${f}" \
    || fail "${path}: managed_by meta"
  grep -q 'deployment_owner[[:space:]]*=[[:space:]]*"aleks-clark"' "${f}" \
    || fail "${path}: deployment_owner meta"
  grep -q 'release_set[[:space:]]*=[[:space:]]*"primer"' "${f}" \
    || fail "${path}: release_set meta"
  grep -q "source_path[[:space:]]*=[[:space:]]*\"deploy/nomad/${path}\"" "${f}" \
    || fail "${path}: source_path meta"
  grep -q "nomadVar \"${nomad_var}\"" "${f}" || fail "${path}: nomadVar ${nomad_var}"
  grep -q "var.${image_var}" "${f}" || fail "${path}: image var ${image_var}"
  # no shell ${ENV} placeholders for secrets/images
  if grep -nE '\$\{(IMAGE_TAG|DATABASE_URL|SERVICE_TOKEN|TV_|INGEST_[A-Z0-9_]*KEY|INGEST_[A-Z0-9_]*TOKEN)' "${f}"; then
    fail "${path}: shell \${ENV} placeholders remain"
  fi
  if grep -nE 'image[[:space:]]*=[[:space:]]*"[^"]*:latest"' "${f}"; then
    fail "${path}: :latest image"
  fi
  if grep -nE 'image[[:space:]]*=[[:space:]]*"[^"@]+"' "${f}"; then
    fail "${path}: non-variable / non-digest image literal"
  fi
  # no secret-looking literals
  if grep -nEi "${SECRET_VALUE_RE}" "${f}"; then
    fail "${path}: possible secret literal"
  fi
  pass "job ${id}"
}

check_job primer jobs/primer.nomad.hcl image_primer nomad/jobs/primer
check_job primer-tv jobs/primer-tv.nomad.hcl image_primer_tv nomad/jobs/primer-tv
check_job content-ingest jobs/content-ingest.nomad.hcl image_content_ingest nomad/jobs/content-ingest

# content-ingest periodic invariants
CI="${ROOT}/jobs/content-ingest.nomad.hcl"
ENVF="${ROOT}/env/home.nomadvars.hcl"
grep -q 'type[[:space:]]*=[[:space:]]*"batch"' "${CI}" || fail "content-ingest must be batch"
grep -q 'periodic[[:space:]]*{' "${CI}" || fail "content-ingest must be periodic"
grep -q 'prohibit_overlap[[:space:]]*=[[:space:]]*true' "${CI}" || fail "prohibit_overlap required"
grep -q 'moosefs-media' "${CI}" || fail "content-ingest moosefs-media volume"
grep -qi 'pause' "${ROOT}/README.md" || fail "README must document pause-before-handoff"
grep -q 'never-from-reconciler-or-ci' "${CI}" || fail "dispatch policy meta missing"
# YouTube root must not be nested under /media/tv (Jellyfin would resolve the
# container folder as one Series). Register /media/primer/Shows as a media path.
grep -qE 'default[[:space:]]*=[[:space:]]*"/media/primer"' "${CI}" \
  || fail "content-ingest ytdlp output default must be /media/primer"
grep -qE 'content_ingest_ytdlp_output_dir[[:space:]]*=[[:space:]]*"/media/primer"' "${ENVF}" \
  || fail "home.nomadvars ytdlp output must be /media/primer"
grep -q 'INGEST_JELLYFIN_COLLECTION_NAME.*var.content_ingest_jellyfin_collection_name' "${CI}" \
  || fail "content-ingest must wire the named Jellyfin Collection"
grep -qE 'content_ingest_jellyfin_collection_name[[:space:]]*=[[:space:]]*"Primer"' "${ENVF}" \
  || fail "home.nomadvars must select the Primer Collection"
grep -qE 'kill_timeout[[:space:]]*=[[:space:]]*"4h"' "${CI}" \
  || fail "content-ingest kill_timeout must be 4h"
grep -q 'INGEST_YTDLP_COOKIES_PATH' "${CI}" \
  || fail "content-ingest must declare INGEST_YTDLP_COOKIES_PATH env (path only)"
if grep -nE '/data/media' "${CI}" "${ENVF}" "${REPO_ROOT}/deploy/.env.example" 2>/dev/null; then
  fail "legacy /data/media path must not remain in content-ingest deploy surface"
fi
pass "content-ingest periodic + handoff docs"

# env overlay must not hold secret keys/values
if grep -nEi '(password|api_key|service_token|admin_key|database_url)\s*=' "${ENVF}"; then
  fail "env overlay contains secret-shaped keys"
fi
if grep -nEi "${SECRET_VALUE_RE}" "${ENVF}"; then
  fail "env overlay possible secret literal"
fi
pass "env overlay non-secret"

# expected-services.json sanity
python3 - <<'PY' "${ROOT}/tests/expected-services.json" || fail "expected-services.json"
import json, sys
from pathlib import Path
data = json.loads(Path(sys.argv[1]).read_text())
assert data['release_set'] == 'primer'
ids = [j['id'] for j in data['jobs']]
assert ids == ['primer', 'primer-tv', 'content-ingest'], ids
for j in data['jobs']:
    assert j['secret_variable_path'].startswith('nomad/jobs/')
    assert j['secret_keys'], j['id']
    assert j['image_var'].startswith('image_')
ci = data['jobs'][2]
assert ci['periodic']['prohibit_overlap'] is True
assert ci['dispatch'] == 'never-from-reconciler-or-ci'
print('expected-services OK')
PY
pass "expected-services.json"

# deploy.sh must refuse production submit
DS="${REPO_ROOT}/deploy/deploy.sh"
[[ -x "${DS}" || -f "${DS}" ]] || fail "deploy/deploy.sh missing"
grep -qE 'refuses production|fleet pull reconciler' "${DS}" \
  || fail "deploy.sh must refuse production submit and point at reconciler"
# No executable nomad job run/dispatch (allow documentation strings only).
if grep -nE '^[[:space:]]*nomad[[:space:]]+job[[:space:]]+(run|dispatch)\b' "${DS}"; then
  fail "deploy.sh still invokes nomad job run/dispatch"
fi
# Allow documentation mentions of envsubst; forbid executable use.
if grep -nE '^[[:space:]]*envsubst\b' "${DS}"; then
  fail "deploy.sh still uses envsubst"
fi
# Positive refuse behavior: default/legacy targets must exit non-zero
if bash "${DS}" all >/tmp/primer-deploy-refuse.out 2>&1; then
  fail "deploy.sh all must refuse (exit non-zero)"
fi
grep -qiE 'refuse|reconciler|deploy/nomad' /tmp/primer-deploy-refuse.out \
  || fail "deploy.sh refuse message missing reconciler pointer"
pass "deploy.sh refuses production write"

# no deploy/.env committed
if git -C "${REPO_ROOT}" ls-files --error-unmatch deploy/.env >/dev/null 2>&1; then
  fail "deploy/.env must not be tracked"
fi
pass "deploy/.env untracked"

# optional nomad fmt/validate
if command -v nomad >/dev/null 2>&1; then
  nomad fmt -check "${ROOT}/jobs" || fail "nomad fmt -check"
  for spec in primer primer-tv content-ingest; do
    nomad job validate \
      -var-file="${ROOT}/images.lock.hcl" \
      -var-file="${ROOT}/env/home.nomadvars.hcl" \
      "${ROOT}/jobs/${spec}.nomad.hcl" >/dev/null \
      || fail "nomad job validate ${spec}"
  done
  pass "nomad fmt + validate"
else
  echo "SKIP: nomad binary not available for L0 validate"
fi

echo "==> all primer plan-03 contract checks passed"
