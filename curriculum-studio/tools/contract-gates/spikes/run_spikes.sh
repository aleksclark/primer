#!/usr/bin/env bash
# C3 qualification spike runner: generate twice → compile → shape tests → REPORT.
set -euo pipefail

SPIKES="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
export SPIKE_OUT="${SPIKE_OUT:-$SPIKES/.tmp}"
EVIDENCE="$SPIKES/evidence"
REPORT="$SPIKES/REPORT.md"
mkdir -p "$EVIDENCE"

fail() { echo "FAIL: $*" >&2; exit 1; }

echo "==== C3 generate_twice (pinned local buf generate) ===="
bash "$SPIKES/generate_twice.sh"

GEN_MOD="$SPIKE_OUT/gen-mod"
[[ -f "$GEN_MOD/go.mod" ]] || fail "missing gen module $GEN_MOD"

echo "==== C3 Go shape harness (nested module spike.local/harness) ===="
# Source tree is a nested module (go/harness/go.mod) so production
# curriculum-studio never resolves spike.local/gen. Runtime still copies into
# SPIKE_OUT and pins replace → gen-mod for a real local compile/test.
HARNESS_MOD="$SPIKE_OUT/harness-mod"
rm -rf "$HARNESS_MOD"
mkdir -p "$HARNESS_MOD"
cp -a "$SPIKES/go/harness/." "$HARNESS_MOD/"
# Drop any leftover go.sum from a prior manual tidy of the source tree.
rm -f "$HARNESS_MOD/go.sum"

cat >"$HARNESS_MOD/go.mod" <<EOF
module spike.local/harness

go 1.25.0

require (
        spike.local/gen v0.0.0
        google.golang.org/grpc v1.72.2
        google.golang.org/protobuf v1.36.6
)

replace spike.local/gen => $GEN_MOD
EOF

(
  cd "$HARNESS_MOD"
  GOWORK=off go mod tidy
  # Strip go test wall-clock durations so committed go_shapes.txt is byte-stable.
  GOWORK=off go test -tags=spike -count=1 -v . 2>&1 \
    | sed -E \
      -e 's/ \([0-9]+(\.[0-9]+)?s\)/ (0.00s)/g' \
      -e 's/^(ok[[:space:]]+[^[:space:]]+)[[:space:]]+[0-9]+(\.[0-9]+)?s$/\1\t0.000s/' \
    | tee "$EVIDENCE/go_shapes.txt"
)

echo "==== synthesize REPORT.md ===="
DIGESTS="$EVIDENCE/generate_digests.txt"
[[ -f "$DIGESTS" ]] || fail "missing $DIGESTS"

require_digest_key() {
  local key="$1"
  if ! grep -Eq "^${key}=" "$DIGESTS"; then
    fail "generate_digests.txt missing required key: $key"
  fi
  awk -F= -v k="$key" '$1==k{print substr($0,index($0,"=")+1); exit}' "$DIGESTS"
}

GEN_PATH="$(require_digest_key generation_path)"
GEN_DETAIL="$(require_digest_key generation_path_detail)"
BUF_CLI="$(require_digest_key buf_cli)"
BUF_GEN_YAML="$(require_digest_key buf_gen_yaml)"
PLUGIN_MODE="$(require_digest_key plugin_mode)"
PLUGIN_BIN_DIR="$(require_digest_key plugin_bin_dir)"
PLUGIN_PGO_PATH="$(require_digest_key plugin_protoc_gen_go_path)"
PLUGIN_PGGRPC_PATH="$(require_digest_key plugin_protoc_gen_go_grpc_path)"
PLUGIN_PGO="$(require_digest_key plugin_protoc_gen_go)"
PLUGIN_PGGRPC="$(require_digest_key plugin_protoc_gen_go_grpc)"
PLUGIN_PGO_MOD="$(require_digest_key plugin_protoc_gen_go_module)"
PLUGIN_PGGRPC_MOD="$(require_digest_key plugin_protoc_gen_go_grpc_module)"
PROTO_D="$(require_digest_key proto_digest)"
TS_D="$(require_digest_key ts_digest)"
TS_STRICT="$(require_digest_key ts_strict)"
PGO="$(require_digest_key protoc_gen_go)"
PGGRPC="$(require_digest_key protoc_gen_go_grpc)"
OTS="$(require_digest_key openapi_typescript)"

# Fail closed: PROCEED only when the qualified path is the pinned local buf generate chain.
if [[ "$GEN_PATH" != "buf generate" ]]; then
  fail "C3 generation_path must be 'buf generate' (got '$GEN_PATH'); refusing PROCEED"
fi
if [[ "$PLUGIN_MODE" != "local" ]]; then
  fail "C3 plugin_mode must be 'local' (got '$PLUGIN_MODE'); refusing remote/BSR path"
fi
if [[ "$PGO" != "protoc-gen-go v1.36.11" ]]; then
  fail "C3 protoc-gen-go pin mismatch: '$PGO' (want protoc-gen-go v1.36.11)"
fi
if [[ "$PGGRPC" != "protoc-gen-go-grpc v1.5.1" ]]; then
  fail "C3 protoc-gen-go-grpc pin mismatch: '$PGGRPC' (want protoc-gen-go-grpc v1.5.1)"
fi
if [[ "$PLUGIN_PGO" != "local:protoc-gen-go@v1.36.11" ]]; then
  fail "C3 local plugin pin mismatch: '$PLUGIN_PGO'"
fi
if [[ "$PLUGIN_PGGRPC" != "local:protoc-gen-go-grpc@v1.5.1" ]]; then
  fail "C3 local plugin pin mismatch: '$PLUGIN_PGGRPC'"
fi
if [[ ! "$PLUGIN_PGO_MOD" =~ protoc-gen-go@v1\.36\.11$ ]]; then
  fail "C3 protoc-gen-go module pin mismatch: '$PLUGIN_PGO_MOD'"
fi
if [[ ! "$PLUGIN_PGGRPC_MOD" =~ protoc-gen-go-grpc@v1\.5\.1$ ]]; then
  fail "C3 protoc-gen-go-grpc module pin mismatch: '$PLUGIN_PGGRPC_MOD'"
fi
if [[ ! -x "$PLUGIN_PGO_PATH" ]]; then
  # Evidence may store studio-relative path; resolve against repo if needed.
  if [[ -x "$SPIKES/../../../$PLUGIN_PGO_PATH" ]]; then
    PLUGIN_PGO_PATH_ABS="$(cd "$(dirname "$SPIKES/../../../$PLUGIN_PGO_PATH")" && pwd)/$(basename "$PLUGIN_PGO_PATH")"
  elif [[ "$PLUGIN_PGO_PATH" == curriculum-studio/* ]]; then
    REPO_ROOT="$(cd "$SPIKES/../../../.." && pwd)"
    PLUGIN_PGO_PATH_ABS="$REPO_ROOT/$PLUGIN_PGO_PATH"
  else
    PLUGIN_PGO_PATH_ABS="$PLUGIN_PGO_PATH"
  fi
else
  PLUGIN_PGO_PATH_ABS="$PLUGIN_PGO_PATH"
fi
if [[ ! -x "$PLUGIN_PGGRPC_PATH" ]]; then
  if [[ "$PLUGIN_PGGRPC_PATH" == curriculum-studio/* ]]; then
    REPO_ROOT="$(cd "$SPIKES/../../../.." && pwd)"
    PLUGIN_PGGRPC_PATH_ABS="$REPO_ROOT/$PLUGIN_PGGRPC_PATH"
  else
    PLUGIN_PGGRPC_PATH_ABS="$PLUGIN_PGGRPC_PATH"
  fi
else
  PLUGIN_PGGRPC_PATH_ABS="$PLUGIN_PGGRPC_PATH"
fi
if [[ ! -x "$PLUGIN_PGO_PATH_ABS" ]]; then
  fail "C3 private protoc-gen-go path missing/not executable: $PLUGIN_PGO_PATH (resolved $PLUGIN_PGO_PATH_ABS)"
fi
if [[ ! -x "$PLUGIN_PGGRPC_PATH_ABS" ]]; then
  fail "C3 private protoc-gen-go-grpc path missing/not executable: $PLUGIN_PGGRPC_PATH (resolved $PLUGIN_PGGRPC_PATH_ABS)"
fi
if [[ ! "$BUF_CLI" =~ ^1\.72\.[0-9]+$ ]]; then
  fail "C3 buf CLI pin mismatch: '$BUF_CLI' (want 1.72.x)"
fi
if [[ "$GEN_DETAIL" == *buf.build/* ]]; then
  fail "C3 generation_path_detail still references BSR plugins: $GEN_DETAIL"
fi
# Require local-plugin marker in detail string.
case "$GEN_DETAIL" in
  *"local plugins"*) ;;
  *) fail "C3 generation_path_detail must describe local plugins: $GEN_DETAIL" ;;
esac
case "$PLUGIN_BIN_DIR" in
  curriculum-studio/*) ;;
  /*)
    fail "C3 plugin_bin_dir in evidence must be studio-relative (got '$PLUGIN_BIN_DIR')"
    ;;
esac

go_ok=1
if grep -E '^FAIL' "$EVIDENCE/go_shapes.txt" >/dev/null 2>&1; then
  go_ok=0
fi
if ! grep -q 'PASS' "$EVIDENCE/go_shapes.txt"; then
  go_ok=0
fi

verdict_shape() {
  if [[ "$go_ok" -eq 1 ]]; then
    echo "PROCEED"
  else
    echo "STOP"
  fi
}

OVERALL="PROCEED"
if [[ "$go_ok" -ne 1 ]]; then
  OVERALL="STOP"
fi

if [[ "$TS_STRICT" == "ok" ]]; then
  TS_NOTE="PROCEED (strict tsc ok)"
elif [[ "$TS_STRICT" == "skipped" ]]; then
  TS_NOTE="CONDITIONAL PROCEED (tsc skipped; schema.d.ts generated)"
  OVERALL="STOP"
else
  TS_NOTE="STOP"
  OVERALL="STOP"
fi

S1=$(verdict_shape); S2=$(verdict_shape); S3=$(verdict_shape)
S4=$(verdict_shape); S5=$(verdict_shape); S6=$(verdict_shape)

cat >"$REPORT" <<EOF
# Curriculum Studio contracts — C3 qualification REPORT

**Overall gate for C4/C6:** \`${OVERALL}\`

Evidence is regenerated byte-stably by ordinary \`make contracts-spikes\` (no wall-clock stamp).

## Generation path (authoritative)

What actually ran for the digests below:

- **Command:** \`buf generate\`
- **Working set:** isolated temp copy of \`curriculum-studio/contracts/{buf.yaml,buf.gen.yaml,proto}\` (repo tree not mutated)
- **Config:** \`${BUF_GEN_YAML}\` (local plugins only — no BSR remote plugins)
- **Detail:** ${GEN_DETAIL}
- **Private plugin bin:** \`${PLUGIN_BIN_DIR}\`
- **protoc-gen-go path:** \`${PLUGIN_PGO_PATH}\`
- **protoc-gen-go-grpc path:** \`${PLUGIN_PGGRPC_PATH}\`
- **Host ambient \`protoc-gen-go\`:** **not used** (private bin prepended; exact pin verified)
- **BSR remote plugins:** **not used** (avoids \`resource_exhausted\` rate limits)

If local plugin bootstrap, version checks, or \`buf generate\` fail, this gate is **STOP** (no ambient/remote fallback).

## Pins

| Tool | Version / pin |
| --- | --- |
| Buf CLI (actual) | ${BUF_CLI} |
| plugin mode | ${PLUGIN_MODE} |
| buf.gen.yaml local | ${PLUGIN_PGO} |
| buf.gen.yaml local | ${PLUGIN_PGGRPC} |
| go install module | ${PLUGIN_PGO_MOD} |
| go install module | ${PLUGIN_PGGRPC_MOD} |
| protoc-gen-go (from generated headers) | ${PGO} |
| protoc-gen-go-grpc (from generated headers) | ${PGGRPC} |
| openapi-typescript | ${OTS} |
| TypeScript (tsc) | 5.9.2 via npx |
| google.golang.org/grpc | v1.72.2 (spike module) |
| google.golang.org/protobuf | v1.36.6 (spike module) |

## Determinism (E3-07)

| Artifact | Digest (sha256 of path+content tree) |
| --- | --- |
| proto Go stubs (double \`buf generate\`) | \`${PROTO_D}\` |
| openapi-typescript schema | \`${TS_D}\` |

Double-generate compared run1 vs run2: **equal**.

Evidence: \`evidence/generate_digests.txt\`

## Shape results

| Shape | Stack | Result | Evidence | Mitigations |
| --- | --- | --- | --- | --- |
| nullable / optional / absent (P3-S1, E3-01) | protoc-gen-go protojson + REST JSON omitempty | ${S1} | evidence/nullable_optional.txt | OpenAPI \`AuthoringWindow.end\` is optional (absent), not typed null; proto3 timestamp presence preserved |
| timestamp / duration minutes (P3-S2, E3-02) | protobuf binary + RFC3339 | ${S2} | evidence/timestamp_duration.txt | Invalid timestamps fail parse; minutes stay int32 |
| typed errors (P3-S3, E3-03) | gRPC status details ErrorDetail + problem+json | ${S3} | evidence/typed_errors.txt | Unknown ErrorCode maps to \`internal\` (fail closed) |
| pagination (P3-S4, E3-04) | gRPC PageRequest/Response + REST limit/offset | ${S4} | evidence/pagination.txt | Transports remain non-unified (no shared domain Page type) |
| long-running materialization (P3-S5, E3-05) | generated gRPC client → bufconn server | ${S5} | evidence/lro.txt | GetBundle fails FailedPrecondition+ErrorDetail until ready; idempotent Materialize |
| Struct tutor_context / event data (P3-S6, E3-06) | google.protobuf.Struct binary + protojson + REST object | ${S6} | evidence/struct.txt | Nested JSON preserved; no silent key drop on fixture |
| TS authoring client types | openapi-typescript + tsc --strict | ${TS_NOTE} | spikes/.tmp/ts-gen/current/ | Smoke types cover ErrorCode, PageMeta, DomainEvent.data, AuthoringWindow |

## Planted assertion teeth

\`TestSpikePlantedAssertionTeeth\` proves dropped Struct keys fail deep equality.

## Go test log

See \`evidence/go_shapes.txt\`.

## Gate interpretation (P3-S7)

- Any **STOP** on a required shape blocks C4/C6 generator adoption for that stack.
- Spike fixtures retained as conformance seeds for later phases.
- No business materializer was implemented; harnesses only.
- Generated stubs remain under gitignored \`spikes/.tmp/\` only.
- C3 digests and PROCEED are valid **only** for the pinned local \`buf generate\` path above.
- Ordinary second run must not call BSR remote plugins (reproducibility / no rate-limit STOP).

## Overall: ${OVERALL}
EOF

echo "REPORT written to $REPORT"
echo "OVERALL=$OVERALL"
echo "generation_path=$GEN_PATH"
echo "buf_cli=$BUF_CLI"
echo "plugin_mode=$PLUGIN_MODE"
echo "plugins=$PLUGIN_PGO $PLUGIN_PGGRPC"
echo "plugin_bin_dir=$PLUGIN_BIN_DIR"

if [[ "$OVERALL" != "PROCEED" ]]; then
  echo "C3 overall STOP — see REPORT.md" >&2
  exit 2
fi

echo "OK: C3 spikes PROCEED (pinned local buf generate)"
