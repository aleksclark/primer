#!/usr/bin/env python3
"""Non-mutating public mount/auth preflight. Does NOT prove real Clerk login.
Run the interactive parent -> schedule -> paired student -> approval checklist
in the release note as well. No tokens, passwords, or cookies are accepted here.
"""
import json
import os
import urllib.error
import urllib.request

origin = os.environ.get("TASKS_SMOKE_ORIGIN", "https://api.primerlms.com").rstrip("/")
if not origin.startswith("https://") or "/" in origin.removeprefix("https://"):
    raise SystemExit("TASKS_SMOKE_ORIGIN must be an exact HTTPS origin")

class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None

opener = urllib.request.build_opener(NoRedirect)
def check(path, status, headers=None):
    req = urllib.request.Request(origin + path, headers=headers or {})
    try:
        response = opener.open(req, timeout=15)
    except urllib.error.HTTPError as error:
        response = error
    with response:
        body = response.read(1 << 20)
        if response.status != status:
            raise SystemExit(f"FAIL {path}: HTTP {response.status}, expected {status}")
        print(f"PASS {path}: HTTP {status}")
        return response.headers, body

headers, _ = check("/tasks", 308)
assert headers.get("Location") == "/tasks/", "incorrect base redirect"
headers, body = check("/tasks/", 200)
assert b'/tasks/assets/' in body and 'text/html' in headers.get('Content-Type', ''), "wrong SPA/asset base"
_, body = check("/tasks/api/health", 200)
assert json.loads(body)["status"] == "ok", "Tasks health missing"
for headers in ({}, {"Authorization": "Bearer deliberately-invalid"}):
    response, _ = check("/tasks/api/students", 401, headers)
    assert response.get("WWW-Authenticate") == "Bearer", "missing bearer challenge"
headers, _ = check("/tasks/api/auth/login", 302)
assert headers.get("Location") == "/tasks/parent/students", "login return escaped Tasks mount"
check("/tasks/api/auth/callback", 410)
print("PUBLIC PREFLIGHT ONLY; real Clerk sign-in + household workflow remains mandatory.")
