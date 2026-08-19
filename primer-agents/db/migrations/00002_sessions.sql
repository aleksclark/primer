-- +goose Up
-- +goose StatementBegin

CREATE TABLE agents.sessions (
    id              uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_namespace text        NOT NULL CHECK (char_length(owner_namespace) BETWEEN 1 AND 256),
    caller_context  text        CHECK (char_length(caller_context) <= 4096),
    profile         text        NOT NULL CHECK (char_length(profile) BETWEEN 1 AND 128),
    status          text        NOT NULL DEFAULT 'open'
                                CHECK (status IN ('open', 'closed')),
    revision        bigint      NOT NULL DEFAULT 1,
    state_ref       text        CHECK (char_length(state_ref) <= 1024),
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    expires_at      timestamptz
);

CREATE INDEX sessions_owner_idx  ON agents.sessions (owner_namespace);
CREATE INDEX sessions_status_idx ON agents.sessions (owner_namespace, status);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS agents.sessions;
-- +goose StatementEnd
