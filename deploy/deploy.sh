#!/usr/bin/env bash
# Deploy the Primer LMS to the Nomad fleet at primer.fleet.clark.team.
#
# The image is built by the docker-image GitHub Actions workflow (which has
# packages:write) and pushed to ghcr.io. This script triggers that workflow
# for the current HEAD, waits for it, renders the Nomad job spec, and submits
# it. Set BUILD=local to build and push from this machine instead (requires
# a docker login to ghcr.io with write:packages).
#
# Configuration comes from deploy/.env (see deploy/.env.example).
set -euo pipefail

cd "$(dirname "$0")/.."

ENV_FILE="deploy/.env"
if [[ -f "$ENV_FILE" ]]; then
  # shellcheck disable=SC1090
  source "$ENV_FILE"
fi

: "${NOMAD_ADDR:?set NOMAD_ADDR in deploy/.env or the environment}"
: "${NOMAD_TOKEN:?set NOMAD_TOKEN in deploy/.env or the environment}"
: "${DATABASE_URL:?set DATABASE_URL in deploy/.env or the environment}"
IMAGE_REPO="${IMAGE_REPO:-ghcr.io/aleksclark/primer}"
IMAGE_TAG="${IMAGE_TAG:-$(git rev-parse --short HEAD)}"
BUILD="${BUILD:-ci}"
export NOMAD_ADDR NOMAD_TOKEN

if [[ "$BUILD" == "local" ]]; then
  echo "==> Building ${IMAGE_REPO}:${IMAGE_TAG} locally"
  docker build -t "${IMAGE_REPO}:${IMAGE_TAG}" .
  docker push "${IMAGE_REPO}:${IMAGE_TAG}"
else
  if [[ -n "$(git status --porcelain)" ]]; then
    echo "error: working tree is dirty; commit and push before a CI deploy (or use BUILD=local)" >&2
    exit 1
  fi
  if ! git merge-base --is-ancestor HEAD "origin/$(git rev-parse --abbrev-ref HEAD)" 2>/dev/null; then
    echo "error: HEAD is not pushed to origin; push first so CI can build it" >&2
    exit 1
  fi

  echo "==> Triggering docker-image workflow for $(git rev-parse --abbrev-ref HEAD) @ ${IMAGE_TAG}"
  gh workflow run docker-image.yml --ref "$(git rev-parse --abbrev-ref HEAD)"
  sleep 5
  RUN_ID="$(gh run list --workflow=docker-image.yml --limit 1 --json databaseId --jq '.[0].databaseId')"
  echo "==> Waiting for workflow run ${RUN_ID}"
  gh run watch "$RUN_ID" --exit-status
fi

echo "==> Rendering job spec"
JOB_FILE="$(mktemp /tmp/primer.nomad.XXXXXX.hcl)"
trap 'rm -f "$JOB_FILE"' EXIT
IMAGE_TAG="$IMAGE_TAG" DATABASE_URL="$DATABASE_URL" \
  envsubst '${IMAGE_TAG} ${DATABASE_URL}' \
  < deploy/primer.nomad.hcl.tmpl > "$JOB_FILE"

echo "==> Submitting job to ${NOMAD_ADDR}"
nomad job run "$JOB_FILE"

echo "==> Deployment status"
nomad job status primer | sed -n '/^Latest Deployment/,/^$/p'

echo "==> Deployed: https://primer.fleet.clark.team"
