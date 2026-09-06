#!/bin/sh
# First-boot helper for the compose DEV postgres: LMS uses POSTGRES_DB=primer,
# TV needs a sibling database on the same instance.
set -eu

psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" <<'SQL'
SELECT 'CREATE DATABASE primer_tv'
WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'primer_tv')\gexec
SQL
