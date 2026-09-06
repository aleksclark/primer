#!/usr/bin/env python3
"""Small source-only checks for the isolated Tasks production package."""
from pathlib import Path
import re

root = Path(__file__).resolve().parents[3]
job = (root / "deploy/nomad/jobs/primer-tasks.nomad.hcl").read_text()
image = (root / "Dockerfile.tasks").read_text()
ignore = (root / "Dockerfile.tasks.dockerignore").read_text()
env = (root / "deploy/nomad/env/tasks-home.nomadvars.hcl").read_text()
workflow = (root / ".github/workflows/tasks-image.yml").read_text()

for text in (
    'job "primer-tasks"',
    'name     = "primer-tasks"',
    'release_set      = "primer-tasks"',
    'nomadVar "nomad/jobs/primer-tasks"',
    'entrypoint      = ["/app/tasks-migrate"]',
    'hook    = "prestart"',
    'sidecar = false',
    'path     = "/health"',
    'Host(`api.primerlms.com`) && (Path(`/tasks`) || PathPrefix(`/tasks/`))',
    'traefik.http.routers.primer-tasks.priority=200',
    'traefik.http.routers.primer-tasks.tls=true',
    'TASKS_AUTH_MODE     = "clerk"',
):
    assert text in job, f"missing Tasks runtime contract: {text}"
assert 'tls.certresolver=' not in job, 'reuse existing fleet TLS certificate; no new-zone ACME'
assert job.count('readonly_rootfs = true') == 2
assert job.count('image           = var.image_primer_tasks') == 2
assert len(re.findall(r'(?m)^\s*user\s*=\s*"65532:65532"\s*$', job)) == 2
assert 'regex_replace(var.image_primer_tasks,' in job, 'use supported Nomad HCL functions'
assert 'can(regex(' not in job
assert 'shutdown_delay = "5s"' in job
lock = (root / 'deploy/nomad/images.tasks.lock.hcl').read_text()
assert re.search(r'(?m)^image_primer_tasks\s*=\s*"ghcr\.io/aleksclark/primer-tasks@sha256:[0-9a-f]{64}"$', lock)
for prohibited in ("stripprefix", "replacepath", "tasks-test-issuer", "CLERK_SECRET_KEY", "TASKS_TEST_AUTH"):
    assert prohibited.lower() not in job.lower(), f"unexpected runtime surface: {prohibited}"
assert 'command         = "/app/tasks-migrate"' not in job, "must override image ENTRYPOINT, not CMD"
assert 'tasks_public_origin = "https://api.primerlms.com"' in env
assert not re.search(r"(?i)(database_url|secret|token|api_key)\s*=", env)

for binary in ("tasks-server", "tasks-migrate", "tasks-bootstrap"):
    assert f"./cmd/{binary}" in image
assert 'ENTRYPOINT ["/app/tasks-server"]' in image
assert 'USER 65532:65532' in image
assert 'COPY --from=web /src/primer-tasks/web/dist/ /app/web/' in image
assert 'COPY --from=source /usr/share/zoneinfo/' in image
assert 'test -n "$VITE_CLERK_PUBLISHABLE_KEY"' in image
assert 'type=secret,id=VITE_CLERK_PUBLISHABLE_KEY,env=VITE_CLERK_PUBLISHABLE_KEY,required=true' in image
assert 'ARG VITE_CLERK_PUBLISHABLE_KEY' not in image
assert 'no-cache-filters: web' in workflow
assert '--no-cache-filter web' in (root / 'scripts/build-tasks-image.sh').read_text()
assert 'ENV GOWORK=off CGO_ENABLED=0' in image
assert 'GOPRIVATE=' not in image, 'public VCS-qualified module uses public proxy/checksum verification'
assert "cmd/tasks-test-issuer" not in image + ignore
assert "**/.env" in ignore and "**/.env.*" in ignore
assert "COPY . " not in image, "production sources must stay allowlisted"
assert "CLERK_SECRET_KEY" not in image + workflow
assert "nomad job run" not in workflow and "deploy/deploy.sh" not in workflow
assert "provenance: mode=max" in workflow and "sbom: true" in workflow
assert "primer-tasks:sha-${{ github.sha }}" in workflow
assert ":latest" not in workflow
print("Tasks package contract OK (source only; not image or deployment proof)")
