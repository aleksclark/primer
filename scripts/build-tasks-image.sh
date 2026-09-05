#!/usr/bin/env bash
# Local production-image build only. This script never publishes or deploys.
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."

if [[ -n "$(git status --porcelain)" ]]; then
  echo 'Refusing a release build from a dirty worktree; commit the combined app + packaging first.' >&2
  exit 1
fi
if [[ -z "${VITE_CLERK_PUBLISHABLE_KEY:-}" ]]; then
  echo 'Missing approved build input NAME: VITE_CLERK_PUBLISHABLE_KEY (no fallback key).' >&2
  exit 1
fi
revision=$(git rev-parse HEAD)
image="ghcr.io/aleksclark/primer-tasks:sha-${revision}"
mkdir -p tmp/tasks-image

docker buildx build \
  --file Dockerfile.tasks \
  --platform linux/amd64 \
  --no-cache-filter web \
  --build-arg "SOURCE_REVISION=${revision}" \
  --build-arg VITE_TASKS_BASE_PATH=/tasks/ \
  --build-arg VITE_TASKS_API_BASE=/tasks/api \
  --secret id=VITE_CLERK_PUBLISHABLE_KEY,env=VITE_CLERK_PUBLISHABLE_KEY \
  --metadata-file tmp/tasks-image/build-metadata.json \
  --tag "$image" \
  --load .

# The local image ID is not a registry digest. Publication must obtain and lock
# the pushed manifest digest from the approved Tasks image workflow.
printf 'Built %s\n' "$image"
docker image inspect "$image" --format 'Local image ID: {{.Id}}; revision: {{index .Config.Labels "org.opencontainers.image.revision"}}'
