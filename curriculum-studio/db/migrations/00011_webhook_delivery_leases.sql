-- +goose Up
-- +goose StatementBegin

ALTER TABLE curriculum_studio.webhook_deliveries
    ADD COLUMN lease_owner TEXT NOT NULL DEFAULT '',
    ADD COLUMN lease_expires_at TIMESTAMPTZ,
    ADD COLUMN attempt_started_at TIMESTAMPTZ;

CREATE INDEX idx_studio_webhook_deliveries_claim
    ON curriculum_studio.webhook_deliveries(status, lease_expires_at, created_at);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS curriculum_studio.idx_studio_webhook_deliveries_claim;
ALTER TABLE curriculum_studio.webhook_deliveries
    DROP COLUMN IF EXISTS attempt_started_at,
    DROP COLUMN IF EXISTS lease_expires_at,
    DROP COLUMN IF EXISTS lease_owner;
-- +goose StatementEnd
