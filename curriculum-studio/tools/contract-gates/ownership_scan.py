#!/usr/bin/env python3
"""C1 ownership scanner (E1-01, P1-S1).

Fails if OpenAPI components reintroduce Primer-only integration schemas.
Also verifies reserved package layout paths exist.
"""
from __future__ import annotations

import argparse
import sys
from pathlib import Path

try:
    import yaml
except ImportError as exc:  # pragma: no cover
    raise SystemExit("PyYAML is required") from exc

# Forbidden as OpenAPI component schema names (integration SoT is protobuf).
FORBIDDEN_SCHEMA_NAMES = frozenset(
    {
        "MaterializationContext",
        "MaterializationBundle",
        "SessionSpec",
        "MasterySnapshot",
        "ActiveProject",
        "MaterializationOverrides",
        # Full Primer bundle trees must not appear under alternate names.
        "PrimerMaterializationContext",
        "PrimerMaterializationBundle",
    }
)

# Thin authoring subset schemas that are intentionally present in OpenAPI.
ALLOWED_AUTHORING_SUBSET = frozenset(
    {
        "AuthoringBundle",
        "AuthoringMaterializeRequest",
        "AuthoringWindow",
        "AuthoringGenerationPolicy",
        "GenericLearnerProfile",
        "Materialization",
        "MaterializationStatus",
        "MaterializedItem",
        "MaterializedItemKind",
        "MaterializedItemStatus",
        "ItemLockState",
        "DomainEvent",
        "EventTypeName",
    }
)

REQUIRED_REL_PATHS = [
    "contracts",
    "contracts/OWNERS.md",
    "cmd/openapi-gen/README.md",
    "cmd/studio-api/README.md",
    "internal/api/README.md",
    "internal/grpcapi/README.md",
    "internal/boundary/README.md",
    "internal/authn/README.md",
    "clients/ts-rest/README.md",
    "clients/go-rest/README.md",
    "clients/go-grpc/README.md",
    "tools/contract-gates/README.md",
]


def studio_root_from_here() -> Path:
    # tools/contract-gates -> tools -> curriculum-studio
    return Path(__file__).resolve().parents[2]


def scan_openapi(openapi_path: Path) -> list[str]:
    doc = yaml.safe_load(openapi_path.read_text(encoding="utf-8"))
    schemas = (doc.get("components") or {}).get("schemas") or {}
    if not isinstance(schemas, dict):
        return ["components.schemas is missing or not an object"]
    errors: list[str] = []
    for name in schemas:
        if name in FORBIDDEN_SCHEMA_NAMES:
            errors.append(
                f"forbidden OpenAPI schema {name!r} "
                f"(Primer/machine integration types belong in protobuf only)"
            )
    # Deep scan for MaterializationContext as a nested title/ref leak
    text = openapi_path.read_text(encoding="utf-8")
    for bad in ("MaterializationContext", "MaterializationBundle"):
        # AuthoringBundle is allowed; exact forbidden tokens as schema keys already caught.
        if f"{bad}:" in text or f"/{bad}" in text:
            # Allow mentions in descriptions only if not schema definitions — check schema block.
            if f"    {bad}:" in text or f"\n    {bad}:" in text:
                errors.append(f"forbidden schema key pattern for {bad}")
    return errors


def scan_layout(root: Path) -> list[str]:
    errors: list[str] = []
    for rel in REQUIRED_REL_PATHS:
        p = root / rel
        if not p.exists():
            errors.append(f"missing reserved path: {rel}")
    return errors


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--studio-root",
        type=Path,
        default=None,
        help="curriculum-studio root (default: inferred)",
    )
    parser.add_argument(
        "--openapi",
        type=Path,
        default=None,
        help="OpenAPI YAML path (default: contracts/openapi/v1/curriculum-studio.yaml)",
    )
    args = parser.parse_args(argv)
    root = (args.studio_root or studio_root_from_here()).resolve()
    openapi = (
        args.openapi
        if args.openapi is not None
        else root / "contracts/openapi/v1/curriculum-studio.yaml"
    ).resolve()

    errors: list[str] = []
    if not openapi.is_file():
        errors.append(f"OpenAPI not found: {openapi}")
    else:
        errors.extend(scan_openapi(openapi))
    errors.extend(scan_layout(root))

    if errors:
        print("FAIL: ownership scan", file=sys.stderr)
        for e in errors:
            print(f"  - {e}", file=sys.stderr)
        return 1

    schemas = yaml.safe_load(openapi.read_text(encoding="utf-8"))["components"][
        "schemas"
    ]
    present_subset = sorted(n for n in schemas if n in ALLOWED_AUTHORING_SUBSET)
    print("OK: ownership scan")
    print(f"  openapi={openapi}")
    print(f"  schemas={len(schemas)}")
    print(f"  authoring_subset_present={present_subset}")
    print(f"  layout_paths={len(REQUIRED_REL_PATHS)}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
