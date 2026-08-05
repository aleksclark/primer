-- +goose Up
-- +goose StatementBegin

-- Phase 6: real artifact bytes, portfolio promotion, continuity bindings,
-- and durable curriculum import manifests.

ALTER TABLE learning_session_artifacts
    ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'metadata_only',
    ADD COLUMN IF NOT EXISTS student_id UUID REFERENCES students(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS activity_revision_id UUID REFERENCES learning_activity_revisions(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS bytes_stored BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS retention_until TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT now();

-- reserved | uploaded | rejected | metadata_only
ALTER TABLE learning_session_artifacts
    DROP CONSTRAINT IF EXISTS learning_session_artifacts_status_check;
ALTER TABLE learning_session_artifacts
    ADD CONSTRAINT learning_session_artifacts_status_check
    CHECK (status IN ('metadata_only', 'reserved', 'uploaded', 'rejected'));

CREATE INDEX IF NOT EXISTS idx_session_artifacts_student
    ON learning_session_artifacts(student_id);
CREATE INDEX IF NOT EXISTS idx_session_artifacts_sha
    ON learning_session_artifacts(sha256);

-- Parent-promoted portfolio items with provenance.
CREATE TABLE portfolio_items (
    id                     UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    student_id             UUID NOT NULL REFERENCES students(id) ON DELETE CASCADE,
    source_artifact_id     UUID NOT NULL,
    session_id             UUID NOT NULL REFERENCES learning_sessions(id) ON DELETE CASCADE,
    activity_revision_id   UUID NOT NULL REFERENCES learning_activity_revisions(id),
    title                  TEXT NOT NULL,
    media_type             TEXT NOT NULL DEFAULT 'application/octet-stream',
    byte_size              BIGINT NOT NULL DEFAULT 0,
    sha256                 TEXT NOT NULL,
    storage_path           TEXT NOT NULL DEFAULT '',
    promoted_by            UUID REFERENCES educators(id) ON DELETE SET NULL,
    status                 TEXT NOT NULL DEFAULT 'active',
    destination            TEXT NOT NULL DEFAULT 'portfolio',
    provenance             JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (byte_size >= 0),
    CHECK (status IN ('active', 'withdrawn')),
    CHECK (destination IN ('portfolio', 'fixture_bundle')),
    UNIQUE (student_id, source_artifact_id, destination)
);

CREATE INDEX idx_portfolio_items_student ON portfolio_items(student_id);
CREATE INDEX idx_portfolio_items_sha ON portfolio_items(sha256);

-- Immutable parent-approved fixture bundles for later activities.
CREATE TABLE approved_fixture_bundles (
    id                       UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    student_id               UUID NOT NULL REFERENCES students(id) ON DELETE CASCADE,
    source_portfolio_item_id UUID REFERENCES portfolio_items(id) ON DELETE SET NULL,
    source_artifact_id       UUID NOT NULL,
    digest                   TEXT NOT NULL,
    label                    TEXT NOT NULL DEFAULT '',
    entries                  JSONB NOT NULL DEFAULT '[]'::jsonb,
    storage_root             TEXT NOT NULL DEFAULT '',
    approved_by              UUID REFERENCES educators(id) ON DELETE SET NULL,
    status                   TEXT NOT NULL DEFAULT 'approved',
    provenance               JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at               TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (status IN ('approved', 'withdrawn')),
    UNIQUE (student_id, digest)
);

CREATE INDEX idx_approved_bundles_student ON approved_fixture_bundles(student_id);

-- Continuity decision binding an assignment to an approved bundle (or fresh).
CREATE TABLE assignment_continuity_bindings (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    assignment_id    UUID NOT NULL REFERENCES student_assignments(id) ON DELETE CASCADE,
    student_id       UUID NOT NULL REFERENCES students(id) ON DELETE CASCADE,
    continuity_mode  TEXT NOT NULL DEFAULT 'fresh',
    bundle_id        UUID REFERENCES approved_fixture_bundles(id) ON DELETE SET NULL,
    decided_by       UUID REFERENCES educators(id) ON DELETE SET NULL,
    notes            TEXT NOT NULL DEFAULT '',
    decided_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (continuity_mode IN ('fresh', 'optional_previous', 'required_project', 'portfolio_review')),
    UNIQUE (assignment_id)
);

CREATE INDEX idx_continuity_bindings_student ON assignment_continuity_bindings(student_id);

-- Durable curriculum import plan/apply audit + result manifests.
CREATE TABLE curriculum_import_runs (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    bundle_digest  TEXT NOT NULL,
    actor_id       UUID REFERENCES educators(id) ON DELETE SET NULL,
    source_label   TEXT NOT NULL DEFAULT '',
    mode           TEXT NOT NULL,
    status         TEXT NOT NULL,
    plan           JSONB NOT NULL DEFAULT '{}'::jsonb,
    result_manifest JSONB NOT NULL DEFAULT '{}'::jsonb,
    error_message  TEXT NOT NULL DEFAULT '',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    applied_at     TIMESTAMPTZ,
    CHECK (mode IN ('plan', 'apply')),
    CHECK (status IN ('planned', 'applied', 'failed', 'rejected'))
);

CREATE INDEX idx_curriculum_import_digest ON curriculum_import_runs(bundle_digest);
CREATE INDEX idx_curriculum_import_created ON curriculum_import_runs(created_at DESC);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP TABLE IF EXISTS curriculum_import_runs;
DROP TABLE IF EXISTS assignment_continuity_bindings;
DROP TABLE IF EXISTS approved_fixture_bundles;
DROP TABLE IF EXISTS portfolio_items;

DROP INDEX IF EXISTS idx_session_artifacts_sha;
DROP INDEX IF EXISTS idx_session_artifacts_student;

ALTER TABLE learning_session_artifacts
    DROP CONSTRAINT IF EXISTS learning_session_artifacts_status_check;

ALTER TABLE learning_session_artifacts
    DROP COLUMN IF EXISTS updated_at,
    DROP COLUMN IF EXISTS retention_until,
    DROP COLUMN IF EXISTS bytes_stored,
    DROP COLUMN IF EXISTS activity_revision_id,
    DROP COLUMN IF EXISTS student_id,
    DROP COLUMN IF EXISTS status;

-- +goose StatementEnd
