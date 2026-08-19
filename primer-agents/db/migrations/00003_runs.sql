-- +goose Up
-- +goose StatementBegin

CREATE TABLE agents.runs (
    id                  uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id          uuid        REFERENCES agents.sessions(id) ON DELETE RESTRICT,
    owner_namespace     text        NOT NULL CHECK (char_length(owner_namespace) BETWEEN 1 AND 256),
    idempotency_key     text        NOT NULL CHECK (char_length(idempotency_key) BETWEEN 1 AND 256),
    -- SHA-256 hex of (namespace || "|" || idempotency_key || "|" || profile || "|" || input_hash).
    -- Scoped per-namespace so different callers cannot collide or discover each other's keys.
    idempotency_hash    text        NOT NULL CHECK (char_length(idempotency_hash) = 64),
    profile             text        NOT NULL CHECK (char_length(profile) BETWEEN 1 AND 128),
    -- SHA-256 hex of the input content; used to detect materially-different create conflicts.
    input_hash          text        CHECK (char_length(input_hash) = 64),
    -- Bounded diagnostic preview; never stores raw prompt or credential material.
    input_preview       text        CHECK (char_length(input_preview) <= 2000),
    status              text        NOT NULL DEFAULT 'queued'
                                    CHECK (status IN (
                                        'queued','running','cancel_requested',
                                        'succeeded','failed','canceled','interrupted'
                                    )),
    -- Monotonic fencing version incremented on every status change.
    state_version       bigint      NOT NULL DEFAULT 1,
    -- Atomic per-run sequence counter: incremented before each event INSERT.
    next_event_seq      bigint      NOT NULL DEFAULT 1,
    cancel_requested_at timestamptz,
    cancel_reason_class text        CHECK (cancel_reason_class IS NULL OR char_length(cancel_reason_class) <= 64),
    attempt_count       int         NOT NULL DEFAULT 0,
    lease_expires_at    timestamptz,
    provider_started_at timestamptz,
    result_class        text        CHECK (result_class IS NULL OR char_length(result_class) <= 64),
    error_class         text        CHECK (error_class IS NULL OR char_length(error_class) <= 256),
    created_at          timestamptz NOT NULL DEFAULT now(),
    started_at          timestamptz,
    ended_at            timestamptz,

    UNIQUE (owner_namespace, idempotency_key)
);

-- Owner namespace + status for queue-claim and owner list queries.
CREATE INDEX runs_owner_status_idx    ON agents.runs (owner_namespace, status);
-- Session-scoped run listing.
CREATE INDEX runs_session_idx         ON agents.runs (session_id) WHERE session_id IS NOT NULL;
-- Cancellation observation: find runs awaiting cancel acknowledgment.
CREATE INDEX runs_cancel_request_idx  ON agents.runs (owner_namespace, cancel_requested_at)
    WHERE cancel_requested_at IS NOT NULL;
-- Expired-lease reclaim (Phase 4 worker).
CREATE INDEX runs_lease_expiry_idx    ON agents.runs (lease_expires_at)
    WHERE lease_expires_at IS NOT NULL AND status IN ('running');
-- Namespace + created_at for ordered owner listing.
CREATE INDEX runs_owner_created_idx   ON agents.runs (owner_namespace, created_at DESC);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS agents.runs;
-- +goose StatementEnd
