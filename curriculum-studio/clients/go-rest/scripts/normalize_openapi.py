#!/usr/bin/env python3
"""Deterministically lower the emitted 3.1 schema subset for ogen.

The source remains the Huma-emitted 3.1 document. ogen currently accepts the
same schemas in 3.0 form but not JSON Schema's `type: [T, null]` spelling, so
this ignored client-build IR changes nullable spelling, removes the 3.1
dialect marker, and lowers binary contentMediaType (already represented by
3.0 format: binary and the response Content-Type).
"""
from __future__ import annotations

import sys
from pathlib import Path

import yaml


def normalize(value):
    if isinstance(value, dict):
        out = {key: normalize(item) for key, item in value.items()}
        if out.get("format") == "binary":
            out.pop("contentMediaType", None)
        typ = out.get("type")
        if isinstance(typ, list) and "null" in typ:
            non_null = [item for item in typ if item != "null"]
            if len(non_null) == 1:
                out["type"] = non_null[0]
                out["nullable"] = True
        return out
    if isinstance(value, list):
        return [normalize(item) for item in value]
    return value


if len(sys.argv) != 3:
    raise SystemExit("usage: normalize_openapi.py INPUT OUTPUT")
source, target = map(Path, sys.argv[1:])
doc = normalize(yaml.safe_load(source.read_text(encoding="utf-8")))
doc["openapi"] = "3.0.3"
doc.pop("jsonSchemaDialect", None)
target.parent.mkdir(parents=True, exist_ok=True)
target.write_text(yaml.safe_dump(doc, sort_keys=False), encoding="utf-8")
