# tools/contract-gates

Deterministic offline gates for Curriculum Studio contracts.

| Tool | Wave | Purpose |
| --- | --- | --- |
| `ownership_scan.py` | C1 | Forbidden OpenAPI integration schemas; layout checks |
| `check_no_tracked_generated.sh` | C1 / C10 | Fail if generated paths are tracked |
| `layout_test.go` (module) | C1 | Reserved paths + import direction docs |
| `enum_parity.py` | C2 | Proto / OpenAPI / DB wire-string parity |
| `enum_mappings.yaml` | C2 | Mapping rules only (not value catalogs) |
| `spikes/` | C3 | Hard-type qualification fixtures + REPORT |
| `requirements.txt` | C1→C11 | REQ-* registry seed |

Run via `curriculum-studio/Makefile` targets (`contracts-validate`,
`contracts-parity`, `contracts-gates`, …) or directly.

**Root Makefile is F0-owned** — do not add root targets from this tree.
