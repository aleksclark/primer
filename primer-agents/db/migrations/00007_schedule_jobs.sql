-- Phase 6: enhanced schedules with lease/next-due, job-type, and student
-- credential admission flag. The schedule_firings table already has the
-- UNIQUE (schedule_id, due_at) idempotency key from migration 00005.

-- +goose Up
-- +goose StatementBegin

ALTER TABLE agents.schedules
    -- Human-readable job type; server validates against allowed list.
    ADD COLUMN job_type      text        NOT NULL DEFAULT 'generic'
                                         CHECK (char_length(job_type) BETWEEN 1 AND 64),
    -- Bounded diagnostic input summary; no raw prompt/credential material.
    ADD COLUMN input_preview text        CHECK (input_preview IS NULL OR char_length(input_preview) <= 2000),
    -- IANA time zone name (e.g. America/Chicago). Used for cron evaluation.
    ADD COLUMN timezone      text        NOT NULL DEFAULT 'UTC'
                                         CHECK (char_length(timezone) BETWEEN 1 AND 64),
    -- Pre-computed next fire instant; NULL when disabled.
    -- Updated transactionally by the scheduler after each firing.
    ADD COLUMN next_due_at   timestamptz,
    -- Maximum number of missed firings to create on catch-up (bounded burst).
    ADD COLUMN max_catch_up  smallint    NOT NULL DEFAULT 1
                                         CHECK (max_catch_up BETWEEN 1 AND 10),
    -- Scheduler instance lease: prevents concurrent claim of the same schedule.
    ADD COLUMN lease_token      uuid,
    ADD COLUMN lease_expires_at timestamptz;

-- Index for scheduler polling: enabled schedules with a due next_due_at.
CREATE INDEX schedules_due_idx ON agents.schedules (next_due_at ASC)
    WHERE enabled = true AND next_due_at IS NOT NULL;

-- student_admissions maps a registered public client_id to the student scope.
-- An entry here is necessary (but not sufficient) for student route access.
-- The associated Identity credential must independently issue the correct scope.
-- Absence = fail closed; presence does NOT override missing JWT scope.
CREATE TABLE agents.student_admissions (
    client_id       text        PRIMARY KEY CHECK (char_length(client_id) BETWEEN 1 AND 128),
    owner_namespace_prefix text NOT NULL CHECK (char_length(owner_namespace_prefix) BETWEEN 1 AND 256),
    enabled         boolean     NOT NULL DEFAULT false,
    note            text,
    created_at      timestamptz NOT NULL DEFAULT now()
);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS agents.student_admissions;
DROP INDEX IF EXISTS agents.schedules_due_idx;
ALTER TABLE agents.schedules
    DROP COLUMN IF EXISTS lease_expires_at,
    DROP COLUMN IF EXISTS lease_token,
    DROP COLUMN IF EXISTS max_catch_up,
    DROP COLUMN IF EXISTS next_due_at,
    DROP COLUMN IF EXISTS timezone,
    DROP COLUMN IF EXISTS input_preview,
    DROP COLUMN IF EXISTS job_type;
-- +goose StatementEnd
