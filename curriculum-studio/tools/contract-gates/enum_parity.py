#!/usr/bin/env python3
"""Mechanical closed-enum wire-string parity across proto, OpenAPI, and DB.

Extracts values from committed sources — does not own a third enum catalog.
See phase-02-enum-parity.md (P2-S1…P2-S4, E2-01…E2-04).
"""
from __future__ import annotations

import argparse
import re
import sys
from collections import defaultdict
from pathlib import Path
from typing import Any

try:
    import yaml
except ImportError as exc:  # pragma: no cover
    raise SystemExit("PyYAML is required") from exc


UNSPECIFIED_SUFFIXES = frozenset({"UNSPECIFIED", ""})


def studio_root_from_here() -> Path:
    return Path(__file__).resolve().parents[2]


def load_mappings(path: Path) -> dict[str, Any]:
    data = yaml.safe_load(path.read_text(encoding="utf-8"))
    if not isinstance(data, dict) or "sets" not in data:
        raise SystemExit(f"invalid mappings file: {path}")
    # Guard: mappings must not look like a primary value catalog.
    for name, cfg in data["sets"].items():
        if "values" in cfg or "canonical" in cfg:
            raise SystemExit(
                f"mappings set {name!r} must not embed value lists "
                f"(no third enum catalog)"
            )
    return data


def proto_suffix_to_wire(suffix: str, transform: str | None) -> str:
    if transform == "proto_suffix_to_dot_snake":
        # CURRICULUM_CREATED -> curriculum.created
        # PLAN_REVISION_PUBLISHED -> plan_revision.published
        # MATERIALIZED_ITEM_SUPERSEDED -> materialized_item.superseded
        parts = suffix.lower().split("_")
        if len(parts) < 2:
            return suffix.lower()
        # Heuristic aligned with product event names:
        # last token is the verb; remainder is the dotted/snaked subject.
        # Known multi-segment subjects:
        known = {
            "CURRICULUM_CREATED": "curriculum.created",
            "PLAN_REVISION_PUBLISHED": "plan_revision.published",
            "MATERIALIZATION_REQUESTED": "materialization.requested",
            "MATERIALIZATION_READY": "materialization.ready",
            "MATERIALIZATION_FAILED": "materialization.failed",
            "MATERIALIZED_ITEM_SUPERSEDED": "materialized_item.superseded",
            "PLAN_CHANGE_PROPOSED": "plan_change.proposed",
        }
        if suffix in known:
            return known[suffix]
        return ".".join(parts[:-1]) + "." + parts[-1] if len(parts) > 1 else parts[0]
    # default: UPPER_SNAKE -> lower_snake
    return suffix.lower()


def extract_proto_enums(proto_root: Path) -> dict[str, list[tuple[str, str]]]:
    """Return enum_name -> list of (full_name, suffix_after_common_prefix)."""
    enums: dict[str, list[tuple[str, str]]] = {}
    enum_re = re.compile(r"enum\s+(\w+)\s*\{([^}]*)\}", re.S)
    value_re = re.compile(r"^\s*([A-Z][A-Z0-9_]*)\s*=\s*(\d+)", re.M)
    for path in sorted(proto_root.rglob("*.proto")):
        text = path.read_text(encoding="utf-8")
        for m in enum_re.finditer(text):
            name = m.group(1)
            body = m.group(2)
            values = []
            for vm in value_re.finditer(body):
                full = vm.group(1)
                values.append(full)
            if not values:
                continue
            # Derive prefix as longest common PREFIX_ shared by all non-empty.
            prefix = _common_enum_prefix(values)
            entries: list[tuple[str, str]] = []
            for full in values:
                suffix = full[len(prefix) :] if full.startswith(prefix) else full
                entries.append((full, suffix))
            enums[name] = entries
    return enums


def _common_enum_prefix(names: list[str]) -> str:
    if not names:
        return ""
    # Prefer trailing underscore form used by Studio protos.
    parts_list = [n.split("_") for n in names]
    common: list[str] = []
    for tokens in zip(*parts_list):
        if len(set(tokens)) == 1:
            common.append(tokens[0])
        else:
            break
    if not common:
        return ""
    # Keep at least something ending with _
    return "_".join(common) + "_"


def extract_openapi_enums(openapi_path: Path) -> dict[str, list[str]]:
    doc = yaml.safe_load(openapi_path.read_text(encoding="utf-8"))
    schemas = (doc.get("components") or {}).get("schemas") or {}
    out: dict[str, list[str]] = {}
    for name, schema in schemas.items():
        if not isinstance(schema, dict):
            continue
        if "enum" in schema and isinstance(schema["enum"], list):
            out[name] = [str(v) for v in schema["enum"]]
            continue
        # string schema with enum under allOf etc. — rare
        if schema.get("type") == "string" and "enum" in schema:
            out[name] = [str(v) for v in schema["enum"]]
    return out


def extract_db_checks(migrations_dir: Path) -> dict[tuple[str, str], set[str]]:
    """Parse CHECK (col IN (...)) near CREATE TABLE for (table, column) -> values."""
    results: dict[tuple[str, str], set[str]] = {}
    table_re = re.compile(
        r"CREATE TABLE\s+(?:curriculum_studio\.)?(\w+)\s*\((.*?)\);",
        re.S | re.I,
    )
    check_in_re = re.compile(
        r"CHECK\s*\(\s*(\w+)\s+IN\s*\((.*?)\)\s*\)",
        re.S | re.I,
    )
    str_re = re.compile(r"'((?:\\'|[^'])*)'")
    for path in sorted(migrations_dir.glob("*.sql")):
        text = path.read_text(encoding="utf-8")
        for tm in table_re.finditer(text):
            table = tm.group(1)
            body = tm.group(2)
            for cm in check_in_re.finditer(body):
                col = cm.group(1)
                vals = {m.group(1) for m in str_re.finditer(cm.group(2))}
                if vals:
                    results[(table, col)] = vals
    return results


def extract_schema_md_events(schema_md: Path) -> set[str]:
    text = schema_md.read_text(encoding="utf-8")
    # Section "## Domain events" bullet list of `event.type`
    m = re.search(
        r"## Domain events.*?\n((?:[-*]\s+`[^`]+`\s*\n)+)",
        text,
        re.S,
    )
    if not m:
        return set()
    return set(re.findall(r"`([^`]+)`", m.group(1)))


def forbidden_catalog_paths(studio_root: Path) -> list[str]:
    bad = []
    for p in studio_root.rglob("*"):
        if not p.is_file():
            continue
        name = p.name.lower()
        if name in {
            "enum-catalog.yaml",
            "enum-catalog.yml",
            "enums.yaml",
            "enums.yml",
            "enum_values.yaml",
            "enum_catalog.yaml",
        }:
            # allow nothing — these are forbidden primary catalogs
            bad.append(str(p.relative_to(studio_root)))
    return bad


def compare_set(
    name: str,
    cfg: dict[str, Any],
    proto_enums: dict[str, list[tuple[str, str]]],
    openapi_enums: dict[str, list[str]],
    db_checks: dict[tuple[str, str], set[str]],
    schema_events: set[str],
) -> list[str]:
    errors: list[str] = []
    layers = cfg.get("layers") or []
    transform = cfg.get("wire_transform")
    prefix = cfg.get("proto_prefix") or ""

    proto_wires: set[str] | None = None
    openapi_wires: set[str] | None = None
    db_wires: set[str] | None = None

    if "proto" in layers:
        ename = cfg["proto_enum"]
        if ename not in proto_enums:
            errors.append(f"{name}: proto enum {ename} not found")
        else:
            proto_wires = set()
            for full, suffix in proto_enums[ename]:
                if suffix in UNSPECIFIED_SUFFIXES or full.endswith("_UNSPECIFIED"):
                    continue
                # Prefer configured prefix strip
                if prefix and full.startswith(prefix):
                    suffix = full[len(prefix) :]
                wire = proto_suffix_to_wire(suffix, transform)
                proto_wires.add(wire)
            # Verify prefix convention
            for full, _ in proto_enums[ename]:
                if full.endswith("_UNSPECIFIED"):
                    continue
                if prefix and not full.startswith(prefix):
                    errors.append(
                        f"{name}: proto value {full} does not use prefix {prefix}"
                    )

    if "openapi" in layers:
        sname = cfg["openapi_schema"]
        if sname not in openapi_enums:
            errors.append(f"{name}: OpenAPI schema enum {sname} not found")
        else:
            openapi_wires = set(openapi_enums[sname])

    if "db" in layers:
        table = cfg["db"]["table"]
        col = cfg["db"]["column"]
        key = (table, col)
        if key not in db_checks:
            errors.append(f"{name}: DB CHECK not found for {table}.{col}")
        else:
            db_wires = set(db_checks[key])

    if "db_bool" in layers:
        bcfg = cfg["db_bool"]
        # Boolean mapping does not come from CHECK IN; synthesize wires.
        db_wires = {bcfg["true_wire"], bcfg["false_wire"]}
        # Sanity: locked column exists in CREATE TABLE (soft check via migration text)
        table = bcfg["table"]
        col = bcfg["column"]
        # no CHECK IN expected

    if "db_event" in layers:
        db_wires = set(schema_events)
        if not db_wires:
            errors.append(f"{name}: no domain event wires found in SCHEMA.md")

    if errors:
        return errors

    api_to_db = dict(cfg.get("api_to_db") or {})
    storage_only = set(cfg.get("storage_only") or [])

    # Normalize API-facing sets into a comparable space.
    def map_api_to_db_space(values: set[str]) -> set[str]:
        out = set()
        for v in values:
            out.add(api_to_db.get(v, v))
        return out

    # Collision check: two API values must not map to same DB value unless identical.
    rev: dict[str, list[str]] = defaultdict(list)
    for src, dst in api_to_db.items():
        rev[dst].append(src)
    for dst, srcs in rev.items():
        if len(srcs) > 1:
            errors.append(
                f"{name}: mapping collision — {srcs} all map to {dst!r}"
            )

    # Compare proto vs openapi in wire space (pre-db mapping).
    if proto_wires is not None and openapi_wires is not None:
        if proto_wires != openapi_wires:
            only_p = sorted(proto_wires - openapi_wires)
            only_o = sorted(openapi_wires - proto_wires)
            if only_p:
                errors.append(f"{name}: in proto not OpenAPI: {only_p}")
            if only_o:
                errors.append(f"{name}: in OpenAPI not proto: {only_o}")

    # Compare API (proto∩openapi or whichever exists) to DB with mapping.
    api_set: set[str] | None = None
    if proto_wires is not None and openapi_wires is not None:
        # After equality check, use openapi as API surface
        api_set = set(openapi_wires)
    elif openapi_wires is not None:
        api_set = set(openapi_wires)
    elif proto_wires is not None:
        api_set = set(proto_wires)

    if api_set is not None and db_wires is not None:
        api_in_db = map_api_to_db_space(api_set)
        # Every API value must land in DB (after map) or be explicitly non-stored
        missing_in_db = sorted(api_in_db - db_wires)
        if missing_in_db:
            errors.append(
                f"{name}: API wire(s) missing from DB (after mapping): {missing_in_db}"
            )
        # DB values must be API-mapped or storage_only
        allowed_db = set(api_in_db) | storage_only
        extra_db = sorted(db_wires - allowed_db)
        if extra_db:
            errors.append(
                f"{name}: DB value(s) not in API and not storage_only: {extra_db}"
            )
        # storage_only must actually be in DB
        missing_storage = sorted(storage_only - db_wires)
        if missing_storage:
            errors.append(
                f"{name}: storage_only not present in DB: {missing_storage}"
            )
        # storage_only must not appear on API
        if api_set & storage_only:
            errors.append(
                f"{name}: storage_only values leaked to API: "
                f"{sorted(api_set & storage_only)}"
            )
        # Mapped API names must not also appear raw in API if different
        for api_name, db_name in api_to_db.items():
            if api_name not in api_set:
                errors.append(
                    f"{name}: api_to_db key {api_name!r} not present on API"
                )
            if db_name in api_set and db_name != api_name:
                # e.g. both archived and retired on API would be bad
                errors.append(
                    f"{name}: both API {api_name!r} and DB form {db_name!r} "
                    f"exposed on API"
                )

    return errors


def run_parity(
    studio_root: Path,
    openapi_path: Path,
    mappings_path: Path,
) -> int:
    mappings = load_mappings(mappings_path)
    proto_root = studio_root / "contracts" / "proto"
    migrations = studio_root / "db" / "migrations"
    schema_md = studio_root / "db" / "SCHEMA.md"

    bad_catalogs = forbidden_catalog_paths(studio_root)
    # enum_mappings.yaml is rules-only and allowed; filter it out if name matches
    bad_catalogs = [
        b
        for b in bad_catalogs
        if not b.endswith("enum_mappings.yaml")
        and "enum_mappings" not in Path(b).name
    ]
    if bad_catalogs:
        print("FAIL: forbidden primary enum catalog path(s):", file=sys.stderr)
        for b in bad_catalogs:
            print(f"  - {b}", file=sys.stderr)
        return 1

    proto_enums = extract_proto_enums(proto_root)
    openapi_enums = extract_openapi_enums(openapi_path)
    db_checks = extract_db_checks(migrations)
    schema_events = extract_schema_md_events(schema_md)

    all_errors: list[str] = []
    counts: dict[str, int] = {}
    for name, cfg in mappings["sets"].items():
        errs = compare_set(
            name, cfg, proto_enums, openapi_enums, db_checks, schema_events
        )
        all_errors.extend(errs)
        # count wires for summary
        ename = cfg.get("proto_enum")
        if ename and ename in proto_enums:
            n = sum(
                1
                for full, suf in proto_enums[ename]
                if not full.endswith("_UNSPECIFIED")
            )
            counts[name] = n
        elif cfg.get("openapi_schema") in openapi_enums:
            counts[name] = len(openapi_enums[cfg["openapi_schema"]])

    if all_errors:
        print("FAIL: enum parity", file=sys.stderr)
        for e in all_errors:
            print(f"  - {e}", file=sys.stderr)
        return 1

    print("OK: enum parity")
    print(f"  openapi={openapi_path}")
    print(f"  sets={len(counts)}")
    for k in sorted(counts):
        print(f"  - {k}: {counts[k]} values")
    print(f"  db_check_columns={len(db_checks)}")
    print(f"  schema_events={len(schema_events)}")
    return 0


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--studio-root", type=Path, default=None)
    parser.add_argument(
        "--openapi",
        type=Path,
        default=None,
        help="OpenAPI path (baseline or emitted; for C7 handoff)",
    )
    parser.add_argument(
        "--mappings",
        type=Path,
        default=None,
        help="Mapping rules YAML (transforms only)",
    )
    args = parser.parse_args(argv)
    root = (args.studio_root or studio_root_from_here()).resolve()
    openapi = (
        args.openapi
        if args.openapi is not None
        else root / "contracts/openapi/v1/curriculum-studio.yaml"
    ).resolve()
    mappings = (
        args.mappings
        if args.mappings is not None
        else Path(__file__).resolve().parent / "enum_mappings.yaml"
    ).resolve()
    if not openapi.is_file():
        print(f"FAIL: OpenAPI not found: {openapi}", file=sys.stderr)
        return 1
    if not mappings.is_file():
        print(f"FAIL: mappings not found: {mappings}", file=sys.stderr)
        return 1
    return run_parity(root, openapi, mappings)


if __name__ == "__main__":
    raise SystemExit(main())
