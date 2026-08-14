#!/usr/bin/env python3
"""Generate or verify curriculum-studio/db/baseline_manifest.json.

Preferred path is the Go migrator:

  go run ./cmd/migrate -check-freeze
  go run ./cmd/migrate -write-freeze

This script remains a thin checksum helper for environments without a Go
toolchain on the path of the Python schema suite.

Write mode is pre-live only: refused when STUDIO_MIGRATIONS_LIVE is truthy or
when db/STUDIO_MIGRATIONS_LIVE marker exists (any path shape). Check mode remains
available. Live classification matches Go studiodb.ClassifyLive / Truthy /
LiveMarkerExists — do not fork a third parser.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
MIGRATIONS = ROOT / "migrations"
MANIFEST = ROOT / "baseline_manifest.json"
LIVE_MARKER = ROOT / "STUDIO_MIGRATIONS_LIVE"
BASELINE = [
    "00001_identity_and_catalogs.sql",
    "00002_plan_domain.sql",
    "00003_materialization_and_integration.sql",
    "00004_invariants.sql",
]

# Match Go studiodb.Truthy: TrimSpace + ToLower ∈ {1, true, yes, on}.
_TRUTHY = frozenset({"1", "true", "yes", "on"})


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


def _truthy_env_value(value: str) -> bool:
    """Shared truthy parser (Go studiodb.Truthy parity)."""
    return value.strip().lower() in _TRUTHY


def _truthy_env(name: str) -> bool:
    return _truthy_env_value(os.environ.get(name, ""))


def live_marker_exists(path: Path | None = None) -> bool:
    """Match Go LiveMarkerExists / os.Stat: any existing path is live.

    Regular file, directory, symlink-to-file, and symlink-to-dir all classify
    live. Broken symlinks (lexists but not exists after resolve) are not live.
    """
    p = LIVE_MARKER if path is None else path
    try:
        # Path.exists() follows symlinks like os.Stat; False for broken links.
        return p.exists()
    except OSError:
        return False


def is_live_env() -> bool:
    """Match Go studiodb.ClassifyLive: env flag or marker under db root."""
    if _truthy_env("STUDIO_MIGRATIONS_LIVE"):
        return True
    return live_marker_exists()


def guard_write_freeze() -> None:
    if not is_live_env():
        return
    raise SystemExit(
        "refusing write-freeze: migrations are live-classified "
        "(STUDIO_MIGRATIONS_LIVE env or STUDIO_MIGRATIONS_LIVE marker); "
        "baseline rewrite is pre-live only — use --check and add 00005+ instead"
    )


def main() -> int:
    p = argparse.ArgumentParser()
    p.add_argument("--write", action="store_true", help="write baseline_manifest.json (pre-live only)")
    p.add_argument("--check", action="store_true", help="verify manifest matches files")
    args = p.parse_args()
    if not args.write and not args.check:
        args.check = True

    if args.write:
        guard_write_freeze()
        built = build()
        MANIFEST.write_text(json.dumps(built, indent=2) + "\n", encoding="utf-8")
        print(f"wrote {MANIFEST}")
        return 0

    built = build()
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
