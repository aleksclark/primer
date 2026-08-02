#!/usr/bin/env bash
# Deploy Primer LMS and/or the TV channel to the Nomad fleet.
#
# Images are built by the docker-image GitHub Actions workflow (which has
# packages:write) and pushed to ghcr.io. This script triggers that workflow
# for the current HEAD, waits for it, renders the Nomad job specs, and
# submits them. Set BUILD=local to build and push from this machine instead
# (requires a docker login to ghcr.io with write:packages).
#
# What gets deployed:
#   SERVICE=all   (default) both primer and primer-tv
#   SERVICE=lms   LMS only
#   SERVICE=tv    TV channel only
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

SERVICE="${SERVICE:-all}"
IMAGE_TAG="${IMAGE_TAG:-$(git rev-parse --short HEAD)}"
BUILD="${BUILD:-ci}"
LMS_IMAGE_REPO="${IMAGE_REPO:-ghcr.io/aleksclark/primer}"
TV_IMAGE_REPO="${TV_IMAGE_REPO:-ghcr.io/aleksclark/primer-tv}"
export NOMAD_ADDR NOMAD_TOKEN

need_lms=false
need_tv=false
case "$SERVICE" in
  all) need_lms=true; need_tv=true ;;
  lms) need_lms=true ;;
  tv)  need_tv=true ;;
  *)
    echo "error: SERVICE must be all, lms, or tv (got $SERVICE)" >&2
    exit 1
    ;;
esac

if $need_lms; then
  : "${DATABASE_URL:?set DATABASE_URL in deploy/.env or the environment}"
fi
if $need_tv; then
  : "${TV_DATABASE_URL:?set TV_DATABASE_URL in deploy/.env or the environment}"
  : "${TV_JELLYFIN_BASE_URL:?set TV_JELLYFIN_BASE_URL in deploy/.env or the environment}"
  : "${TV_JELLYFIN_API_KEY:?set TV_JELLYFIN_API_KEY in deploy/.env or the environment}"
  : "${TV_ADMIN_API_KEY:?set TV_ADMIN_API_KEY in deploy/.env or the environment}"
  # Optional — empty is valid (server warns and stays unconfigured).
  TV_JELLYFIN_USER_ID="${TV_JELLYFIN_USER_ID:-}"
  TV_PRIMER_BASE_URL="${TV_PRIMER_BASE_URL:-https://primer.fleet.clark.team}"
  TV_PRIMER_SERVICE_TOKEN="${TV_PRIMER_SERVICE_TOKEN:-${SERVICE_TOKEN:-}}"
fi
# Optional on the LMS side too.
SERVICE_TOKEN="${SERVICE_TOKEN:-}"

if [[ "$BUILD" == "local" ]]; then
  if $need_lms; then
    echo "==> Building ${LMS_IMAGE_REPO}:${IMAGE_TAG} locally"
    docker build -t "${LMS_IMAGE_REPO}:${IMAGE_TAG}" -t "${LMS_IMAGE_REPO}:latest" .
    docker push "${LMS_IMAGE_REPO}:${IMAGE_TAG}"
    docker push "${LMS_IMAGE_REPO}:latest"
  fi
  if $need_tv; then
    echo "==> Building ${TV_IMAGE_REPO}:${IMAGE_TAG} locally"
    docker build -f Dockerfile.tv -t "${TV_IMAGE_REPO}:${IMAGE_TAG}" -t "${TV_IMAGE_REPO}:latest" .
    docker push "${TV_IMAGE_REPO}:${IMAGE_TAG}"
    docker push "${TV_IMAGE_REPO}:latest"
  fi
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

TMPDIR_DEPLOY="$(mktemp -d /tmp/primer-deploy.XXXXXX)"
trap 'rm -rf "$TMPDIR_DEPLOY"' EXIT

if $need_lms; then
  echo "==> Rendering LMS job spec"
  JOB_FILE="$TMPDIR_DEPLOY/primer.nomad.hcl"
  IMAGE_TAG="$IMAGE_TAG" DATABASE_URL="$DATABASE_URL" SERVICE_TOKEN="$SERVICE_TOKEN" \
    envsubst '${IMAGE_TAG} ${DATABASE_URL} ${SERVICE_TOKEN}' \
    < deploy/primer.nomad.hcl.tmpl > "$JOB_FILE"
  echo "==> Submitting primer to ${NOMAD_ADDR}"
  nomad job run "$JOB_FILE"
  nomad job status primer | sed -n '/^Latest Deployment/,/^$/p' || true
  echo "==> LMS: https://primer.fleet.clark.team"
fi

if $need_tv; then
  echo "==> Rendering TV job spec"
  JOB_FILE="$TMPDIR_DEPLOY/primer-tv.nomad.hcl"
  IMAGE_TAG="$IMAGE_TAG" \
  TV_DATABASE_URL="$TV_DATABASE_URL" \
  TV_JELLYFIN_BASE_URL="$TV_JELLYFIN_BASE_URL" \
  TV_JELLYFIN_API_KEY="$TV_JELLYFIN_API_KEY" \
  TV_JELLYFIN_USER_ID="$TV_JELLYFIN_USER_ID" \
  TV_ADMIN_API_KEY="$TV_ADMIN_API_KEY" \
  TV_PRIMER_BASE_URL="$TV_PRIMER_BASE_URL" \
  TV_PRIMER_SERVICE_TOKEN="$TV_PRIMER_SERVICE_TOKEN" \
    envsubst '${IMAGE_TAG} ${TV_DATABASE_URL} ${TV_JELLYFIN_BASE_URL} ${TV_JELLYFIN_API_KEY} ${TV_JELLYFIN_USER_ID} ${TV_ADMIN_API_KEY} ${TV_PRIMER_BASE_URL} ${TV_PRIMER_SERVICE_TOKEN}' \
    < deploy/primer-tv.nomad.hcl.tmpl > "$JOB_FILE"

  echo "==> Submitting primer-tv to ${NOMAD_ADDR}"
  nomad job run "$JOB_FILE"
  nomad job status primer-tv | sed -n '/^Latest Deployment/,/^$/p' || true
  echo "==> TV:  https://tv.fleet.clark.team"
fi
