-- Phase 6 reserved: schedule and schedule-firing tables.
-- These tables define the uniqueness key (schedule_id, due_at) required by
-- the lifecycle plan before Phase 6 workers consume them. No Phase 2 code
-- reads or writes these tables; they exist only so Phase 6 does not require
-- a schema migration that touches the runs schema.

-- +goose Up
-- +goose StatementBegin

CREATE TABLE agents.schedules (
    id              uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_namespace text        NOT NULL CHECK (char_length(owner_namespace) BETWEEN 1 AND 256),
    profile         text        NOT NULL CHECK (char_length(profile) BETWEEN 1 AND 128),
    cron_expr       text        NOT NULL CHECK (char_length(cron_expr) BETWEEN 1 AND 256),
    input_hash      text        CHECK (input_hash IS NULL OR char_length(input_hash) = 64),
    enabled         boolean     NOT NULL DEFAULT true,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX schedules_owner_idx ON agents.schedules (owner_namespace);

CREATE TABLE agents.schedule_firings (
    id          uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    schedule_id uuid        NOT NULL REFERENCES agents.schedules(id) ON DELETE RESTRICT,
    due_at      timestamptz NOT NULL,
    run_id      uuid        REFERENCES agents.runs(id) ON DELETE RESTRICT,
    status      text        NOT NULL DEFAULT 'pending'
                            CHECK (status IN ('pending','fired','skipped','failed')),
    created_at  timestamptz NOT NULL DEFAULT now(),

    UNIQUE (schedule_id, due_at)
);

CREATE INDEX schedule_firings_pending_idx ON agents.schedule_firings (due_at)
    WHERE status = 'pending';

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS agents.schedule_firings;
DROP TABLE IF EXISTS agents.schedules;
-- +goose StatementEnd
