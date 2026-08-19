-- +goose Up
-- +goose StatementBegin

CREATE TABLE agents.run_events (
    id             uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    run_id         uuid        NOT NULL REFERENCES agents.runs(id) ON DELETE RESTRICT,
    sequence       bigint      NOT NULL,
    schema_version smallint    NOT NULL DEFAULT 1,
    -- Lineage: root and parent run IDs for nested child attribution.
    root_run_id    uuid,
    parent_run_id  uuid,
    agent_id       text        CHECK (agent_id IS NULL OR char_length(agent_id) <= 256),
    agent_type     text        CHECK (agent_type IS NULL OR char_length(agent_type) <= 128),
    agent_depth    int,
    kind           text        NOT NULL CHECK (char_length(kind) BETWEEN 1 AND 64),
    -- Bounded JSON payload. Content sensitivity: raw response text is never stored
    -- in normal events; callers supply structured summaries only.
    payload        text        CHECK (payload IS NULL OR octet_length(payload) <= 65535),
    created_at     timestamptz NOT NULL DEFAULT now(),

    UNIQUE (run_id, sequence)
);

-- Cursor-based event replay: run_id + sequence ASC is the primary replay index.
CREATE INDEX run_events_replay_idx ON agents.run_events (run_id, sequence ASC);
-- Lineage lookup (Phase 4/5).
CREATE INDEX run_events_root_idx   ON agents.run_events (root_run_id) WHERE root_run_id IS NOT NULL;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS agents.run_events;
-- +goose StatementEnd
