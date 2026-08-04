-- +goose Up
-- +goose StatementBegin

-- Parent/educator password credentials (bcrypt hash). Separate from educators so
-- the public educator row stays free of secrets in generic CRUD responses.
ALTER TABLE educators
    ADD COLUMN password_hash TEXT NOT NULL DEFAULT '';

CREATE TABLE parent_sessions (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    educator_id  UUID NOT NULL REFERENCES educators(id) ON DELETE CASCADE,
    token_hash   TEXT NOT NULL UNIQUE,
    expires_at   TIMESTAMPTZ NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_parent_sessions_educator ON parent_sessions(educator_id);
CREATE INDEX idx_parent_sessions_expires ON parent_sessions(expires_at);

-- Learning activities: authoring identity that survives revisions.
CREATE TABLE learning_activities (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    slug        TEXT NOT NULL UNIQUE,
    title       TEXT NOT NULL,
    summary     TEXT NOT NULL DEFAULT '',
    kind        TEXT NOT NULL,
    subject_id  UUID REFERENCES subjects(id) ON DELETE SET NULL,
    status      TEXT NOT NULL DEFAULT 'draft',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (kind IN ('terminal', 'typing')),
    CHECK (status IN ('draft', 'published', 'retired'))
);

CREATE INDEX idx_learning_activities_status ON learning_activities(status);
CREATE INDEX idx_learning_activities_subject ON learning_activities(subject_id);

-- Immutable published (or draft) revision payloads.
CREATE TABLE learning_activity_revisions (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    activity_id     UUID NOT NULL REFERENCES learning_activities(id) ON DELETE CASCADE,
    revision        INTEGER NOT NULL,
    schema_version  TEXT NOT NULL DEFAULT '1',
    content         JSONB NOT NULL DEFAULT '{}'::jsonb,
    content_sha256  TEXT NOT NULL,
    published_at    TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (activity_id, revision),
    UNIQUE (activity_id, content_sha256),
    CHECK (revision >= 1)
);

CREATE INDEX idx_activity_revisions_activity ON learning_activity_revisions(activity_id);
CREATE INDEX idx_activity_revisions_sha ON learning_activity_revisions(content_sha256);

-- Standards links are immutable revision data.
CREATE TABLE learning_activity_revision_standards (
    id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    activity_revision_id  UUID NOT NULL REFERENCES learning_activity_revisions(id) ON DELETE CASCADE,
    standard_id           UUID NOT NULL REFERENCES standards(id) ON DELETE CASCADE,
    role                  TEXT NOT NULL DEFAULT 'primary',
    weight                NUMERIC(6,3) NOT NULL DEFAULT 1.0,
    mastery_criterion     TEXT NOT NULL DEFAULT '',
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (activity_revision_id, standard_id),
    CHECK (role IN ('primary', 'reinforcement')),
    CHECK (weight >= 0)
);

CREATE INDEX idx_rev_standards_revision ON learning_activity_revision_standards(activity_revision_id);
CREATE INDEX idx_rev_standards_standard ON learning_activity_revision_standards(standard_id);

-- Concrete work item for a student.
CREATE TABLE student_assignments (
    id                     UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    student_id             UUID NOT NULL REFERENCES students(id) ON DELETE CASCADE,
    activity_revision_id   UUID NOT NULL REFERENCES learning_activity_revisions(id) ON DELETE RESTRICT,
    enrollment_id          UUID REFERENCES enrollments(id) ON DELETE SET NULL,
    state                  TEXT NOT NULL DEFAULT 'available',
    priority               INTEGER NOT NULL DEFAULT 0,
    available_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    due_at                 TIMESTAMPTZ,
    assigned_by            UUID REFERENCES educators(id) ON DELETE SET NULL,
    reason                 TEXT NOT NULL DEFAULT '',
    constraints            JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (state IN ('available', 'in_progress', 'completed', 'cancelled'))
);

CREATE INDEX idx_student_assignments_student ON student_assignments(student_id);
CREATE INDEX idx_student_assignments_revision ON student_assignments(activity_revision_id);
CREATE INDEX idx_student_assignments_state ON student_assignments(state);
CREATE INDEX idx_student_assignments_updated ON student_assignments(student_id, updated_at, id);

-- Student workstation devices (token stored as hash only).
CREATE TABLE student_devices (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    student_id   UUID NOT NULL REFERENCES students(id) ON DELETE CASCADE,
    name         TEXT NOT NULL DEFAULT '',
    token_hash   TEXT NOT NULL DEFAULT '',
    last_seen_at TIMESTAMPTZ,
    revoked_at   TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX idx_student_devices_token_hash
    ON student_devices(token_hash) WHERE token_hash <> '' AND revoked_at IS NULL;
CREATE INDEX idx_student_devices_student ON student_devices(student_id);

-- One-time pairing codes issued by a parent.
CREATE TABLE student_device_pairing_codes (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    student_id   UUID NOT NULL REFERENCES students(id) ON DELETE CASCADE,
    code_hash    TEXT NOT NULL,
    created_by   UUID REFERENCES educators(id) ON DELETE SET NULL,
    expires_at   TIMESTAMPTZ NOT NULL,
    used_at      TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_pairing_codes_hash ON student_device_pairing_codes(code_hash)
    WHERE used_at IS NULL;
CREATE INDEX idx_pairing_codes_student ON student_device_pairing_codes(student_id);

-- Learning sessions bound to an assignment and device.
CREATE TABLE learning_sessions (
    id                     UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    assignment_id          UUID NOT NULL REFERENCES student_assignments(id) ON DELETE CASCADE,
    student_id             UUID NOT NULL REFERENCES students(id) ON DELETE CASCADE,
    device_id              UUID NOT NULL REFERENCES student_devices(id) ON DELETE CASCADE,
    client_session_id      TEXT NOT NULL,
    activity_revision_id   UUID NOT NULL REFERENCES learning_activity_revisions(id) ON DELETE RESTRICT,
    state                  TEXT NOT NULL DEFAULT 'started',
    started_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_event_at          TIMESTAMPTZ,
    completed_at           TIMESTAMPTZ,
    duration_seconds       INTEGER NOT NULL DEFAULT 0,
    summary                TEXT NOT NULL DEFAULT '',
    created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (device_id, client_session_id),
    CHECK (state IN ('started', 'paused', 'completed', 'abandoned'))
);

CREATE INDEX idx_learning_sessions_assignment ON learning_sessions(assignment_id);
CREATE INDEX idx_learning_sessions_student ON learning_sessions(student_id);
CREATE INDEX idx_learning_sessions_state ON learning_sessions(state);
-- At most one non-terminal session per assignment.
CREATE UNIQUE INDEX idx_learning_sessions_open_assignment
    ON learning_sessions(assignment_id)
    WHERE state IN ('started', 'paused');

-- Append-only session event stream.
CREATE TABLE learning_session_events (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id      UUID NOT NULL REFERENCES learning_sessions(id) ON DELETE CASCADE,
    event_id        UUID NOT NULL,
    sequence        BIGINT NOT NULL,
    event_type      TEXT NOT NULL,
    client_time     TIMESTAMPTZ NOT NULL,
    received_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    schema_version  TEXT NOT NULL DEFAULT '1',
    payload         JSONB NOT NULL DEFAULT '{}'::jsonb,
    UNIQUE (event_id),
    UNIQUE (session_id, sequence),
    CHECK (sequence >= 0)
);

CREATE INDEX idx_session_events_session ON learning_session_events(session_id, sequence);

-- Immutable completion result per session (keyed by client completion UUID).
CREATE TABLE learning_session_completions (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id       UUID NOT NULL REFERENCES learning_sessions(id) ON DELETE CASCADE,
    completion_id    UUID NOT NULL,
    request_digest   TEXT NOT NULL,
    response         JSONB NOT NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (completion_id),
    UNIQUE (session_id)
);

-- Optional evidence artifact metadata (bytes live outside PostgreSQL later).
CREATE TABLE learning_session_artifacts (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id    UUID NOT NULL REFERENCES learning_sessions(id) ON DELETE CASCADE,
    artifact_id   UUID NOT NULL,
    filename      TEXT NOT NULL,
    media_type    TEXT NOT NULL DEFAULT 'application/octet-stream',
    byte_size     BIGINT NOT NULL DEFAULT 0,
    sha256        TEXT NOT NULL,
    storage_path  TEXT NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (artifact_id),
    UNIQUE (session_id, artifact_id),
    CHECK (byte_size >= 0)
);

CREATE INDEX idx_session_artifacts_session ON learning_session_artifacts(session_id);

-- Idempotent mastery evidence from student-client completions.
CREATE UNIQUE INDEX idx_mastery_evidence_source_ref
    ON mastery_evidence (mastery_record_id, source_ref)
    WHERE source_ref <> '';

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP INDEX IF EXISTS idx_mastery_evidence_source_ref;

DROP TABLE IF EXISTS learning_session_artifacts;
DROP TABLE IF EXISTS learning_session_completions;
DROP TABLE IF EXISTS learning_session_events;
DROP TABLE IF EXISTS learning_sessions;
DROP TABLE IF EXISTS student_device_pairing_codes;
DROP TABLE IF EXISTS student_devices;
DROP TABLE IF EXISTS student_assignments;
DROP TABLE IF EXISTS learning_activity_revision_standards;
DROP TABLE IF EXISTS learning_activity_revisions;
DROP TABLE IF EXISTS learning_activities;
DROP TABLE IF EXISTS parent_sessions;

ALTER TABLE educators DROP COLUMN IF EXISTS password_hash;

-- +goose StatementEnd
