#!/usr/bin/env bash
# Fail-closed validation for Primer Stacklane docker-compose.yml.
# Uses `docker compose config --format json` into a mode-0700 temp file; never dumps full env/secrets.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PROJECT_SLUG="primer"

die() { echo "compose-dev-check: FAIL: $*" >&2; exit 1; }
ok() { echo "compose-dev-check: ok: $*" >&2; }
info() { echo "compose-dev-check: $*" >&2; }

# Do not inherit an operator's compose file/profile/project accidentally.
unset COMPOSE_FILE COMPOSE_PROFILES COMPOSE_PROJECT_NAME || true
COMPOSE_FILE="${ROOT}/docker-compose.yml"

sanitize_instance() {
  local s="${1:-}"
  s="$(printf '%s' "$s" | tr '[:upper:]' '[:lower:]')"
  s="$(printf '%s' "$s" | sed -E 's/[^a-z0-9-]+/-/g; s/-+/-/g; s/^-+//; s/-+$//')"
  if [[ ${#s} -gt 48 ]]; then
    s="${s:0:48}"
    s="$(printf '%s' "$s" | sed -E 's/-+$//')"
  fi
  [[ -z "$s" ]] && s="dev"
  printf '%s' "$s"
}

derive_instance() {
  if [[ -n "${STACKLANE_INSTANCE:-}" ]]; then
    sanitize_instance "$STACKLANE_INSTANCE"
    return
  fi
  sanitize_instance "$(basename "$ROOT")"
}

require_tools() {
  command -v docker >/dev/null 2>&1 || die "docker not found"
  docker compose version >/dev/null 2>&1 || die "docker compose not available"
  command -v python3 >/dev/null 2>&1 || die "python3 required for JSON parse"
  [[ -f "$COMPOSE_FILE" ]] || die "missing $COMPOSE_FILE"
}

render_config() {
  local project="$1"
  local instance="$2"
  local out="$3"
  local base_dom="${STACKLANE_BASE_DOMAIN:-test}"
  umask 077
  : >"$out"
  chmod 600 "$out"
  if ! STACKLANE_INSTANCE="$instance" \
    STACKLANE_BASE_DOMAIN="$base_dom" \
    LMS_CORS_ORIGINS="${LMS_CORS_ORIGINS:-http://web.${instance}.${PROJECT_SLUG}.${base_dom}:3000}" \
    TV_CORS_ORIGINS="${TV_CORS_ORIGINS:-http://tv-web.${instance}.${PROJECT_SLUG}.${base_dom}:3001}" \
    COMPOSE_PROJECT_NAME="$project" \
    docker compose -p "$project" --project-directory "$ROOT" -f "$COMPOSE_FILE" config --format json >"$out"; then
    die "compose config render failed (rule: render)"
  fi
  chmod 600 "$out"
}

run_mutation_probes() {
  local tmpdir="$1"
  local base_yml="$COMPOSE_FILE"
  python3 - "$base_yml" "$tmpdir" "$ROOT" <<'PY'
import json, os, pathlib, subprocess, sys

base_path = pathlib.Path(sys.argv[1])
tmpdir = pathlib.Path(sys.argv[2])
root = pathlib.Path(sys.argv[3])
src = base_path.read_text()

def write_mut(name, text):
    p = tmpdir / f"mut-{name}.yml"
    p.write_text(text)
    return p

mutations = [
    ("wildcard-host", src.replace('"127.0.0.1::8080"', '"0.0.0.0::8080"'), "host_ip must be 127.0.0.1"),
    ("fixed-host-port", src.replace('"127.0.0.1::8080"', '"127.0.0.1:18080:8080"'), "published port must be ephemeral"),
    ("missing-enable-label", src.replace('\n      stacklane.enable: "true"\n', '\n', 1), "stacklane.enable"),
    ("no-source-mount", src.replace("- .:/src\n", "", 1), "source bind mount"),
]

failures = 0
for name, text, expect_hint in mutations:
    mut_path = write_mut(name, text)
    out = tmpdir / f"mut-{name}.json"
    env = os.environ.copy()
    env.pop("COMPOSE_FILE", None)
    env.pop("COMPOSE_PROFILES", None)
    env["STACKLANE_INSTANCE"] = "mutprobe"
    env["STACKLANE_BASE_DOMAIN"] = "test"
    env["COMPOSE_PROJECT_NAME"] = "primer-mutprobe"
    try:
        with out.open("w") as fh:
            subprocess.run(
                ["docker", "compose", "-p", "primer-mutprobe",
                 "--project-directory", str(root), "-f", str(mut_path),
                 "config", "--format", "json"],
                check=True, env=env, stdout=fh, stderr=subprocess.DEVNULL, timeout=60,
            )
    except Exception:
        print(f"mutation {name}: config rejected (ok)", file=sys.stderr)
        continue
    try:
        cfg = json.loads(out.read_text())
    except Exception as e:
        print(f"mutation {name}: invalid json {e}", file=sys.stderr)
        failures += 1
        continue
    services = cfg.get("services") or {}
    bad = False
    reason = ""
    if name == "wildcard-host":
        for svc, sc in services.items():
            for p in sc.get("ports") or []:
                hip = p.get("host_ip") or ""
                if hip != "127.0.0.1":
                    bad = True
                    reason = f"{svc} host_ip={hip!r}"
    elif name == "fixed-host-port":
        for svc, sc in services.items():
            for p in sc.get("ports") or []:
                pub = p.get("published")
                if pub not in (None, "", 0, "0"):
                    bad = True
                    reason = f"{svc} published={pub!r}"
    elif name == "missing-enable-label":
        for svc, sc in services.items():
            labels = sc.get("labels") or {}
            if isinstance(labels, list):
                kv = {}
                for item in labels:
                    if isinstance(item, str) and "=" in item:
                        k, v = item.split("=", 1)
                        kv[k] = v
                labels = kv
            if str(labels.get("stacklane.enable", "")).lower() not in ("true", "1"):
                bad = True
                reason = f"{svc} missing enable"
                break
    elif name == "no-source-mount":
        api = services.get("api") or {}
        vols = api.get("volumes") or []
        has_src = False
        for v in vols:
            if isinstance(v, dict):
                tgt = v.get("target") or v.get("destination") or ""
                typ = v.get("type") or ""
                if tgt in ("/src",) and typ == "bind":
                    has_src = True
            elif isinstance(v, str) and ":/src" in v:
                has_src = True
        if not has_src:
            bad = True
            reason = "api missing /src bind"
    if not bad:
        print(f"mutation {name}: FAIL expected defect not detected ({expect_hint})", file=sys.stderr)
        failures += 1
    else:
        print(f"mutation {name}: defect detected ({reason}) ok", file=sys.stderr)

if failures:
    sys.exit(2)
print("mutation probes: all defects detected", file=sys.stderr)
sys.exit(0)
PY
}

validate_rendered() {
  local json_path="$1"
  local expect_instance="$2"
  local expect_project="$3"
  python3 - "$json_path" "$expect_instance" "$expect_project" <<'PY'
import json, re, sys

path, expect_instance, expect_project = sys.argv[1], sys.argv[2], sys.argv[3]
with open(path, "r", encoding="utf-8") as f:
    cfg = json.load(f)

errors = []

def err(msg):
    errors.append(msg)

services = cfg.get("services") or {}
required_services = ("postgres", "api", "tv", "web", "tv-web", "investor")
for name in required_services:
    if name not in services:
        err(f"missing service {name}")

rendered_name = cfg.get("name") or ""
if rendered_name and rendered_name != expect_project:
    err(f"compose name {rendered_name!r} != expected project {expect_project!r}")

volumes_top = cfg.get("volumes") or {}
named_required = {
    "primer_pg_data",
    "primer_go_mod_cache",
    "primer_lms_go_cache",
    "primer_tv_go_cache",
    "primer_lms_air_tmp",
    "primer_tv_air_tmp",
    "primer_web_node_modules",
    "primer_tv_web_node_modules",
    "primer_investor_node_modules",
    "primer_npm_cache",
}
vol_keys = set(volumes_top.keys())
for req in named_required:
    if req not in vol_keys and not any(k.endswith(req) or k == req for k in vol_keys):
        err(f"missing named volume {req}")

def labels_map(sc):
    labels = sc.get("labels") or {}
    if isinstance(labels, list):
        out = {}
        for item in labels:
            if isinstance(item, str) and "=" in item:
                k, v = item.split("=", 1)
                out[k] = v
        return out
    if isinstance(labels, dict):
        return {str(k): str(v) for k, v in labels.items()}
    return {}

def check_ports(svc_name, sc, expect_target):
    ports = sc.get("ports") or []
    if not ports:
        err(f"{svc_name}: no published ports")
        return
    for p in ports:
        if not isinstance(p, dict):
            err(f"{svc_name}: port entry not object")
            continue
        hip = p.get("host_ip")
        if hip != "127.0.0.1":
            err(f"{svc_name}: host_ip must be 127.0.0.1")
        published = p.get("published")
        if published not in (None, "", 0, "0"):
            err(f"{svc_name}: published must be ephemeral empty/0")
        target = p.get("target")
        if int(target) != int(expect_target):
            err(f"{svc_name}: target port want {expect_target}")

def check_isolation(svc_name, sc):
    if (sc.get("network_mode") or "") == "host":
        err(f"{svc_name}: network_mode=host forbidden")
    pid = sc.get("pid")
    if pid is not None and str(pid).strip().lower() in ("host", '"host"'):
        err(f"{svc_name}: pid=host forbidden")
    priv = sc.get("privileged")
    if priv is True or (isinstance(priv, str) and priv.strip().lower() in ("true", "1", "yes", "on")):
        err(f"{svc_name}: privileged=true forbidden")

def volume_entries(sc):
    return sc.get("volumes") or []

def has_bind(sc, target_suffix):
    for v in volume_entries(sc):
        if isinstance(v, dict):
            tgt = v.get("target") or v.get("destination") or ""
            typ = (v.get("type") or "").lower()
            if typ == "bind" and (tgt == target_suffix or tgt.endswith(target_suffix)):
                return True
        elif isinstance(v, str):
            if v.rstrip("/").endswith(":" + target_suffix.rstrip("/")) or f":{target_suffix}" in v:
                return True
    return False

def has_named(sc, name_part):
    for v in volume_entries(sc):
        if isinstance(v, dict):
            src = str(v.get("source") or "")
            if name_part in src:
                return True
        elif isinstance(v, str) and name_part in v:
            return True
    return False

def as_env(raw):
    if isinstance(raw, list):
        out = {}
        for e in raw:
            if isinstance(e, str):
                k, _, v = e.partition("=")
                out[k] = v
        return out
    if isinstance(raw, dict):
        return {str(k): str(v) for k, v in raw.items()}
    return {}

def check_labels(svc_name, sc, endpoint, public_port, target_port=None):
    labels = labels_map(sc)
    want = {
        "stacklane.enable": "true",
        "stacklane.project": "primer",
        "stacklane.instance": expect_instance,
        "stacklane.endpoint": endpoint,
        "stacklane.port": str(public_port),
    }
    if target_port is not None:
        want["stacklane.target_port"] = str(target_port)
    for k, expected in want.items():
        got = str(labels.get(k, ""))
        if got != expected:
            err(f"{svc_name} label {k} mismatch")
    enable = str(labels.get("stacklane.enable", ""))
    if enable not in ("true", "1"):
        err(f"{svc_name} stacklane.enable must be true or 1")
    if str(labels.get("stacklane.port", "")) == "80":
        err(f"{svc_name} public stacklane.port must not be 80")

def require_health(svc_name, sc, needle):
    hc = sc.get("healthcheck") or {}
    test = hc.get("test") or []
    test_s = test if isinstance(test, str) else " ".join(str(x) for x in test)
    if needle not in test_s:
        err(f"{svc_name} healthcheck must mention {needle}")

specs = {
    "postgres": {"target": 5432, "endpoint": "postgres", "public": 5432},
    "api": {"target": 8080, "endpoint": "api", "public": 8080},
    "tv": {"target": 8081, "endpoint": "tv", "public": 8081},
    "web": {"target": 5173, "endpoint": "web", "public": 3000, "target_port": 5173},
    "tv-web": {"target": 5173, "endpoint": "tv-web", "public": 3001, "target_port": 5173},
    "investor": {"target": 5173, "endpoint": "investor", "public": 3002, "target_port": 5173},
}

for svc_name, spec in specs.items():
    sc = services.get(svc_name) or {}
    check_ports(svc_name, sc, spec["target"])
    check_isolation(svc_name, sc)
    check_labels(svc_name, sc, spec["endpoint"], spec["public"], spec.get("target_port"))
    if not (sc.get("healthcheck") or {}):
        err(f"{svc_name}: missing healthcheck")

postgres = services.get("postgres") or {}
if not has_named(postgres, "primer_pg_data"):
    err("postgres: missing named volume primer_pg_data")
require_health("postgres", postgres, "pg_isready")

api = services.get("api") or {}
if not has_bind(api, "/src"):
    err("api: missing source bind mount to /src")
for nv in ("primer_go_mod_cache", "primer_lms_go_cache", "primer_lms_air_tmp"):
    if not has_named(api, nv):
        err(f"api: missing named volume involving {nv}")
api_env = as_env(api.get("environment") or {})
if not str(api_env.get("DATABASE_URL", "")).startswith("postgres://primer:primer@postgres:5432/primer"):
    err("api DATABASE_URL must use Compose DNS postgres:5432/primer")
if api_env.get("HOST") != "0.0.0.0":
    err("api HOST must be 0.0.0.0")
if api_env.get("PORT") != "8080":
    err("api PORT must be 8080")
if api_env.get("PRIMER_AGENTS_ENABLED") not in ("false", "0"):
    err("api PRIMER_AGENTS_ENABLED must stay false")
if api_env.get("AGENT_RUNTIME_ENABLED") not in ("false", "0"):
    err("api AGENT_RUNTIME_ENABLED must stay false")
require_health("api", api, "/api/v1/health")

tv = services.get("tv") or {}
if not has_bind(tv, "/src"):
    err("tv: missing source bind mount to /src")
for nv in ("primer_go_mod_cache", "primer_tv_go_cache", "primer_tv_air_tmp"):
    if not has_named(tv, nv):
        err(f"tv: missing named volume involving {nv}")
tv_env = as_env(tv.get("environment") or {})
if not str(tv_env.get("TV_DATABASE_URL", "")).startswith("postgres://primer:primer@postgres:5432/primer_tv"):
    err("tv TV_DATABASE_URL must use Compose DNS postgres:5432/primer_tv")
if tv_env.get("TV_PRIMER_BASE_URL") != "http://api:8080":
    err("tv TV_PRIMER_BASE_URL must be http://api:8080")
if tv_env.get("TV_HOST") != "0.0.0.0":
    err("tv TV_HOST must be 0.0.0.0")
if tv_env.get("TV_PORT") != "8081":
    err("tv TV_PORT must be 8081")
require_health("tv", tv, "/api/v1/health")

web = services.get("web") or {}
if not has_bind(web, "/src"):
    err("web: missing source bind mount to /src")
if not has_named(web, "primer_web_node_modules"):
    err("web: missing named volume primer_web_node_modules")
web_env = as_env(web.get("environment") or {})
if web_env.get("VITE_API_PROXY_TARGET") != "http://api:8080":
    err("web VITE_API_PROXY_TARGET must be http://api:8080")
require_health("web", web, "5173")

tv_web = services.get("tv-web") or {}
if not has_bind(tv_web, "/src"):
    err("tv-web: missing source bind mount to /src")
if not has_named(tv_web, "primer_tv_web_node_modules"):
    err("tv-web: missing named volume primer_tv_web_node_modules")
tv_web_env = as_env(tv_web.get("environment") or {})
if tv_web_env.get("VITE_API_PROXY_TARGET") != "http://tv:8081":
    err("tv-web VITE_API_PROXY_TARGET must be http://tv:8081")
require_health("tv-web", tv_web, "5173")

investor = services.get("investor") or {}
if not has_bind(investor, "/src"):
    err("investor: missing source bind mount to /src")
if not has_named(investor, "primer_investor_node_modules"):
    err("investor: missing named volume primer_investor_node_modules")
require_health("investor", investor, "5173")

blob = json.dumps({
    "api": api_env,
    "tv": tv_env,
    "web": web_env,
    "tv-web": tv_web_env,
    "investor": as_env(investor.get("environment") or {}),
    "labels": [labels_map(services.get(name) or {}) for name in required_services],
})
if ".local" in blob.lower():
    err("compose-derived config must not use .local domains")

secret_keys = (
    "SERVICE_TOKEN",
    "TV_ADMIN_API_KEY",
    "TV_PRIMER_SERVICE_TOKEN",
    "TV_JELLYFIN_API_KEY",
    "TUTOR_BEDROCK_API_KEY",
    "AGENT_RUNTIME_API_KEY",
    "OPENAI_API_KEY",
    "IDENTITY_JWKS_URL",
)
env_maps = (api_env, tv_env, web_env, tv_web_env, as_env(investor.get("environment") or {}))
for env in env_maps:
    for secret_key in secret_keys:
        if secret_key in env and str(env.get(secret_key) or "").strip():
            err(f"env must not include {secret_key}")

if errors:
    for e in errors:
        print(f"ERROR: {e}", file=sys.stderr)
    sys.exit(1)
print(f"validated instance={expect_instance} project={expect_project}", file=sys.stderr)
sys.exit(0)
PY
}

main() {
  require_tools
  local instance project
  instance="$(derive_instance)"
  project="${PROJECT_SLUG}-${instance}"
  export STACKLANE_INSTANCE="$instance"
  export STACKLANE_BASE_DOMAIN="${STACKLANE_BASE_DOMAIN:-test}"

  umask 077
  COMPOSE_CHECK_TMPDIR="$(mktemp -d "${TMPDIR:-/tmp}/primer-compose-check.XXXXXX")"
  chmod 700 "$COMPOSE_CHECK_TMPDIR"
  # shellcheck disable=SC2064
  trap 'rm -rf "${COMPOSE_CHECK_TMPDIR:-}"' EXIT INT TERM

  local cfg1 cfg2
  cfg1="${COMPOSE_CHECK_TMPDIR}/compose-${instance}.json"
  info "rendering compose config for instance=${instance} project=${project}"
  render_config "$project" "$instance" "$cfg1"
  validate_rendered "$cfg1" "$instance" "$project"
  ok "default instance path (${instance})"

  local alt="slc-altcheck"
  local alt_project="${PROJECT_SLUG}-${alt}"
  cfg2="${COMPOSE_CHECK_TMPDIR}/compose-${alt}.json"
  STACKLANE_INSTANCE="$alt" render_config "$alt_project" "$alt" "$cfg2"
  validate_rendered "$cfg2" "$alt" "$alt_project"
  if cmp -s "$cfg1" "$cfg2"; then
    die "two instances produced identical rendered configs"
  fi
  ok "override instance path differs (${alt})"

  info "running mutation probes (fail-closed)"
  run_mutation_probes "$COMPOSE_CHECK_TMPDIR"
  ok "mutation probes"

  ok "Postgres DSN: compose-internal postgres://primer:primer@postgres:5432/{primer,primer_tv}"
  ok "all compose-dev checks passed"
}

main "$@"
