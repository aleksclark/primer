#!/usr/bin/env python3

import argparse
import json
import os
import sys
import urllib.error
import urllib.parse
import urllib.request
from pathlib import Path


def load_json(path):
    with path.open(encoding="utf-8") as handle:
        return json.load(handle)


def request_json(base_url, method, path, token=None, body=None):
    data = None if body is None else json.dumps(body).encode()
    headers = {"Accept": "application/json"}
    if body is not None:
        headers["Content-Type"] = "application/json"
    if token:
        headers["Authorization"] = "Bearer " + token
    request = urllib.request.Request(base_url.rstrip("/") + path, data=data, headers=headers, method=method)
    try:
        with urllib.request.urlopen(request) as response:
            payload = response.read()
    except urllib.error.HTTPError as error:
        detail = error.read().decode(errors="replace")
        raise RuntimeError(f"{method} {path}: HTTP {error.code}: {detail}") from error
    except urllib.error.URLError as error:
        raise RuntimeError(f"{method} {path}: {error.reason}") from error
    return json.loads(payload) if payload else {}


def validate_course(root, manifest):
    try:
        from jsonschema import Draft202012Validator
    except ImportError as error:
        raise RuntimeError("jsonschema is required: python3 -m pip install jsonschema") from error

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


def find_activity(api, token, slug):
    query = urllib.parse.urlencode({"filter": "slug:" + slug, "limit": 100})
    response = request_json(api, "GET", "/learning-activities?" + query, token=token)
    for item in response.get("items", []):
        if item.get("slug") == slug:
            return item
    return None


def create_activity(api, token, document):
    response = request_json(
        api,
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
    return response


def publish_revision(api, token, activity_id, document):
    return request_json(
        api,
        "POST",
        f"/learning-activities/{activity_id}/revisions",
        token=token,
        body={
            "schemaVersion": document["schemaVersion"],
            "content": document["content"],
            "standards": document["standards"],
        },
    )


def assign_activity(api, token, student_id, document, priority, reason):
    return request_json(
        api,
        "POST",
        f"/students/{student_id}/assign-next",
        token=token,
        body={"slug": document["slug"], "priority": priority, "reason": reason},
    )


def parse_args():
    parser = argparse.ArgumentParser(description="Validate, publish, and assign the basic Linux course in Primer.")
    parser.add_argument("--api", default=os.getenv("PRIMER_API", "http://127.0.0.1:8080/api/v1"))
    parser.add_argument("--student-id", default=os.getenv("PRIMER_STUDENT_ID"))
    parser.add_argument("--token", default=os.getenv("PRIMER_PARENT_TOKEN"))
    parser.add_argument("--email")
    parser.add_argument("--password")
    parser.add_argument("--validate-only", action="store_true")
    parser.add_argument("--dry-run", action="store_true")
    parser.add_argument("--no-assign", action="store_true")
    parser.add_argument("--priority", type=int, default=0)
    return parser.parse_args()


def main():
    args = parse_args()
    root = Path(__file__).resolve().parent
    manifest = load_json(root / "course.json")
    documents = validate_course(root, manifest)
    print(f"validated {len(documents)} activities", file=sys.stderr)

    if args.validate_only:
        return 0
    if not args.no_assign and not args.student_id:
        raise RuntimeError("--student-id is required unless --no-assign is set")
    if args.dry_run:
        for _, document in documents:
            action = "publish"
            if not args.no_assign:
                action += " and assign"
            print(f"would {action}: {document['slug']}")
        return 0

    token = parent_token(args)
    for item, document in documents:
        activity = find_activity(args.api, token, document["slug"])
        if activity is None:
            activity = create_activity(args.api, token, document)
        revision = publish_revision(args.api, token, activity["id"], document)
        message = f"published {document['slug']} revision {revision.get('revision', '?')}"
        if not args.no_assign:
            reason = f"{manifest['title']} reference lesson {item['order']}; advance by mastery, not date"
            assignment = assign_activity(args.api, token, args.student_id, document, args.priority, reason)
            created = assignment.get("created", False)
            message += " and " + ("assigned" if created else "kept existing assignment")
        print(message)
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except (KeyError, OSError, ValueError, RuntimeError) as error:
        print(f"load.py: {error}", file=sys.stderr)
        raise SystemExit(1)
