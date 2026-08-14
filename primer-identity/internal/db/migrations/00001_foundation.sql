-- +goose Up
-- +goose StatementBegin

-- Identity foundation only. Account/OAuth tables land in later waves.
-- pgcrypto is optional; gen_random_uuid() is built into PostgreSQL 13+.
DO $$
BEGIN
    CREATE EXTENSION IF NOT EXISTS "pgcrypto";
EXCEPTION WHEN OTHERS THEN
    RAISE NOTICE 'pgcrypto unavailable, skipping';
END
$$;

CREATE TABLE IF NOT EXISTS schema_meta (
    key         TEXT PRIMARY KEY,
    value       TEXT NOT NULL,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO schema_meta (key, value)
VALUES ('service', 'primer-identity')
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now();

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS schema_meta;
-- +goose StatementEnd
