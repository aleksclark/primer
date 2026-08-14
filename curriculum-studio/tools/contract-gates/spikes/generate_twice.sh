#!/usr/bin/env bash
# Deterministic double-generate for C3 qualification (E3-07).
# Production-intended path: `buf generate` using the committed contracts/buf.gen.yaml
# **local** plugin pins (protoc-gen-go v1.36.11, protoc-gen-go-grpc v1.5.1) resolved
# via contracts/scripts/bootstrap_local_plugins.sh into an ignored private bin.
# Outputs only under build-only / gitignored paths. No BSR remote plugins.
set -euo pipefail

SPIKES_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
STUDIO_ROOT="$(cd "$SPIKES_ROOT/../../.." && pwd)"
CONTRACTS="$STUDIO_ROOT/contracts"
OUT_ROOT="${SPIKE_OUT:-$SPIKES_ROOT/.tmp}"
EVIDENCE="$SPIKES_ROOT/evidence"
PROTO_OUT="$OUT_ROOT/proto-gen"
TS_OUT="$OUT_ROOT/ts-gen"
OPENAPI="$CONTRACTS/openapi/v1/curriculum-studio.yaml"
BUF_GEN_YAML="$CONTRACTS/buf.gen.yaml"
BOOTSTRAP="$CONTRACTS/scripts/bootstrap_local_plugins.sh"

# Exact pins from committed bootstrap + buf.gen.yaml (must match generated headers).
EXPECTED_PGO_VER="v1.36.11"
EXPECTED_PGGRPC_VER="v1.5.1"
EXPECTED_BUF_MAJOR_MINOR="1.72"
EXPECTED_OPENAPI_TS="7.13.0"
EXPECTED_TSC="5.9.2"

rm -rf "$OUT_ROOT"
mkdir -p "$PROTO_OUT" "$TS_OUT" "$EVIDENCE"

fail() { echo "FAIL: $*" >&2; exit 1; }

command -v buf >/dev/null || fail "buf required (pinned path uses buf generate)"
command -v python3 >/dev/null || fail "python3 required"
command -v npx >/dev/null || fail "npx required"
command -v go >/dev/null || fail "go required"
[[ -f "$BUF_GEN_YAML" ]] || fail "missing committed buf.gen.yaml: $BUF_GEN_YAML"
[[ -f "$CONTRACTS/buf.yaml" ]] || fail "missing committed buf.yaml"
[[ -x "$BOOTSTRAP" || -f "$BOOTSTRAP" ]] || fail "missing bootstrap script: $BOOTSTRAP"

digest_tree() {
  local dir="$1"
  (
    cd "$dir"
    find . -type f | LC_ALL=C sort | while read -r f; do
      printf '%s ' "$f"
      sha256sum "$f" | awk '{print $1}'
    done
  ) | sha256sum | awk '{print $1}'
}

# Normalize generated Go for local spike module import path.
# Proto go_package ends with /v1 which is an invalid standalone module path under
# Go major-version import rules; spikes import via spike.local/gen instead.
normalize_gen_go() {
  local dest="$1"
  mkdir -p "$dest"
  # Flatten curriculumstudio/v1/*.go → dest/
  if [[ -d "$dest/curriculumstudio/v1" ]]; then
    mv "$dest/curriculumstudio/v1/"*.go "$dest/" 2>/dev/null || true
    rm -rf "$dest/curriculumstudio"
  fi
  # Drop any accidental go.mod inside
  rm -f "$dest/go.mod" "$dest/go.sum"
}

# Require committed buf.gen.yaml to declare exact local plugins (not BSR remote).
assert_buf_gen_pins() {
  if grep -Eq '^[[:space:]]*-[[:space:]]*remote:' "$BUF_GEN_YAML"; then
    fail "buf.gen.yaml must use local plugins (found remote:). BSR remote plugins are not reproducible under rate limits."
  fi
  local local_count
  local_count="$(grep -Ec '^[[:space:]]*-[[:space:]]*local:[[:space:]]*protoc-gen-go(-grpc)?[[:space:]]*$' "$BUF_GEN_YAML" || true)"
  [[ "$local_count" -ge 2 ]] || \
    fail "buf.gen.yaml must declare local: protoc-gen-go and local: protoc-gen-go-grpc"
  if ! grep -Eq '^[[:space:]]*-[[:space:]]*local:[[:space:]]*protoc-gen-go[[:space:]]*$' "$BUF_GEN_YAML"; then
    fail "buf.gen.yaml missing local: protoc-gen-go"
  fi
  if ! grep -Eq '^[[:space:]]*-[[:space:]]*local:[[:space:]]*protoc-gen-go-grpc[[:space:]]*$' "$BUF_GEN_YAML"; then
    fail "buf.gen.yaml missing local: protoc-gen-go-grpc"
  fi
  # Comments must document exact pins (single source with bootstrap).
  if ! grep -Fq "protoc-gen-go@${EXPECTED_PGO_VER}" "$BUF_GEN_YAML" && \
     ! grep -Fq "protoc-gen-go ${EXPECTED_PGO_VER}" "$BUF_GEN_YAML" && \
     ! grep -Fq "protoc-gen-go@v1.36.11" "$BUF_GEN_YAML"; then
    # Accept either @pin or plain pin in comments
    if ! grep -Eq "protoc-gen-go.*${EXPECTED_PGO_VER}" "$BUF_GEN_YAML"; then
      fail "buf.gen.yaml must document protoc-gen-go ${EXPECTED_PGO_VER}"
    fi
  fi
  if ! grep -Eq "protoc-gen-go-grpc.*${EXPECTED_PGGRPC_VER}" "$BUF_GEN_YAML"; then
    fail "buf.gen.yaml must document protoc-gen-go-grpc ${EXPECTED_PGGRPC_VER}"
  fi
}

assert_buf_cli_pin() {
  local ver
  ver="$(buf --version 2>&1 | tr -s '[:space:]' ' ' | sed 's/[[:space:]]*$//')"
  # Accept 1.72.x only (workspace pin documented in buf.gen.yaml).
  if [[ ! "$ver" =~ ^${EXPECTED_BUF_MAJOR_MINOR}\.[0-9]+$ ]]; then
    fail "buf CLI version '$ver' not in ${EXPECTED_BUF_MAJOR_MINOR}.x (required for qualified path)"
  fi
  printf '%s' "$ver"
}

# Assert generated headers match exact plugin pins (fail closed on drift).
assert_generated_plugin_headers() {
  local dest="$1"
  local f pgo_hdr grpc_hdr
  local count_go=0 count_grpc=0

  shopt -s nullglob
  for f in "$dest"/*.pb.go; do
    [[ "$f" == *_grpc.pb.go ]] && continue
    count_go=$((count_go + 1))
    pgo_hdr="$(awk '/protoc-gen-go v/{print; exit}' "$f" | sed -E 's/.*protoc-gen-go[[:space:]]+//')"
    pgo_hdr="$(printf '%s' "$pgo_hdr" | tr -d '[:space:]')"
    [[ "$pgo_hdr" == "$EXPECTED_PGO_VER" ]] || \
      fail "generated header pin mismatch in $(basename "$f"): protoc-gen-go '$pgo_hdr' want $EXPECTED_PGO_VER"
  done
  for f in "$dest"/*_grpc.pb.go; do
    count_grpc=$((count_grpc + 1))
    grpc_hdr="$(awk '/protoc-gen-go-grpc v/{print; exit}' "$f" | sed -E 's/.*protoc-gen-go-grpc[[:space:]]+//')"
    grpc_hdr="$(printf '%s' "$grpc_hdr" | tr -d '[:space:]')"
    [[ "$grpc_hdr" == "$EXPECTED_PGGRPC_VER" ]] || \
      fail "generated header pin mismatch in $(basename "$f"): protoc-gen-go-grpc '$grpc_hdr' want $EXPECTED_PGGRPC_VER"
  done
  shopt -u nullglob

  ((count_go > 0)) || fail "no *.pb.go produced by buf generate"
  ((count_grpc > 0)) || fail "no *_grpc.pb.go produced by buf generate"
}

# Bootstrap private plugin bin (exact Go module pins). Fail closed — no ambient fallback.
bootstrap_plugins() {
  local out
  out="$(bash "$BOOTSTRAP")" || fail "bootstrap_local_plugins.sh failed (exact pins unavailable)"
  PLUGIN_BIN_DIR="$(printf '%s\n' "$out" | awk -F= '/^plugin_bin_dir=/{print substr($0,index($0,"=")+1); exit}')"
  PLUGIN_PGO_PATH="$(printf '%s\n' "$out" | awk -F= '/^plugin_protoc_gen_go_path=/{print substr($0,index($0,"=")+1); exit}')"
  PLUGIN_PGGRPC_PATH="$(printf '%s\n' "$out" | awk -F= '/^plugin_protoc_gen_go_grpc_path=/{print substr($0,index($0,"=")+1); exit}')"
  PLUGIN_PGO_MOD="$(printf '%s\n' "$out" | awk -F= '/^plugin_protoc_gen_go_module=/{print substr($0,index($0,"=")+1); exit}')"
  PLUGIN_PGGRPC_MOD="$(printf '%s\n' "$out" | awk -F= '/^plugin_protoc_gen_go_grpc_module=/{print substr($0,index($0,"=")+1); exit}')"
  PLUGIN_PGO_REPORTED="$(printf '%s\n' "$out" | awk -F= '/^plugin_protoc_gen_go_reported=/{print substr($0,index($0,"=")+1); exit}')"
  PLUGIN_PGGRPC_REPORTED="$(printf '%s\n' "$out" | awk -F= '/^plugin_protoc_gen_go_grpc_reported=/{print substr($0,index($0,"=")+1); exit}')"
  [[ -n "$PLUGIN_BIN_DIR" && -d "$PLUGIN_BIN_DIR" ]] || fail "bootstrap did not report plugin_bin_dir"
  [[ -x "$PLUGIN_PGO_PATH" ]] || fail "missing pinned protoc-gen-go at $PLUGIN_PGO_PATH"
  [[ -x "$PLUGIN_PGGRPC_PATH" ]] || fail "missing pinned protoc-gen-go-grpc at $PLUGIN_PGGRPC_PATH"
  # Stable evidence paths (relative to studio root when under the module tree).
  # Absolute host paths stay on stdout only so committed evidence is multi-machine stable.
  rel_from_studio() {
    local p="$1"
    if [[ "$p" == "$STUDIO_ROOT"/* ]]; then
      printf 'curriculum-studio/%s' "${p#"$STUDIO_ROOT"/}"
    else
      # Non-default cache (e.g. CURRICULUM_STUDIO_PLUGIN_CACHE) — keep basename pin key.
      printf '%s' "$p"
    fi
  }
  PLUGIN_BIN_DIR_EVIDENCE="$(rel_from_studio "$PLUGIN_BIN_DIR")"
  PLUGIN_PGO_PATH_EVIDENCE="$(rel_from_studio "$PLUGIN_PGO_PATH")"
  PLUGIN_PGGRPC_PATH_EVIDENCE="$(rel_from_studio "$PLUGIN_PGGRPC_PATH")"
  # Refuse ambient shims: private bin must be first and match reported paths.
  export PATH="$PLUGIN_BIN_DIR:$PATH"
  local which_pgo which_grpc
  which_pgo="$(command -v protoc-gen-go)"
  which_grpc="$(command -v protoc-gen-go-grpc)"
  [[ "$which_pgo" == "$PLUGIN_PGO_PATH" ]] || \
    fail "PATH resolution for protoc-gen-go is '$which_pgo' not private pin '$PLUGIN_PGO_PATH'"
  [[ "$which_grpc" == "$PLUGIN_PGGRPC_PATH" ]] || \
    fail "PATH resolution for protoc-gen-go-grpc is '$which_grpc' not private pin '$PLUGIN_PGGRPC_PATH'"
  # Re-check --version from PATH-resolved binaries.
  local got_pgo got_grpc
  got_pgo="$(protoc-gen-go --version 2>&1 | tr -d '[:space:]')"
  got_pgo="${got_pgo#protoc-gen-go}"
  [[ "$got_pgo" == "$EXPECTED_PGO_VER" ]] || \
    fail "PATH protoc-gen-go version '$got_pgo' want $EXPECTED_PGO_VER (no ambient fallback)"
  got_grpc="$(protoc-gen-go-grpc --version 2>&1 | tr -d '[:space:]')"
  got_grpc="${got_grpc#protoc-gen-go-grpc}"
  got_grpc="${got_grpc#v}"
  [[ "$got_grpc" == "${EXPECTED_PGGRPC_VER#v}" ]] || \
    fail "PATH protoc-gen-go-grpc version '$got_grpc' want ${EXPECTED_PGGRPC_VER#v} (no ambient fallback)"
}

# Production-intended path: isolated temp copy + `buf generate` with committed
# local-plugin buf.gen.yaml and private PATH. Never call host protoc directly;
# never invoke BSR remote plugins.
generate_proto_once() {
  local dest="$1"
  rm -rf "$dest"
  mkdir -p "$dest"

  local work
  work="$(mktemp -d "${TMPDIR:-/tmp}/c3-buf-gen.XXXXXX")"
  # shellcheck disable=SC2064
  trap "rm -rf -- '$work'" RETURN

  mkdir -p "$work"
  cp "$CONTRACTS/buf.yaml" "$work/buf.yaml"
  cp "$CONTRACTS/buf.gen.yaml" "$work/buf.gen.yaml"
  cp -a "$CONTRACTS/proto" "$work/proto"

  # Refuse accidental remote-plugin reintroduction in the temp copy.
  if grep -Eq '^[[:space:]]*-[[:space:]]*remote:' "$work/buf.gen.yaml"; then
    fail "buf.gen.yaml must use local pins for C3 qualification (found remote: plugin)"
  fi
  if ! grep -Eq '^[[:space:]]*-[[:space:]]*local:' "$work/buf.gen.yaml"; then
    fail "buf.gen.yaml must declare local: plugins for C3 qualification"
  fi

  (
    cd "$work"
    # Plugin/version failure must be STOP — do not fall back to ambient or remote.
    if ! env PATH="$PLUGIN_BIN_DIR:$PATH" buf generate; then
      fail "buf generate failed (local plugin/config). C3 cannot PROCEED."
    fi
  )

  [[ -d "$work/gen/go" ]] || fail "buf generate produced no gen/go output"
  cp -a "$work/gen/go/." "$dest/"
  normalize_gen_go "$dest"
  assert_generated_plugin_headers "$dest"
  rm -rf -- "$work"
  trap - RETURN
}

generate_ts_once() {
  local dest="$1"
  rm -rf "$dest"
  mkdir -p "$dest"
  npx --yes "openapi-typescript@${EXPECTED_OPENAPI_TS}" "$OPENAPI" -o "$dest/schema.d.ts"
  cat >"$dest/package.json" <<'JSON'
{
  "name": "curriculum-studio-spike-ts",
  "private": true,
  "type": "module",
  "types": "schema.d.ts"
}
JSON
  cat >"$dest/smoke.ts" <<'TS'
import type { components } from "./schema.d.ts";

type ErrorCode = components["schemas"]["ErrorCode"];
type MatStatus = components["schemas"]["MaterializationStatus"];
type EventData = components["schemas"]["DomainEvent"]["data"];
type PageMeta = components["schemas"]["PageMeta"];
type AuthoringWindow = components["schemas"]["AuthoringWindow"];

const _code: ErrorCode = "failed_precondition";
const _st: MatStatus = "ready";
const _data: EventData = { nested: { ok: true }, n: 1 };
const _page: PageMeta = { limit: 25, offset: 0, totalCount: 0 };
const _window: AuthoringWindow = { availableMinutes: 30, start: "2026-08-01T00:00:00Z" };

export type SpikeTypes = {
  code: typeof _code;
  status: typeof _st;
  data: typeof _data;
  page: typeof _page;
  window: typeof _window;
};
TS
  cat >"$dest/tsconfig.json" <<'JSON'
{
  "compilerOptions": {
    "strict": true,
    "noEmit": true,
    "module": "nodenext",
    "moduleResolution": "nodenext",
    "target": "es2022",
    "skipLibCheck": true
  },
  "files": ["smoke.ts"]
}
JSON
}

assert_buf_gen_pins
BUF_VER="$(assert_buf_cli_pin)"
bootstrap_plugins
GENERATION_PATH="buf generate (isolated temp copy of contracts/; local plugins from private bin: protoc-gen-go@${EXPECTED_PGO_VER}, protoc-gen-go-grpc@${EXPECTED_PGGRPC_VER}; bin=${PLUGIN_BIN_DIR_EVIDENCE})"

echo "== C3 generate pass 1 (pinned local buf path) =="
echo "generation_path=$GENERATION_PATH"
echo "buf_cli=$BUF_VER"
echo "plugin_bin_dir=$PLUGIN_BIN_DIR"
echo "plugin_bin_dir_evidence=$PLUGIN_BIN_DIR_EVIDENCE"
echo "plugin_protoc_gen_go_path=$PLUGIN_PGO_PATH"
echo "plugin_protoc_gen_go_grpc_path=$PLUGIN_PGGRPC_PATH"
generate_proto_once "$PROTO_OUT/run1"
generate_ts_once "$TS_OUT/run1"
D1_PROTO="$(digest_tree "$PROTO_OUT/run1")"
D1_TS="$(digest_tree "$TS_OUT/run1")"
echo "proto_digest_1=$D1_PROTO"
echo "ts_digest_1=$D1_TS"

echo "== C3 generate pass 2 (pinned local buf path) =="
generate_proto_once "$PROTO_OUT/run2"
generate_ts_once "$TS_OUT/run2"
D2_PROTO="$(digest_tree "$PROTO_OUT/run2")"
D2_TS="$(digest_tree "$TS_OUT/run2")"
echo "proto_digest_2=$D2_PROTO"
echo "ts_digest_2=$D2_TS"

if [[ "$D1_PROTO" != "$D2_PROTO" ]]; then
  fail "proto generation not deterministic: $D1_PROTO vs $D2_PROTO"
fi
if [[ "$D1_TS" != "$D2_TS" ]]; then
  fail "TS generation not deterministic: $D1_TS vs $D2_TS"
fi

rm -rf "$PROTO_OUT/current"
cp -a "$PROTO_OUT/run2" "$PROTO_OUT/current"
rm -rf "$TS_OUT/current"
cp -a "$TS_OUT/run2" "$TS_OUT/current"

# No wall-clock fields: ordinary contracts-spikes must leave tracked evidence
# byte-stable across repeated runs (CI verifies twice with zero git diff).
# Paths in evidence are studio-relative (not absolute host roots).
{
  echo "generation_path=buf generate"
  echo "generation_path_detail=${GENERATION_PATH}"
  echo "buf_cli=${BUF_VER}"
  echo "buf_gen_yaml=curriculum-studio/contracts/buf.gen.yaml"
  echo "plugin_mode=local"
  echo "plugin_bin_dir=${PLUGIN_BIN_DIR_EVIDENCE}"
  echo "plugin_protoc_gen_go_path=${PLUGIN_PGO_PATH_EVIDENCE}"
  echo "plugin_protoc_gen_go_grpc_path=${PLUGIN_PGGRPC_PATH_EVIDENCE}"
  echo "plugin_protoc_gen_go=local:protoc-gen-go@${EXPECTED_PGO_VER}"
  echo "plugin_protoc_gen_go_grpc=local:protoc-gen-go-grpc@${EXPECTED_PGGRPC_VER}"
  echo "plugin_protoc_gen_go_module=${PLUGIN_PGO_MOD}"
  echo "plugin_protoc_gen_go_grpc_module=${PLUGIN_PGGRPC_MOD}"
  echo "plugin_protoc_gen_go_reported=${PLUGIN_PGO_REPORTED}"
  echo "plugin_protoc_gen_go_grpc_reported=${PLUGIN_PGGRPC_REPORTED}"
  echo "protoc_gen_go=protoc-gen-go ${EXPECTED_PGO_VER}"
  echo "protoc_gen_go_grpc=protoc-gen-go-grpc ${EXPECTED_PGGRPC_VER}"
  echo "proto_digest=$D1_PROTO"
  echo "ts_digest=$D1_TS"
  echo "openapi_typescript=${EXPECTED_OPENAPI_TS}"
} | tee "$EVIDENCE/generate_digests.txt"

echo "== compile generated Go (temp module spike.local/gen) =="
GEN_MOD="$OUT_ROOT/gen-mod"
rm -rf "$GEN_MOD"
mkdir -p "$GEN_MOD"
cp -a "$PROTO_OUT/current/." "$GEN_MOD/"
# package name stays curriculumstudiov1; module path is spike-local (not /v1 suffix)
cat >"$GEN_MOD/go.mod" <<'EOF'
module spike.local/gen

go 1.25.0

require (
        google.golang.org/grpc v1.72.2
        google.golang.org/protobuf v1.36.6
)
EOF
# Ensure package clause is curriculumstudiov1 (protoc default from go_package)
(
  cd "$GEN_MOD"
  GOWORK=off go mod tidy
  GOWORK=off go build .
)

echo "OK: generated Go compiles"

echo "== strict TS check =="
set +e
(
  cd "$TS_OUT/current"
  npx --yes -p "typescript@${EXPECTED_TSC}" tsc -p tsconfig.json
)
tsc_ec=$?
set -e
if [[ "$tsc_ec" -eq 0 ]]; then
  echo "OK: TS strict compile"
  echo "ts_strict=ok" >>"$EVIDENCE/generate_digests.txt"
else
  echo "FAIL: tsc strict failed (exit $tsc_ec)" >&2
  echo "ts_strict=fail" >>"$EVIDENCE/generate_digests.txt"
  exit 1
fi

echo "OK: double-generate deterministic via pinned local buf generate"
