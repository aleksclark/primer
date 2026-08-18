-- +goose Up
-- +goose StatementBegin

-- DB fencing state is part of the stage row. A reclaimed stage gets a new
-- fence token; stale workers cannot update it with an old owner/token pair.
ALTER TABLE curriculum_studio.workflow_stages
    ADD COLUMN lease_owner TEXT NOT NULL DEFAULT '',
    ADD COLUMN lease_expires_at TIMESTAMPTZ,
    ADD COLUMN fence_token BIGINT NOT NULL DEFAULT 0,
    ADD CONSTRAINT workflow_stages_fence_token_nonnegative CHECK (fence_token >= 0);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE curriculum_studio.workflow_stages
    DROP CONSTRAINT IF EXISTS workflow_stages_fence_token_nonnegative,
    DROP COLUMN IF EXISTS fence_token,
    DROP COLUMN IF EXISTS lease_expires_at,
    DROP COLUMN IF EXISTS lease_owner;
-- +goose StatementEnd
