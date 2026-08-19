#!/usr/bin/env bash
# Disposable logical backup/restore drill. Never point this at production.
set -euo pipefail

: "${STUDIO_BACKUP_DSN:?set STUDIO_BACKUP_DSN to a disposable Studio PostgreSQL DSN}"
: "${STUDIO_RESTORE_DSN:?set STUDIO_RESTORE_DSN to an empty disposable restore DSN}"
command -v pg_dump >/dev/null || { echo "pg_dump is required" >&2; exit 2; }
command -v pg_restore >/dev/null || { echo "pg_restore is required" >&2; exit 2; }
command -v psql >/dev/null || { echo "psql is required" >&2; exit 2; }

backup="$(mktemp "${TMPDIR:-/tmp}/studio-backup.XXXXXX.dump")"
trap 'rm -f "$backup"' EXIT
pg_dump --format=custom --no-owner --no-privileges "$STUDIO_BACKUP_DSN" >"$backup"
pg_restore --clean --if-exists --no-owner --no-privileges --dbname="$STUDIO_RESTORE_DSN" "$backup"

version="$(psql "$STUDIO_RESTORE_DSN" -Atqc "SELECT max(version_id) FROM studio_goose_db_version WHERE is_applied")"
tables="$(psql "$STUDIO_RESTORE_DSN" -Atqc "SELECT count(*) FROM pg_tables WHERE schemaname='curriculum_studio'")"
[[ -n "$version" && "$version" -ge 1 ]] || { echo "restore has no Studio migration version" >&2; exit 1; }
[[ "$tables" -gt 0 ]] || { echo "restore has no Studio tables" >&2; exit 1; }
printf 'backup restore drill OK: version=%s studio_tables=%s\n' "$version" "$tables"
