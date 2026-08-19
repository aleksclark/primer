#!/usr/bin/env bash
# Vite DEV container entrypoint: npm ci into the named volume, then exec CMD as PID1.
set -euo pipefail

if [[ ! -f package.json ]]; then
  echo "primer-web-dev: package.json missing under ${PWD} (bind mount required)" >&2
  exit 1
fi

need_install=0
if [[ ! -d node_modules ]] || [[ -z "$(ls -A node_modules 2>/dev/null || true)" ]]; then
  need_install=1
elif [[ -f package-lock.json ]] && [[ ! -f node_modules/.package-lock.json ]]; then
  need_install=1
fi

if [[ "$need_install" -eq 1 ]]; then
  if [[ ! -f package-lock.json ]]; then
    echo "primer-web-dev: package-lock.json missing under ${PWD}" >&2
    echo "primer-web-dev: on the host run: npm install && git add package-lock.json" >&2
    exit 1
  fi
  echo "primer-web-dev: installing dependencies (npm ci)…"
  if ! npm ci --ignore-scripts --no-audit --no-fund; then
    echo "primer-web-dev: npm ci failed (fail-closed; no mutable npm install fallback)." >&2
    echo "primer-web-dev: lockfile/package.json are out of sync or network failed." >&2
    echo "primer-web-dev: on the host run: npm install && commit the updated package-lock.json." >&2
    exit 1
  fi
fi

exec "$@"
