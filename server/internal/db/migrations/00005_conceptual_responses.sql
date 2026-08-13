-- +goose Up
-- +goose StatementBegin

-- Conceptual response submissions and parent review decisions (Phase 3).
-- Student text is stored as evidence-bearing content, not chat transcript.

CREATE TABLE student_responses (
    id                     UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    submission_id          UUID NOT NULL,
    student_id             UUID NOT NULL REFERENCES students(id) ON DELETE CASCADE,
    session_id             UUID NOT NULL REFERENCES learning_sessions(id) ON DELETE CASCADE,
    assignment_id          UUID NOT NULL REFERENCES student_assignments(id) ON DELETE CASCADE,
    activity_revision_id   UUID NOT NULL REFERENCES learning_activity_revisions(id) ON DELETE RESTRICT,
    task_id                TEXT NOT NULL,
    body                   TEXT NOT NULL,
    body_sha256            TEXT NOT NULL,
    status                 TEXT NOT NULL DEFAULT 'submitted',
    request_digest         TEXT NOT NULL DEFAULT '',
    attempt                INTEGER NOT NULL DEFAULT 1,
    rubric_snapshot        JSONB NOT NULL DEFAULT '[]'::jsonb,
    parent_review_required BOOLEAN NOT NULL DEFAULT FALSE,
    submitted_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    reviewed_at            TIMESTAMPTZ,
    reviewed_by            UUID REFERENCES educators(id) ON DELETE SET NULL,
    return_reason          TEXT NOT NULL DEFAULT '',
    created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (submission_id),
    CHECK (status IN ('submitted', 'accepted', 'returned')),
    CHECK (attempt >= 1),
    CHECK (char_length(body) <= 8000)
);

CREATE INDEX idx_student_responses_student ON student_responses(student_id, status, submitted_at DESC);
CREATE INDEX idx_student_responses_session ON student_responses(session_id);
CREATE INDEX idx_student_responses_status ON student_responses(status) WHERE status = 'submitted';
CREATE INDEX idx_student_responses_assignment_task ON student_responses(assignment_id, task_id, attempt DESC);

CREATE TABLE student_response_reviews (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    response_id     UUID NOT NULL REFERENCES student_responses(id) ON DELETE CASCADE,
    educator_id     UUID NOT NULL REFERENCES educators(id) ON DELETE CASCADE,
    decision        TEXT NOT NULL,
    reason          TEXT NOT NULL DEFAULT '',
    criteria        JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (decision IN ('accept', 'return'))
);

CREATE INDEX idx_response_reviews_response ON student_response_reviews(response_id, created_at DESC);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP TABLE IF EXISTS student_response_reviews;
DROP TABLE IF EXISTS student_responses;

-- +goose StatementEnd
