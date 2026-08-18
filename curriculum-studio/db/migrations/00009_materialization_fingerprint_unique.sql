-- +goose Up
-- +goose StatementBegin

-- Idempotent materialization requests are one logical run per revision and
-- input fingerprint. The repository still compares snapshots before replaying
-- an existing row; this index closes the concurrent insert race.
CREATE UNIQUE INDEX idx_studio_mat_runs_fingerprint_unique
    ON curriculum_studio.materialization_runs(plan_revision_id, input_fingerprint);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS curriculum_studio.idx_studio_mat_runs_fingerprint_unique;
-- +goose StatementEnd
