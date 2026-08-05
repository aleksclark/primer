#!/usr/bin/env python3
"""Validate and deploy the basic Linux course via Primer's guarded import API.

Default path (Phase 6):
  1. Local schema validation
  2. POST /curriculum/import/plan
  3. Display human-readable + machine-readable diff
  4. POST /curriculum/import/apply with the planned bundle digest
  5. Write a local result manifest

Assignment/enrollment is never implied. Use --assign only as an explicit
follow-up against an already-imported activity slug.
"""

from __future__ import annotations

import argparse
import json
import os
import sys
import urllib.error
import urllib.parse
import urllib.request
from pathlib import Path


def load_json(path: Path):
    with path.open(encoding="utf-8") as handle:
        return json.load(handle)


def request_json(base_url, method, path, token=None, body=None):
    data = None if body is None else json.dumps(body).encode()
    headers = {"Accept": "application/json"}
    if body is not None:
        headers["Content-Type"] = "application/json"
    if token:
        headers["Authorization"] = "Bearer " + token
    request = urllib.request.Request(
        base_url.rstrip("/") + path, data=data, headers=headers, method=method
    )
    try:
        with urllib.request.urlopen(request) as response:
            payload = response.read()
    except urllib.error.HTTPError as error:
        detail = error.read().decode(errors="replace")
        raise RuntimeError(f"{method} {path}: HTTP {error.code}: {detail}") from error
    except urllib.error.URLError as error:
        raise RuntimeError(f"{method} {path}: {error.reason}") from error
    return json.loads(payload) if payload else {}


def validate_course(root: Path, manifest: dict):
    try:
        from jsonschema import Draft202012Validator
    except ImportError as error:
        raise RuntimeError("jsonschema is required: python3 -m pip install jsonschema") from error

    course_schema_path = root / "course.schema.json"
    if course_schema_path.exists():
        course_validator = Draft202012Validator(load_json(course_schema_path))
        course_errors = sorted(course_validator.iter_errors(manifest), key=lambda entry: list(entry.path))
        if course_errors:
            locations = []
            for error in course_errors:
                location = ".".join(str(part) for part in error.path) or "<root>"
                locations.append(f"course.json:{location}: {error.message}")
            raise RuntimeError("\n".join(locations))

    schema = load_json(root / "activity.schema.json")
    validator = Draft202012Validator(schema)
    activities = manifest.get("activities", [])
    if not activities:
        raise RuntimeError("course.json has no activities")

    documents = []
    seen_slugs = set()
    for expected_order, item in enumerate(activities, start=1):
        if item.get("order") != expected_order:
            raise RuntimeError(f"course order must be contiguous at {expected_order}")
        path = root / item["file"]
        document = load_json(path)
        errors = sorted(validator.iter_errors(document), key=lambda entry: list(entry.path))
        if errors:
            locations = []
            for error in errors:
                location = ".".join(str(part) for part in error.path) or "<root>"
                locations.append(f"{path}:{location}: {error.message}")
            raise RuntimeError("\n".join(locations))
        if document["slug"] != item["slug"]:
            raise RuntimeError(f"{path}: slug does not match course.json")
        if document["slug"] in seen_slugs:
            raise RuntimeError(f"duplicate slug: {document['slug']}")
        seen_slugs.add(document["slug"])
        documents.append((item, document))
    return documents


def parent_token(args):
    if args.token:
        return args.token
    email = args.email or os.getenv("PRIMER_PARENT_EMAIL")
    password = args.password or os.getenv("PRIMER_PARENT_PASSWORD")
    if not email or not password:
        raise RuntimeError("set --token, or provide parent email and password")
    response = request_json(args.api, "POST", "/auth/login", body={"email": email, "password": password})
    return response["token"]


def load_optional_standards(path: Path | None):
    if path is None or not path.exists():
        return []
    # Accept either {"standards":[...]} YAML-converted JSON or a bare list.
    # For MVP loaders, prefer a JSON export; YAML requires PyYAML.
    if path.suffix in {".yaml", ".yml"}:
        try:
            import yaml  # type: ignore
        except ImportError as error:
            raise RuntimeError("PyYAML required to load standards YAML: pip install pyyaml") from error
        with path.open(encoding="utf-8") as handle:
            data = yaml.safe_load(handle)
    else:
        data = load_json(path)
    if isinstance(data, dict):
        return data.get("standards") or []
    if isinstance(data, list):
        return data
    raise RuntimeError(f"unsupported standards file shape: {path}")


def build_import_bundle(root: Path, manifest: dict, documents, standards, source_label: str):
    # Course document for the import API (drop local file paths from activities).
    course = {
        "schemaVersion": manifest.get("schemaVersion", "1"),
        "slug": manifest["slug"],
        "title": manifest["title"],
        "subjectCode": manifest.get("subjectCode") or manifest.get("subject_code") or "digital-literacy",
    }
    for key in ("version", "parentDescription", "revisionPolicy", "pacingReference",
                "prerequisites", "gates", "remediation", "modules", "continuityDefaults", "metadata"):
        if key in manifest:
            course[key] = manifest[key]
        # also accept snake_case from older manifests
        snake = "".join(["_" + c.lower() if c.isupper() else c for c in key]).lstrip("_")
        if snake in manifest and key not in course:
            course[key] = manifest[snake]

    course_activities = []
    for item, document in documents:
        ref = {"order": item["order"], "slug": document["slug"]}
        for opt in ("module", "capstone", "continuity", "metadata"):
            if opt in item:
                ref[opt] = item[opt]
        course_activities.append(ref)
    course["activities"] = course_activities

    # Normalize standards seeds to API shape (snake_case fields from YAML).
    std_out = []
    for s in standards:
        if not isinstance(s, dict):
            continue
        std_out.append({
            "code": s.get("code"),
            "source": s.get("source") or "custom",
            "subjectCode": s.get("subject_code") or s.get("subjectCode") or course["subjectCode"],
            "gradeLevel": s.get("grade_level", s.get("gradeLevel")),
            "domain": s.get("domain") or "",
            "cluster": s.get("cluster") or "",
            "description": s.get("description") or s.get("code"),
            "masteryCriteria": s.get("mastery_criteria") or s.get("masteryCriteria") or [],
        })

    activities = [document for _, document in documents]
    return {
        "schemaVersion": "1",
        "sourceLabel": source_label,
        "version": manifest.get("version") or "1",
        "standards": std_out,
        "activities": activities,
        "course": course,
    }


def print_plan(plan: dict):
    print(f"bundleDigest: {plan.get('bundleDigest')}")
    print(f"valid: {plan.get('valid')}")
    for w in plan.get("warnings") or []:
        print(f"warning: {w}")
    for e in plan.get("errors") or []:
        print(f"error: {e}", file=sys.stderr)
    print("actions:")
    for action in plan.get("actions") or []:
        kind = action.get("kind")
        target = action.get("slug") or action.get("code") or ""
        act = action.get("action")
        detail = action.get("detail") or ""
        digest = action.get("digest") or ""
        extra = f" digest={digest}" if digest else ""
        if detail:
            extra += f" ({detail})"
        print(f"  - {kind} {target}: {act}{extra}")


def assign_activity(api, token, student_id, slug, priority, reason):
    return request_json(
        api,
        "POST",
        f"/students/{student_id}/assign-next",
        token=token,
        body={"slug": slug, "priority": priority, "reason": reason},
    )


def parse_args():
    parser = argparse.ArgumentParser(
        description="Validate and import the basic Linux course via plan/apply (no implicit enrollment)."
    )
    parser.add_argument("--api", default=os.getenv("PRIMER_API", "http://127.0.0.1:8080/api/v1"))
    parser.add_argument("--student-id", default=os.getenv("PRIMER_STUDENT_ID"))
    parser.add_argument("--token", default=os.getenv("PRIMER_PARENT_TOKEN"))
    parser.add_argument("--email")
    parser.add_argument("--password")
    parser.add_argument("--validate-only", action="store_true")
    parser.add_argument("--plan-only", action="store_true", help="run import plan and exit")
    parser.add_argument("--dry-run", action="store_true", help="alias for --plan-only")
    parser.add_argument(
        "--standards",
        default=os.getenv("PRIMER_STANDARDS_JSON", ""),
        help="optional standards JSON/YAML to include in the import bundle",
    )
    parser.add_argument(
        "--manifest-out",
        default="import-result.json",
        help="path to write the durable apply result manifest",
    )
    parser.add_argument(
        "--assign",
        action="store_true",
        help="explicitly assign imported activities to --student-id AFTER import (never implied)",
    )
    parser.add_argument("--priority", type=int, default=0)
    parser.add_argument(
        "--legacy-http",
        action="store_true",
        help="use per-activity create/publish endpoints instead of plan/apply",
    )
    # Backward-compat flags from earlier loader.
    parser.add_argument("--no-assign", action="store_true", help=argparse.SUPPRESS)
    return parser.parse_args()


def legacy_http_load(args, root, manifest, documents):
    """Previous one-request-at-a-time path kept for emergency use."""
    token = parent_token(args)
    for item, document in documents:
        query = urllib.parse.urlencode({"filter": "slug:" + document["slug"], "limit": 100})
        response = request_json(args.api, "GET", "/learning-activities?" + query, token=token)
        activity = None
        for entry in response.get("items", []):
            if entry.get("slug") == document["slug"]:
                activity = entry
                break
        if activity is None:
            activity = request_json(
                args.api,
                "POST",
                "/learning-activities",
                token=token,
                body={
                    "slug": document["slug"],
                    "title": document["title"],
                    "summary": document["summary"],
                    "kind": document["kind"],
                },
            )
        revision = request_json(
            args.api,
            "POST",
            f"/learning-activities/{activity['id']}/revisions",
            token=token,
            body={
                "schemaVersion": document["schemaVersion"],
                "content": document["content"],
                "standards": document["standards"],
            },
        )
        print(f"published {document['slug']} revision {revision.get('revision', '?')}")
        if args.assign and args.student_id:
            reason = f"{manifest['title']} reference lesson {item['order']}; advance by mastery, not date"
            assignment = assign_activity(args.api, token, args.student_id, document["slug"], args.priority, reason)
            created = assignment.get("created", False)
            print("  " + ("assigned" if created else "kept existing assignment"))


def main():
    args = parse_args()
    root = Path(__file__).resolve().parent
    manifest = load_json(root / "course.json")
    documents = validate_course(root, manifest)
    print(f"validated {len(documents)} activities", file=sys.stderr)

    if args.validate_only:
        return 0

    # --no-assign remains the default; --assign is opt-in.
    if args.no_assign:
        args.assign = False
    if args.assign and not args.student_id:
        raise RuntimeError("--student-id is required with --assign")

    if args.legacy_http:
        if args.dry_run or args.plan_only:
            for _, document in documents:
                print(f"would legacy-publish: {document['slug']}")
            return 0
        legacy_http_load(args, root, manifest, documents)
        return 0

    standards_path = Path(args.standards) if args.standards else None
    # Default: try repo curriculum standards if present relative to content root.
    if standards_path is None:
        candidate = root.parents[2] / "curriculum" / "standards" / "digital-literacy.yaml"
        if candidate.exists():
            standards_path = candidate
    standards = load_optional_standards(standards_path)
    bundle = build_import_bundle(root, manifest, documents, standards, source_label="basic_linux/load.py")

    if args.dry_run or args.plan_only:
        # Local preview without server when no credentials — still show counts.
        try:
            token = parent_token(args)
        except RuntimeError:
            print(f"would plan/apply bundle: activities={len(bundle['activities'])} "
                  f"standards={len(bundle['standards'])} course={bundle['course']['slug']}")
            print("note: provide parent credentials to run a server-side plan")
            return 0
        plan = request_json(args.api, "POST", "/curriculum/import/plan", token=token, body=bundle)
        print_plan(plan)
        print(json.dumps(plan, indent=2, sort_keys=True))
        if not plan.get("valid"):
            return 1
        return 0

    token = parent_token(args)
    plan = request_json(args.api, "POST", "/curriculum/import/plan", token=token, body=bundle)
    print_plan(plan)
    if not plan.get("valid"):
        raise RuntimeError("import plan is invalid; refusing apply")

    digest = plan["bundleDigest"]
    result = request_json(
        args.api,
        "POST",
        "/curriculum/import/apply",
        token=token,
        body={"bundle": bundle, "bundleDigest": digest, "sourceLabel": bundle["sourceLabel"]},
    )
    manifest_out = Path(args.manifest_out)
    manifest_out.write_text(json.dumps(result, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    print(f"wrote result manifest: {manifest_out}")
    applied = result.get("manifest") or {}
    print(f"applied digest={applied.get('bundleDigest')} documents={len(applied.get('documents') or [])}")
    if applied.get("enrolledStudents") or applied.get("assignedStudents"):
        raise RuntimeError("server enrolled/assigned students during import — this is a contract violation")

    if args.assign:
        for item, document in documents:
            reason = f"{manifest['title']} reference lesson {item['order']}; advance by mastery, not date"
            assignment = assign_activity(args.api, token, args.student_id, document["slug"], args.priority, reason)
            created = assignment.get("created", False)
            print(f"assign {document['slug']}: " + ("created" if created else "existing"))
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except (KeyError, OSError, ValueError, RuntimeError) as error:
        print(f"load.py: {error}", file=sys.stderr)
        raise SystemExit(1)
