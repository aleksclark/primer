-- +goose Up
-- +goose StatementBegin

-- Phase 4: immutable curriculum revisions + course activity membership.
-- Curricula remain the stable identity; revisions pin ordered activity refs,
-- prerequisites, gates, and continuity placeholders from CourseDocument.

ALTER TABLE curricula
    ADD COLUMN IF NOT EXISTS slug TEXT,
    ADD COLUMN IF NOT EXISTS subject_code TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'published';

UPDATE curricula
SET slug = 'curriculum-' || REPLACE(id::text, '-', '')
WHERE slug IS NULL OR slug = '';

ALTER TABLE curricula
    ALTER COLUMN slug SET NOT NULL;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'curricula_slug_key'
    ) THEN
        ALTER TABLE curricula ADD CONSTRAINT curricula_slug_key UNIQUE (slug);
    END IF;
END
$$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'curricula_status_check'
    ) THEN
        ALTER TABLE curricula
            ADD CONSTRAINT curricula_status_check
            CHECK (status IN ('draft', 'published', 'retired'));
    END IF;
END
$$;

CREATE INDEX IF NOT EXISTS idx_curricula_slug ON curricula(slug);
CREATE INDEX IF NOT EXISTS idx_curricula_status ON curricula(status);

CREATE TABLE curriculum_revisions (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    curriculum_id    UUID NOT NULL REFERENCES curricula(id) ON DELETE CASCADE,
    revision         INTEGER NOT NULL,
    title            TEXT NOT NULL,
    description      TEXT NOT NULL DEFAULT '',
    subject_code     TEXT NOT NULL DEFAULT '',
    version          TEXT NOT NULL DEFAULT '1',
    revision_policy  TEXT NOT NULL DEFAULT 'latest_published',
    document         JSONB NOT NULL DEFAULT '{}'::jsonb,
    published_at     TIMESTAMPTZ,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (curriculum_id, revision),
    CHECK (revision >= 1),
    CHECK (revision_policy IN ('latest_published', 'pinned_digest'))
);

CREATE INDEX idx_curriculum_revisions_curriculum ON curriculum_revisions(curriculum_id);
CREATE INDEX idx_curriculum_revisions_published ON curriculum_revisions(curriculum_id, published_at DESC);

-- Ordered activity membership within an immutable curriculum revision.
CREATE TABLE curriculum_activities (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    curriculum_revision_id  UUID NOT NULL REFERENCES curriculum_revisions(id) ON DELETE CASCADE,
    position                INTEGER NOT NULL,
    activity_slug           TEXT NOT NULL,
    activity_revision_id    UUID REFERENCES learning_activity_revisions(id) ON DELETE RESTRICT,
    module                  TEXT NOT NULL DEFAULT '',
    capstone                BOOLEAN NOT NULL DEFAULT FALSE,
    continuity_mode         TEXT NOT NULL DEFAULT '',
    metadata                JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (curriculum_revision_id, position),
    UNIQUE (curriculum_revision_id, activity_slug),
    CHECK (position >= 1)
);

CREATE INDEX idx_curriculum_activities_revision ON curriculum_activities(curriculum_revision_id, position);
CREATE INDEX idx_curriculum_activities_slug ON curriculum_activities(activity_slug);
CREATE INDEX idx_curriculum_activities_rev_id ON curriculum_activities(activity_revision_id);

-- Directed prerequisite edges between membership slugs.
CREATE TABLE curriculum_activity_prerequisites (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    curriculum_revision_id  UUID NOT NULL REFERENCES curriculum_revisions(id) ON DELETE CASCADE,
    activity_slug           TEXT NOT NULL,
    requires_slug           TEXT NOT NULL,
    requirement             TEXT NOT NULL DEFAULT 'completed',
    description             TEXT NOT NULL DEFAULT '',
    created_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (curriculum_revision_id, activity_slug, requires_slug),
    CHECK (activity_slug <> requires_slug),
    CHECK (requirement IN ('completed', 'approaching', 'mastered'))
);

CREATE INDEX idx_curr_act_prereq_revision ON curriculum_activity_prerequisites(curriculum_revision_id);
CREATE INDEX idx_curr_act_prereq_activity ON curriculum_activity_prerequisites(curriculum_revision_id, activity_slug);

-- Evidence / parent-review gates attached to membership slugs.
CREATE TABLE curriculum_activity_gates (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    curriculum_revision_id  UUID NOT NULL REFERENCES curriculum_revisions(id) ON DELETE CASCADE,
    activity_slug           TEXT NOT NULL,
    kind                    TEXT NOT NULL,
    standards               TEXT[] NOT NULL DEFAULT '{}',
    description             TEXT NOT NULL DEFAULT '',
    created_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (kind IN ('evidence', 'parent_review'))
);

CREATE INDEX idx_curr_act_gates_revision ON curriculum_activity_gates(curriculum_revision_id);
CREATE INDEX idx_curr_act_gates_activity ON curriculum_activity_gates(curriculum_revision_id, activity_slug);

-- Remediation / reinforcement branch placeholders from CourseDocument.
CREATE TABLE curriculum_activity_remediation (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    curriculum_revision_id  UUID NOT NULL REFERENCES curriculum_revisions(id) ON DELETE CASCADE,
    for_activity_slug       TEXT NOT NULL,
    branch_slug             TEXT NOT NULL,
    kind                    TEXT NOT NULL DEFAULT 'remediation',
    description             TEXT NOT NULL DEFAULT '',
    created_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (kind IN ('remediation', 'reinforcement'))
);

CREATE INDEX idx_curr_act_remediation_revision ON curriculum_activity_remediation(curriculum_revision_id);
CREATE INDEX idx_curr_act_remediation_for ON curriculum_activity_remediation(curriculum_revision_id, for_activity_slug);

-- Enrollment points at an explicit curriculum revision + parent controls.
ALTER TABLE enrollments
    ADD COLUMN IF NOT EXISTS curriculum_revision_id UUID REFERENCES curriculum_revisions(id) ON DELETE RESTRICT,
    ADD COLUMN IF NOT EXISTS priority INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS pinned_activity_slug TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS pinned_reason TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS pinned_by UUID REFERENCES educators(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS pinned_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS override_slugs TEXT[] NOT NULL DEFAULT '{}',
    ADD COLUMN IF NOT EXISTS override_reason TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS blocking_reasons JSONB NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN IF NOT EXISTS constraints JSONB NOT NULL DEFAULT '{}'::jsonb;

CREATE INDEX IF NOT EXISTS idx_enrollments_revision ON enrollments(curriculum_revision_id);
CREATE INDEX IF NOT EXISTS idx_enrollments_status_student ON enrollments(student_id, status);

-- Auditable parent pin/override events on enrollments.
CREATE TABLE enrollment_audit_events (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    enrollment_id   UUID NOT NULL REFERENCES enrollments(id) ON DELETE CASCADE,
    educator_id     UUID REFERENCES educators(id) ON DELETE SET NULL,
    action          TEXT NOT NULL,
    reason          TEXT NOT NULL DEFAULT '',
    detail          JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (action IN (
        'enroll', 'pause', 'resume', 'complete', 'withdraw',
        'pin', 'unpin', 'override_prereq', 'clear_override', 'migrate_revision'
    ))
);

CREATE INDEX idx_enrollment_audit_enrollment ON enrollment_audit_events(enrollment_id, created_at DESC);

-- Deterministic migration: every existing curriculum gets revision 1, and
-- every pre-existing enrollment is pointed at that revision.
INSERT INTO curriculum_revisions (
    curriculum_id, revision, title, description, subject_code, version,
    revision_policy, document, published_at, created_at
)
SELECT
    c.id,
    1,
    c.name,
    c.description,
    COALESCE(NULLIF(c.subject_code, ''), ''),
    '1',
    'latest_published',
    jsonb_build_object(
        'migrated', true,
        'source', 'pre-phase4-curriculum',
        'curriculumId', c.id,
        'name', c.name
    ),
    c.created_at,
    c.created_at
FROM curricula c
WHERE NOT EXISTS (
    SELECT 1 FROM curriculum_revisions r WHERE r.curriculum_id = c.id
);

UPDATE enrollments e
SET curriculum_revision_id = r.id
FROM curriculum_revisions r
WHERE e.curriculum_revision_id IS NULL
  AND r.curriculum_id = e.curriculum_id
  AND r.revision = 1;

-- Assignments may retain membership provenance for course-selected work.
ALTER TABLE student_assignments
    ADD COLUMN IF NOT EXISTS curriculum_activity_id UUID REFERENCES curriculum_activities(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS selection_reason TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_student_assignments_membership
    ON student_assignments(curriculum_activity_id);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

ALTER TABLE student_assignments
    DROP COLUMN IF EXISTS selection_reason,
    DROP COLUMN IF EXISTS curriculum_activity_id;

DROP TABLE IF EXISTS enrollment_audit_events;

ALTER TABLE enrollments
    DROP COLUMN IF EXISTS constraints,
    DROP COLUMN IF EXISTS blocking_reasons,
    DROP COLUMN IF EXISTS override_reason,
    DROP COLUMN IF EXISTS override_slugs,
    DROP COLUMN IF EXISTS pinned_at,
    DROP COLUMN IF EXISTS pinned_by,
    DROP COLUMN IF EXISTS pinned_reason,
    DROP COLUMN IF EXISTS pinned_activity_slug,
    DROP COLUMN IF EXISTS priority,
    DROP COLUMN IF EXISTS curriculum_revision_id;

DROP TABLE IF EXISTS curriculum_activity_remediation;
DROP TABLE IF EXISTS curriculum_activity_gates;
DROP TABLE IF EXISTS curriculum_activity_prerequisites;
DROP TABLE IF EXISTS curriculum_activities;
DROP TABLE IF EXISTS curriculum_revisions;

ALTER TABLE curricula
    DROP CONSTRAINT IF EXISTS curricula_status_check,
    DROP CONSTRAINT IF EXISTS curricula_slug_key,
    DROP COLUMN IF EXISTS status,
    DROP COLUMN IF EXISTS subject_code,
    DROP COLUMN IF EXISTS slug;

-- +goose StatementEnd
