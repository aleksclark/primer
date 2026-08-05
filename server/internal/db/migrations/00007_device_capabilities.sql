-- +goose Up
-- +goose StatementBegin

-- Phase 5: store recent device capability reports for eligibility diagnostics.
-- Not an authorization boundary; parents use this for deployment diagnostics.
ALTER TABLE student_devices
    ADD COLUMN IF NOT EXISTS capabilities JSONB NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN IF NOT EXISTS capabilities_reported_at TIMESTAMPTZ;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

ALTER TABLE student_devices
    DROP COLUMN IF EXISTS capabilities_reported_at,
    DROP COLUMN IF EXISTS capabilities;

-- +goose StatementEnd
