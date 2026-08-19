-- +goose Up
-- +goose StatementBegin
CREATE SCHEMA IF NOT EXISTS agents;

-- service_info is a key/value ledger for service-level metadata.
-- Phase 1 writes schema_version; later phases may add worker/provider state.
CREATE TABLE agents.service_info (
    key   text PRIMARY KEY,
    value text NOT NULL
);

INSERT INTO agents.service_info (key, value)
VALUES ('schema_version', '1');
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS agents.service_info;
DROP SCHEMA IF EXISTS agents;
-- +goose StatementEnd
