-- +goose Up
-- +goose StatementBegin

CREATE EXTENSION IF NOT EXISTS "pgcrypto";
CREATE EXTENSION IF NOT EXISTS "pg_trgm";

-- Educators: the parents / administrators who manage the system.
CREATE TABLE educators (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email       TEXT NOT NULL UNIQUE,
    name        TEXT NOT NULL,
    role        TEXT NOT NULL DEFAULT 'parent',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Students: the learners. The system supports many.
CREATE TABLE students (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    first_name   TEXT NOT NULL,
    last_name    TEXT NOT NULL,
    birthdate    DATE,
    grade_level  INTEGER,
    notes        TEXT NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Subjects: math, ela, science, social_studies, practical, ...
CREATE TABLE subjects (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code         TEXT NOT NULL UNIQUE,
    name         TEXT NOT NULL,
    description  TEXT NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Standards: individual learning standards, hierarchical & multi-source.
CREATE TABLE standards (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source       TEXT NOT NULL DEFAULT 'custom',      -- tennessee | common_core | custom
    subject_id   UUID REFERENCES subjects(id) ON DELETE SET NULL,
    parent_id    UUID REFERENCES standards(id) ON DELETE SET NULL,
    code         TEXT NOT NULL,                        -- e.g. TN.MATH.6.RP.A.3
    grade_level  INTEGER,
    domain       TEXT NOT NULL DEFAULT '',
    cluster      TEXT NOT NULL DEFAULT '',
    description  TEXT NOT NULL DEFAULT '',
    tcap_weight  TEXT NOT NULL DEFAULT '',             -- low | medium | high
    metadata     JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (source, code)
);

CREATE INDEX idx_standards_subject ON standards(subject_id);
CREATE INDEX idx_standards_parent ON standards(parent_id);
CREATE INDEX idx_standards_desc_trgm ON standards USING gin (description gin_trgm_ops);

-- Prerequisite graph between standards.
CREATE TABLE standard_prerequisites (
    standard_id       UUID NOT NULL REFERENCES standards(id) ON DELETE CASCADE,
    prerequisite_id   UUID NOT NULL REFERENCES standards(id) ON DELETE CASCADE,
    PRIMARY KEY (standard_id, prerequisite_id),
    CHECK (standard_id <> prerequisite_id)
);

-- Curricula: a curriculum approach (mastery, spiral, classical, unit study, ...).
CREATE TABLE curricula (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name         TEXT NOT NULL,
    description  TEXT NOT NULL DEFAULT '',
    approach     TEXT NOT NULL DEFAULT 'custom',       -- mastery_based | spiral | classical | unit_study | custom
    grade_level  INTEGER,
    metadata     JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Ordering / grouping of standards within a curriculum approach.
CREATE TABLE curriculum_standards (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    curriculum_id  UUID NOT NULL REFERENCES curricula(id) ON DELETE CASCADE,
    standard_id    UUID NOT NULL REFERENCES standards(id) ON DELETE CASCADE,
    unit           TEXT NOT NULL DEFAULT '',
    position       INTEGER NOT NULL DEFAULT 0,
    notes          TEXT NOT NULL DEFAULT '',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (curriculum_id, standard_id)
);

CREATE INDEX idx_curriculum_standards_curriculum ON curriculum_standards(curriculum_id);
CREATE INDEX idx_curriculum_standards_standard ON curriculum_standards(standard_id);

-- Enrollment of a student into a curriculum.
CREATE TABLE enrollments (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    student_id     UUID NOT NULL REFERENCES students(id) ON DELETE CASCADE,
    curriculum_id  UUID NOT NULL REFERENCES curricula(id) ON DELETE CASCADE,
    status         TEXT NOT NULL DEFAULT 'active',      -- active | paused | completed | withdrawn
    started_on     DATE NOT NULL DEFAULT CURRENT_DATE,
    ended_on       DATE,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (student_id, curriculum_id)
);

CREATE INDEX idx_enrollments_student ON enrollments(student_id);
CREATE INDEX idx_enrollments_curriculum ON enrollments(curriculum_id);

-- Mastery state per (student, standard).
CREATE TABLE mastery_records (
    id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    student_id           UUID NOT NULL REFERENCES students(id) ON DELETE CASCADE,
    standard_id          UUID NOT NULL REFERENCES standards(id) ON DELETE CASCADE,
    status               TEXT NOT NULL DEFAULT 'not_introduced', -- not_introduced | in_progress | approaching | mastered
    confidence           NUMERIC(4,3) NOT NULL DEFAULT 0.0,
    decay_rate           NUMERIC(4,3) NOT NULL DEFAULT 0.02,
    last_assessed_at     TIMESTAMPTZ,
    last_reinforced_at   TIMESTAMPTZ,
    next_reinforcement_at TIMESTAMPTZ,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (student_id, standard_id),
    CHECK (confidence >= 0.0 AND confidence <= 1.0)
);

CREATE INDEX idx_mastery_student ON mastery_records(student_id);
CREATE INDEX idx_mastery_standard ON mastery_records(standard_id);
CREATE INDEX idx_mastery_status ON mastery_records(status);

-- Evidence backing a mastery record.
CREATE TABLE mastery_evidence (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    mastery_record_id  UUID NOT NULL REFERENCES mastery_records(id) ON DELETE CASCADE,
    kind               TEXT NOT NULL DEFAULT 'continuous', -- continuous | formal | project | portfolio
    occurred_on        DATE NOT NULL DEFAULT CURRENT_DATE,
    context            TEXT NOT NULL DEFAULT '',
    source_ref         TEXT NOT NULL DEFAULT '',
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_mastery_evidence_record ON mastery_evidence(mastery_record_id);

-- Assessments: a variety of assessment types.
CREATE TABLE assessments (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    title          TEXT NOT NULL,
    description    TEXT NOT NULL DEFAULT '',
    kind           TEXT NOT NULL DEFAULT 'quiz',   -- continuous | quick_check | comprehensive | tcap_practice | quiz | project_rubric
    subject_id     UUID REFERENCES subjects(id) ON DELETE SET NULL,
    curriculum_id  UUID REFERENCES curricula(id) ON DELETE SET NULL,
    grade_level    INTEGER,
    metadata       JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_assessments_subject ON assessments(subject_id);
CREATE INDEX idx_assessments_kind ON assessments(kind);

-- Items within an assessment.
CREATE TABLE assessment_items (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    assessment_id UUID NOT NULL REFERENCES assessments(id) ON DELETE CASCADE,
    standard_id   UUID REFERENCES standards(id) ON DELETE SET NULL,
    position      INTEGER NOT NULL DEFAULT 0,
    item_type     TEXT NOT NULL DEFAULT 'mc',     -- mc | multi_select | equation_editor | constructed_response | matching | short_answer | true_false
    difficulty    TEXT NOT NULL DEFAULT 'on_track', -- approaching | on_track | mastered
    stem          TEXT NOT NULL DEFAULT '',
    rationale     TEXT NOT NULL DEFAULT '',
    points        NUMERIC(6,2) NOT NULL DEFAULT 1.0,
    metadata      JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_assessment_items_assessment ON assessment_items(assessment_id);
CREATE INDEX idx_assessment_items_standard ON assessment_items(standard_id);

-- Options for choice-type items.
CREATE TABLE assessment_item_options (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    item_id    UUID NOT NULL REFERENCES assessment_items(id) ON DELETE CASCADE,
    position   INTEGER NOT NULL DEFAULT 0,
    text       TEXT NOT NULL DEFAULT '',
    correct    BOOLEAN NOT NULL DEFAULT false,
    feedback   TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_assessment_item_options_item ON assessment_item_options(item_id);

-- A student's attempt at an assessment.
CREATE TABLE assessment_attempts (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    assessment_id UUID NOT NULL REFERENCES assessments(id) ON DELETE CASCADE,
    student_id    UUID NOT NULL REFERENCES students(id) ON DELETE CASCADE,
    status        TEXT NOT NULL DEFAULT 'in_progress', -- in_progress | submitted | scored
    score         NUMERIC(8,2),
    max_score     NUMERIC(8,2),
    started_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    submitted_at  TIMESTAMPTZ,
    scored_at     TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_attempts_assessment ON assessment_attempts(assessment_id);
CREATE INDEX idx_attempts_student ON assessment_attempts(student_id);

-- Individual responses within an attempt.
CREATE TABLE item_responses (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    attempt_id     UUID NOT NULL REFERENCES assessment_attempts(id) ON DELETE CASCADE,
    item_id        UUID NOT NULL REFERENCES assessment_items(id) ON DELETE CASCADE,
    response       JSONB NOT NULL DEFAULT '{}'::jsonb,
    is_correct     BOOLEAN,
    points_awarded NUMERIC(6,2),
    feedback       TEXT NOT NULL DEFAULT '',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (attempt_id, item_id)
);

CREATE INDEX idx_item_responses_attempt ON item_responses(attempt_id);
CREATE INDEX idx_item_responses_item ON item_responses(item_id);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS item_responses;
DROP TABLE IF EXISTS assessment_attempts;
DROP TABLE IF EXISTS assessment_item_options;
DROP TABLE IF EXISTS assessment_items;
DROP TABLE IF EXISTS assessments;
DROP TABLE IF EXISTS mastery_evidence;
DROP TABLE IF EXISTS mastery_records;
DROP TABLE IF EXISTS enrollments;
DROP TABLE IF EXISTS curriculum_standards;
DROP TABLE IF EXISTS curricula;
DROP TABLE IF EXISTS standard_prerequisites;
DROP TABLE IF EXISTS standards;
DROP TABLE IF EXISTS subjects;
DROP TABLE IF EXISTS students;
DROP TABLE IF EXISTS educators;
-- +goose StatementEnd
