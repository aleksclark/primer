#!/usr/bin/env python3
"""Generate or verify curriculum-studio/db/baseline_manifest.json.

Preferred path is the Go migrator:

  go run ./cmd/migrate -check-freeze
  go run ./cmd/migrate -write-freeze

This script remains a thin checksum helper for environments without a Go
toolchain on the path of the Python schema suite.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
MIGRATIONS = ROOT / "migrations"
MANIFEST = ROOT / "baseline_manifest.json"
BASELINE = [
    "00001_identity_and_catalogs.sql",
    "00002_plan_domain.sql",
    "00003_materialization_and_integration.sql",
    "00004_invariants.sql",
]


def sha256_file(path: Path) -> str:
    h = hashlib.sha256()
    h.update(path.read_bytes())
    return h.hexdigest()


def build() -> dict:
    return {
        "version_table": "studio_goose_db_version",
        "schema": "curriculum_studio",
        "migrations": [
            {
                "file": name,
                "sha256": sha256_file(MIGRATIONS / name),
                "baseline_immutable_after_live": True,
            }
            for name in BASELINE
        ],
    }


def main() -> int:
    p = argparse.ArgumentParser()
    p.add_argument("--write", action="store_true", help="write baseline_manifest.json")
    p.add_argument("--check", action="store_true", help="verify manifest matches files")
    args = p.parse_args()
    if not args.write and not args.check:
        args.check = True

    built = build()
    if args.write:
        MANIFEST.write_text(json.dumps(built, indent=2) + "\n", encoding="utf-8")
        print(f"wrote {MANIFEST}")
        return 0

    if not MANIFEST.is_file():
        print(f"missing {MANIFEST}", file=sys.stderr)
        return 1
    committed = json.loads(MANIFEST.read_text(encoding="utf-8"))
    if committed != built:
        print("freeze inventory drift:", file=sys.stderr)
        print(" committed != current file hashes", file=sys.stderr)
        return 1
    print("freeze inventory ok")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
