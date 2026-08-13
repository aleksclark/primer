#!/usr/bin/env bash
# DEPRECATED as a production Nomad writer (plan-03 / I30).
#
# The primer release set is declared under deploy/nomad/ and is enrolled for
# fleet pull-reconciler ownership. This script no longer renders secrets via
# envsubst or runs `nomad job run`.
#
# Allowed local helpers:
#   ./deploy/deploy.sh contract   # static contract tests
#   ./deploy/deploy.sh plan-hint  # print reconciler-oriented plan guidance
#
# Production submit path: fleet pull reconciler (CAS) after reviewed source SHA.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT}"

cmd="${1:-}"

refuse_production() {
  cat >&2 <<'EOF'
ERROR: deploy/deploy.sh refuses production Nomad submit.

The authoritative deploy contract is deploy/nomad/ (plan-03):
  - deployment.yaml
  - jobs/{primer,primer-tv,content-ingest}.nomad.hcl
  - env/home.nomadvars.hcl
  - images.lock.hcl

Production writer: fleet pull reconciler (serial release set, explicit-only prune).
Do not envsubst secrets. Do not `nomad job run` from this repo script.
Do not dispatch content-ingest from CI.

Local checks:
  ./deploy/deploy.sh contract
  ./deploy/nomad/tests/contract.sh

Legacy templates under deploy/*.nomad.hcl.tmpl remain only for dual-source
rollback until S3; they are not the production submit path.
EOF
  exit 2
}

case "${cmd}" in
  contract|test)
    exec bash "${ROOT}/deploy/nomad/tests/contract.sh"
    ;;
  plan-hint|plan)
    cat <<'EOF'
Read-only plan guidance (no cluster write):

  1. Ensure Nomad Variables exist (key names only; see deploy/nomad/README.md).
  2. From a machine with Nomad ACL + fleet tooling:
       nomad job plan \
         -var-file=deploy/nomad/images.lock.hcl \
         -var-file=deploy/nomad/env/home.nomadvars.hcl \
         deploy/nomad/jobs/primer.nomad.hcl
     (repeat for primer-tv, content-ingest — serial)
  3. content-ingest: pause periodic / prove no child BEFORE S2 handoff.
  4. Apply only via fleet pull reconciler CAS for the release-set SHA.
  5. Never dispatch content-ingest from CI or this script.

This script does not run plan or apply.
EOF
    ;;
  -h|--help|help)
    cat <<'EOF'
Usage: ./deploy/deploy.sh <command>

Commands:
  contract   Run deploy/nomad/tests/contract.sh
  plan-hint  Print read-only reconciler plan guidance
  help       Show this help

Any legacy deploy target (lms|tv|ingest|all|...) is refused.
EOF
    ;;
  ""|lms|tv|ingest|all|primer|primer-tv|content-ingest|deploy|run|push|build)
    refuse_production
    ;;
  *)
    refuse_production
    ;;
esac
