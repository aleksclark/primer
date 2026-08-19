-- Phase 5: session turn history and SSE notification.

-- +goose Up
-- +goose StatementBegin

-- session_turns records each durable multi-turn interaction within a session.
-- turn_sequence is 1-based and monotonically increasing within the session;
-- it matches the session revision at the time of creation.
CREATE TABLE agents.session_turns (
    id              uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id      uuid        NOT NULL REFERENCES agents.sessions(id) ON DELETE RESTRICT,
    turn_sequence   bigint      NOT NULL,
    -- run_id is set after the run row is created within the same transaction.
    run_id          uuid        REFERENCES agents.runs(id) ON DELETE RESTRICT,
    -- caller-supplied idempotency key scoped to the session.
    idempotency_key text        NOT NULL CHECK (char_length(idempotency_key) BETWEEN 1 AND 256),
    input_preview   text        CHECK (char_length(input_preview) <= 2000),
    status          text        NOT NULL DEFAULT 'pending'
                                CHECK (status IN ('pending','active','completed','failed','canceled')),
    created_at      timestamptz NOT NULL DEFAULT now(),

    -- At most one turn per sequence position.
    UNIQUE (session_id, turn_sequence),
    -- Idempotency: same key in a session always maps to the same turn.
    UNIQUE (session_id, idempotency_key)
);

CREATE INDEX session_turns_session_idx ON agents.session_turns (session_id, turn_sequence ASC);

-- sse_wakeup_channel is a per-run channel name used with pg_notify.
-- Format: agents_run_<run_id_hex_no_dashes>
-- Subscribers use LISTEN; the event writer calls pg_notify after each INSERT.
-- The notify payload is the sequence number (text).
-- This is an optimization only — subscribers fall back to polling.
CREATE OR REPLACE FUNCTION agents.notify_run_event()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    -- Channel name: agents_run_ + run_id without dashes (<=63 chars, PG limit).
    PERFORM pg_notify(
        'agents_run_' || replace(NEW.run_id::text, '-', ''),
        NEW.sequence::text
    );
    RETURN NEW;
END;
$$;

CREATE TRIGGER run_event_notify
    AFTER INSERT ON agents.run_events
    FOR EACH ROW EXECUTE FUNCTION agents.notify_run_event();

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TRIGGER IF EXISTS run_event_notify ON agents.run_events;
DROP FUNCTION IF EXISTS agents.notify_run_event();
DROP TABLE IF EXISTS agents.session_turns;
-- +goose StatementEnd
