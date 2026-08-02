-- +goose Up
-- +goose StatementBegin

-- Instruction logs: instructional time earned outside the LMS proper. The
-- first producer is the TV server, which pushes each finished viewing of an
-- educational or mixed programme here so the hours land under the right
-- subject. Entertainment viewing is not instructional time and is rejected
-- outright rather than stored and filtered later.
CREATE TABLE instruction_logs (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source          TEXT NOT NULL DEFAULT 'tv',          -- tv | manual
    source_ref      TEXT NOT NULL DEFAULT '',            -- the producer's stable ID for the event
    student_id      UUID REFERENCES students(id) ON DELETE SET NULL,
    media_title     TEXT NOT NULL,
    class           TEXT NOT NULL DEFAULT 'educational', -- educational | mixed
    subject_tags    TEXT[] NOT NULL DEFAULT '{}',
    standard_codes  TEXT[] NOT NULL DEFAULT '{}',
    watched_seconds INTEGER NOT NULL DEFAULT 0,
    occurred_on     DATE NOT NULL DEFAULT CURRENT_DATE,
    notes           TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (class IN ('educational', 'mixed')),
    CHECK (watched_seconds >= 0)
);

-- Idempotency for machine producers: a retry carrying the same reference
-- matches the existing row instead of counting the hours a second time. The
-- index is partial so hand-entered rows, which have no reference, are free to
-- repeat.
CREATE UNIQUE INDEX idx_instruction_logs_source_ref
    ON instruction_logs(source, source_ref) WHERE source_ref <> '';

CREATE INDEX idx_instruction_logs_occurred_on ON instruction_logs(occurred_on);
CREATE INDEX idx_instruction_logs_student ON instruction_logs(student_id);
CREATE INDEX idx_instruction_logs_subject_tags ON instruction_logs USING gin (subject_tags);
CREATE INDEX idx_instruction_logs_standard_codes ON instruction_logs USING gin (standard_codes);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS instruction_logs;
-- +goose StatementEnd
