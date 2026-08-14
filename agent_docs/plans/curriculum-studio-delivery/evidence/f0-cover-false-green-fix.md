# F0 coverage false-green review fix — evidence

Independent quality review findings on tip `59912d55c48f191635bd8675c7eb9c461b44ce98`
before this remediation.

## Findings

1. **CRITICAL** — `studio-cover` used repeated `cd curriculum-studio && …` inside
   one continued shell recipe. After the first `cd`, later `cd`s failed; empty
   `total` / `bc` / test errors fell through to `else`, printing OK and exiting 0.
2. **IMPORTANT** — `identity-cover` had no numeric floor when
   `primer-identity/internal` appeared (print-only; future false-green risk).
   Required ≥80% while Studio remains ≥85%.

## RED (pre-fix recipes)

Command: `./scripts/probe-module-cover-gates.sh` against the broken Makefile
recipes (helper not yet wired; probe expected empty/low fail + high pass).

Observed on broken recipes (manual + early probe):

```text
# empty internal/
make studio-cover → exit 0 with:
  go: warning: "./internal/..." matched no packages
  /bin/sh: line 3: cd: curriculum-studio: No such file or directory
  (standard_in) 1: syntax error
  OK: studio coverage % >= 85%

# low coverage package (~25%)
make studio-cover → exit 0 with same false OK path

# identity high coverage (~100%)
make identity-cover → exit 2 (second cd fails; no floor either way)

# deferred (no internal/) still exit 2 (correct F0 semantic)
```

Probe summary against broken recipes:

```text
PASS: deferred exit 2 (both)
FAIL: studio-cover rejects empty (want 1, got 0)   # false-green
FAIL: identity-cover rejects empty
FAIL: studio-cover fails below 85% (want 1, got 0) # false-green
FAIL: identity-cover fails below 80%
PASS: studio-cover high (exit 0)                   # coincidental; gate not evaluated
FAIL: identity-cover high (want 0, got 2)
module-cover gate probe FAILED with 5 error(s)
```

## Fix

- `scripts/enforce-module-cover.sh` — reusable fail-closed gate:
  one module cwd, non-empty package set, `set -e`, temp coverage profile +
  cleanup, parse validation, require `bc`, no success on missing/empty total;
  no `internal/` → deferred exit 2.
- Makefile: `studio-cover` / `identity-cover` call the helper with
  `STUDIO_COVER_MIN=85` and `IDENTITY_COVER_MIN=80`; root `COVER_MIN` stays 85.
- `scripts/probe-module-cover-gates.sh` — adversarial empty/low/high/deferred/
  missing-bc regression against **isolated mktemp fixtures only** (never mutates
  live `curriculum-studio/` or `primer-identity/` trees; see
  `f0-cover-probe-nondestructive-fix.md` for the C1 non-destruction follow-up).
- `scripts/check-f0-foundations.sh` — verifies floors, helper wiring, no chained
  multi-`cd` recipes, deferred exit 2 when `internal/` absent, and runs the
  isolated probe.

## GREEN (post-fix)

```text
./scripts/probe-module-cover-gates.sh
… all PASS …
module-cover gate probe OK

make foundation-check → F0 foundation check OK
make studio-test → ok
make identity-test → ok
go work sync → go.work / go.work.sum stable
git diff --check → clean
```

Adversarial matrix (helper direct / make):

| Case | Studio helper | Studio make | Identity helper | Identity make |
|------|---------------|-------------|-----------------|---------------|
| no `internal/` | 2 deferred | 2 | 2 deferred | 2 |
| empty `internal/` | 1 | 2 (non-OK) | 1 | 2 (non-OK) |
| low coverage | 1 below 85% | 2 | 1 below 80% | 2 |
| high coverage | 0 OK | 0 OK | 0 OK | 0 OK |

No blockers. Amended into F0 commit preserving message
`build: establish Curriculum Studio module foundations`.
