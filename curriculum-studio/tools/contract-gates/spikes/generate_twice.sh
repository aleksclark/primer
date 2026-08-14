#!/usr/bin/env bash
# Deterministic double-generate for C3 qualification (E3-07).
# Outputs only under build-only / gitignored paths.
set -euo pipefail

SPIKES_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
STUDIO_ROOT="$(cd "$SPIKES_ROOT/../../.." && pwd)"
CONTRACTS="$STUDIO_ROOT/contracts"
OUT_ROOT="${SPIKE_OUT:-$SPIKES_ROOT/.tmp}"
EVIDENCE="$SPIKES_ROOT/evidence"
PROTO_OUT="$OUT_ROOT/proto-gen"
TS_OUT="$OUT_ROOT/ts-gen"
OPENAPI="$CONTRACTS/openapi/v1/curriculum-studio.yaml"

rm -rf "$OUT_ROOT"
mkdir -p "$PROTO_OUT" "$TS_OUT" "$EVIDENCE"

fail() { echo "FAIL: $*" >&2; exit 1; }

command -v protoc >/dev/null || fail "protoc required"
command -v protoc-gen-go >/dev/null || fail "protoc-gen-go required"
command -v protoc-gen-go-grpc >/dev/null || fail "protoc-gen-go-grpc required"
command -v python3 >/dev/null || fail "python3 required"
command -v npx >/dev/null || fail "npx required"
command -v go >/dev/null || fail "go required"

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

generate_proto_once() {
  local dest="$1"
  rm -rf "$dest"
  mkdir -p "$dest"
  protoc \
    -I "$CONTRACTS/proto" \
    -I /usr/include \
    --go_out="$dest" --go_opt=paths=source_relative \
    --go-grpc_out="$dest" --go-grpc_opt=paths=source_relative \
    "$CONTRACTS/proto/curriculumstudio/v1/common.proto" \
    "$CONTRACTS/proto/curriculumstudio/v1/plan.proto" \
    "$CONTRACTS/proto/curriculumstudio/v1/catalog.proto" \
    "$CONTRACTS/proto/curriculumstudio/v1/materialization.proto" \
    "$CONTRACTS/proto/curriculumstudio/v1/events.proto" \
    "$CONTRACTS/proto/curriculumstudio/v1/integration.proto"
  normalize_gen_go "$dest"
}

generate_ts_once() {
  local dest="$1"
  rm -rf "$dest"
  mkdir -p "$dest"
  npx --yes openapi-typescript@7.13.0 "$OPENAPI" -o "$dest/schema.d.ts"
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

echo "== C3 generate pass 1 =="
generate_proto_once "$PROTO_OUT/run1"
generate_ts_once "$TS_OUT/run1"
D1_PROTO="$(digest_tree "$PROTO_OUT/run1")"
D1_TS="$(digest_tree "$TS_OUT/run1")"
echo "proto_digest_1=$D1_PROTO"
echo "ts_digest_1=$D1_TS"

echo "== C3 generate pass 2 =="
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

# Collapse internal whitespace and strip trailing spaces so evidence never
# fails `git diff --check` when tool --version prints a trailing newline.
tool_version() {
  "$@" 2>&1 | tr -s '[:space:]' ' ' | sed 's/[[:space:]]*$//'
}

# No wall-clock fields: ordinary contracts-spikes must leave tracked evidence
# byte-stable across repeated runs (CI verifies twice with zero git diff).
{
  echo "proto_digest=$D1_PROTO"
  echo "ts_digest=$D1_TS"
  echo "protoc_gen_go=$(tool_version protoc-gen-go --version)"
  echo "protoc_gen_go_grpc=$(tool_version protoc-gen-go-grpc --version)"
  echo "openapi_typescript=7.13.0"
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
  npx --yes -p typescript@5.9.2 tsc -p tsconfig.json
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

echo "OK: double-generate deterministic"
