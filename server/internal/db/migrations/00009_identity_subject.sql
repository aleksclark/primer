-- +goose Up
-- +goose StatementBegin

-- Phase 13 (IB8): link local educators to their Primer Identity principal.
-- The column is nullable (not every educator has an identity account yet) and
-- unique (one identity account maps to exactly one local educator).
ALTER TABLE educators
    ADD COLUMN IF NOT EXISTS identity_subject TEXT UNIQUE;

COMMENT ON COLUMN educators.identity_subject IS
    'UUID subject from a verified Primer Identity access token. Maps an Identity account to a local educator.';

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

ALTER TABLE educators
    DROP COLUMN IF EXISTS identity_subject;

-- +goose StatementEnd
