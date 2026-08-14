# F0 coverage probe non-destructive + CI path-filter fix — evidence

Second F0 remediation on tip `8a365945743f0d63864c6855ede8a0ea0a59ed59`
(prior false-green cover fix retained: Studio ≥85, Identity ≥80).

## Findings (fresh spec re-review)

1. **CRITICAL C1** — `scripts/probe-module-cover-gates.sh` unconditionally
   `rm -rf`’d live `curriculum-studio/internal` and `primer-identity/internal`.
   Invoked by `foundation-check` and CI, so it would delete real S1/I1 code or
   untracked work once those trees exist.
2. **IMPORTANT I1** — `.github/workflows/curriculum-studio-foundations.yml`
   path filters omitted `scripts/enforce-module-cover.sh` and
   `scripts/probe-module-cover-gates.sh`, so load-bearing gate edits could skip CI.

## RED (pre-fix class)

Destructive probe shape (prior tip):

```text
cleanup_module() {
  local mod="$1"
  rm -rf "${mod}/internal"   # LIVE module root
  rm -f "${mod}/coverage.out"
}
write_pkg curriculum-studio high   # mutates live tree
…
trap cleanup_all EXIT              # wipes live internal/ on every run
```

CI path filter gap (prior tip):

```yaml
paths:
  - scripts/check-f0-foundations.sh
  # missing: scripts/enforce-module-cover.sh
  # missing: scripts/probe-module-cover-gates.sh
```

## Fix

- Rewrite `scripts/probe-module-cover-gates.sh` to create/delete **only** an
  isolated `mktemp -d` fixture root. Tiny standalone Go modules are generated
  there; `enforce-module-cover.sh` is called with absolute temp module paths for
  deferred / empty / low / high / missing-bc matrices.
- No create/overwrite/move/backup/delete of live `curriculum-studio/internal` or
  `primer-identity/internal`. No `rm -rf` on any path derived from live module roots.
- Before/after sha256 snapshot of live module trees must be byte-identical.
- `scripts/check-f0-foundations.sh`: static wiring/floors + isolated probe only;
  live deferred make checks only when `internal/` is absent; never mutates live code.
- Workflow push + pull_request path filters add both load-bearing scripts.

## GREEN (post-fix)

### Isolated probe matrix

```text
./scripts/probe-module-cover-gates.sh
PASS: static non-destruction guards (no live rm -rf / write / mkdir / legacy helpers)
PASS: helper deferred exit 2 (studio + identity)
PASS: helper empty packages exit 1 + "no packages" (studio + identity)
PASS: helper low coverage exit 1 below 85%/80% (studio + identity)
PASS: helper high coverage exit 0 OK (studio + identity)
PASS: helper missing-bc exit 1 + "bc is required"
PASS: live trees byte-identical to pre-probe snapshot
module-cover gate probe OK (isolated fixtures only; live internals untouched)
```

### Sentinel non-destruction proof

Planted under live trees, ran probe, compared hashes, restored clean:

| Path | Result |
|------|--------|
| `curriculum-studio/internal/.f0-cover-probe-sentinel` | byte-identical |
| `curriculum-studio/internal/realsentinel.go` | byte-identical |
| `primer-identity/internal/.f0-cover-probe-sentinel` | byte-identical |
| `primer-identity/internal/realsentinel.go` | byte-identical |

```text
BEFORE/AFTER sha256 identical for all four sentinels
PASS: live sentinels byte-identical after probe
Restored clean (no tracked/untracked probe debris)
```

### CI path filters

```yaml
# push + pull_request both include:
- scripts/check-f0-foundations.sh
- scripts/enforce-module-cover.sh
- scripts/probe-module-cover-gates.sh
```

### Floors retained

| Target | Floor |
|--------|-------|
| `studio-cover` | 85 (`STUDIO_COVER_MIN`) |
| `identity-cover` | 80 (`IDENTITY_COVER_MIN`) |
| root `COVER_MIN` | 85 (unchanged) |

No blockers. Amended into F0 commit preserving message
`build: establish Curriculum Studio module foundations`.
