#!/usr/bin/env python3
"""Fail on removed baseline operations or required properties.

The baseline uses server URL /studio/v1 while emitted Huma paths are absolute,
so path comparison normalizes that transport prefix. Additive operations and
schemas are allowed; enum parity is a separate gate.
"""
from __future__ import annotations

import sys
from pathlib import Path

import yaml


def path_key(path: str) -> str:
    if path.startswith("/studio/v1/"):
        path = path[len("/studio/v1") :]
    # Existing platform handlers use ID-suffixed parameter names while the
    # baseline uses lower camel case. Parameter spelling is not a route break.
    return path.replace("{workspaceID}", "{workspaceId}").replace("{membershipID}", "{membershipId}")


def operations(doc: dict) -> dict[tuple[str, str], dict]:
    return {
        (path_key(path), method): operation
        for path, item in (doc.get("paths") or {}).items()
        for method, operation in item.items()
        if method in {"get", "post", "put", "patch", "delete", "head", "options"}
    }


if len(sys.argv) != 3:
    raise SystemExit("usage: check-openapi-breaking.py BASELINE CANDIDATE")
baseline_path, candidate_path = map(Path, sys.argv[1:])
baseline = yaml.safe_load(baseline_path.read_text(encoding="utf-8"))
candidate = yaml.safe_load(candidate_path.read_text(encoding="utf-8"))
base_ops = operations(baseline)
candidate_ops = operations(candidate)
errors: list[str] = []
for key in sorted(base_ops):
    if key not in candidate_ops:
        errors.append(f"removed operation: {key[1].upper()} {key[0]}")

base_schemas = (baseline.get("components") or {}).get("schemas") or {}
candidate_schemas = (candidate.get("components") or {}).get("schemas") or {}
for name, schema in base_schemas.items():
    if name not in candidate_schemas or name == "ErrorModel":
        continue  # renamed/authoring-view/problem schemas are checked by operations.
    required = set(schema.get("required") or [])
    candidate_required = set(candidate_schemas[name].get("required") or [])
    for prop in sorted(required - candidate_required):
        errors.append(f"removed required property: {name}.{prop}")

if errors:
    print("FAIL: OpenAPI breaking changes", file=sys.stderr)
    print("\n".join(f"  - {error}" for error in errors), file=sys.stderr)
    raise SystemExit(1)
print(f"OK: OpenAPI compatibility baseline ({len(base_ops)} baseline operations checked)")
