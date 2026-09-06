-- +goose Up
-- +goose StatementBegin
-- A workflow stage may be reclaimed after a process dies between persistence
-- and completion. Its generated item identity is therefore durable provenance,
-- not an in-memory title map.
CREATE UNIQUE INDEX uq_studio_mat_items_stage_identity
    ON curriculum_studio.materialized_items (run_id, (provenance->>'stage_id'), kind, title)
    WHERE provenance ? 'stage_id';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS curriculum_studio.uq_studio_mat_items_stage_identity;
-- +goose StatementEnd
