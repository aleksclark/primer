-- +goose Up
-- +goose StatementBegin

-- Durable curriculum identity. Revisions pin the educational plan.
CREATE TABLE curriculum_studio.curricula (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id    UUID NOT NULL REFERENCES curriculum_studio.workspaces(id) ON DELETE CASCADE,
    slug            TEXT NOT NULL,
    title           TEXT NOT NULL,
    description     TEXT NOT NULL DEFAULT '',
    approach        TEXT NOT NULL DEFAULT 'custom',
    grade_band      TEXT NOT NULL DEFAULT '',
    status          TEXT NOT NULL DEFAULT 'draft',
    current_draft_revision_id UUID,
    published_revision_id     UUID,
    metadata        JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (workspace_id, slug),
    CHECK (slug <> ''),
    CHECK (title <> ''),
    CHECK (approach IN ('mastery_based', 'spiral', 'classical', 'unit_study', 'project_based', 'custom')),
    CHECK (status IN ('draft', 'active', 'retired')),
    CHECK (jsonb_typeof(metadata) = 'object')
);

CREATE INDEX idx_studio_curricula_workspace ON curriculum_studio.curricula(workspace_id);
CREATE INDEX idx_studio_curricula_status ON curriculum_studio.curricula(status);

-- Immutable once published. Drafts remain editable.
CREATE TABLE curriculum_studio.plan_revisions (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    curriculum_id   UUID NOT NULL REFERENCES curriculum_studio.curricula(id) ON DELETE CASCADE,
    revision        INTEGER NOT NULL,
    title           TEXT NOT NULL,
    brief           JSONB NOT NULL DEFAULT '{}'::jsonb,
    status          TEXT NOT NULL DEFAULT 'draft',
    published_at    TIMESTAMPTZ,
    published_by_subject_ref TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (curriculum_id, revision),
    CHECK (revision >= 1),
    CHECK (title <> ''),
    CHECK (status IN ('draft', 'published', 'superseded')),
    CHECK (
        (status = 'draft' AND published_at IS NULL)
        OR (status IN ('published', 'superseded') AND published_at IS NOT NULL)
    ),
    CHECK (jsonb_typeof(brief) = 'object')
);

CREATE INDEX idx_studio_plan_revisions_curriculum ON curriculum_studio.plan_revisions(curriculum_id);
CREATE INDEX idx_studio_plan_revisions_status ON curriculum_studio.plan_revisions(curriculum_id, status);

ALTER TABLE curriculum_studio.curricula
    ADD CONSTRAINT curricula_current_draft_fk
        FOREIGN KEY (current_draft_revision_id)
        REFERENCES curriculum_studio.plan_revisions(id)
        ON DELETE SET NULL,
    ADD CONSTRAINT curricula_published_revision_fk
        FOREIGN KEY (published_revision_id)
        REFERENCES curriculum_studio.plan_revisions(id)
        ON DELETE SET NULL;

CREATE TABLE curriculum_studio.objectives (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    plan_revision_id    UUID NOT NULL REFERENCES curriculum_studio.plan_revisions(id) ON DELETE CASCADE,
    code                TEXT NOT NULL,
    title               TEXT NOT NULL,
    description         TEXT NOT NULL DEFAULT '',
    position            INTEGER NOT NULL DEFAULT 0,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (plan_revision_id, code),
    CHECK (code <> ''),
    CHECK (title <> '')
);

CREATE INDEX idx_studio_objectives_revision ON curriculum_studio.objectives(plan_revision_id);

CREATE TABLE curriculum_studio.outcomes (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    plan_revision_id    UUID NOT NULL REFERENCES curriculum_studio.plan_revisions(id) ON DELETE CASCADE,
    objective_id        UUID REFERENCES curriculum_studio.objectives(id) ON DELETE SET NULL,
    code                TEXT NOT NULL,
    title               TEXT NOT NULL,
    description         TEXT NOT NULL DEFAULT '',
    mastery_criteria    TEXT NOT NULL DEFAULT '',
    position            INTEGER NOT NULL DEFAULT 0,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (plan_revision_id, code),
    CHECK (code <> ''),
    CHECK (title <> '')
);

CREATE INDEX idx_studio_outcomes_revision ON curriculum_studio.outcomes(plan_revision_id);
CREATE INDEX idx_studio_outcomes_objective ON curriculum_studio.outcomes(objective_id);

CREATE TABLE curriculum_studio.outcome_standard_mappings (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    outcome_id      UUID NOT NULL REFERENCES curriculum_studio.outcomes(id) ON DELETE CASCADE,
    standard_id     UUID NOT NULL REFERENCES curriculum_studio.catalog_standards(id) ON DELETE RESTRICT,
    alignment       TEXT NOT NULL DEFAULT 'addresses',
    notes           TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (outcome_id, standard_id),
    CHECK (alignment IN ('addresses', 'assesses', 'introduces', 'reinforces'))
);

CREATE INDEX idx_studio_outcome_std_standard ON curriculum_studio.outcome_standard_mappings(standard_id);

-- Prerequisite DAG among outcomes in the same revision. Cycles rejected by trigger.
CREATE TABLE curriculum_studio.outcome_prerequisites (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    plan_revision_id    UUID NOT NULL REFERENCES curriculum_studio.plan_revisions(id) ON DELETE CASCADE,
    outcome_id          UUID NOT NULL REFERENCES curriculum_studio.outcomes(id) ON DELETE CASCADE,
    prerequisite_id     UUID NOT NULL REFERENCES curriculum_studio.outcomes(id) ON DELETE CASCADE,
    requirement         TEXT NOT NULL DEFAULT 'completed',
    UNIQUE (plan_revision_id, outcome_id, prerequisite_id),
    CHECK (outcome_id <> prerequisite_id),
    CHECK (requirement IN ('introduced', 'completed', 'mastered'))
);

CREATE INDEX idx_studio_outcome_prereq_revision ON curriculum_studio.outcome_prerequisites(plan_revision_id);
CREATE INDEX idx_studio_outcome_prereq_prereq ON curriculum_studio.outcome_prerequisites(prerequisite_id);

CREATE TABLE curriculum_studio.learning_arcs (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    plan_revision_id    UUID NOT NULL REFERENCES curriculum_studio.plan_revisions(id) ON DELETE CASCADE,
    code                TEXT NOT NULL,
    title               TEXT NOT NULL,
    description         TEXT NOT NULL DEFAULT '',
    position            INTEGER NOT NULL DEFAULT 0,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (plan_revision_id, code),
    CHECK (code <> ''),
    CHECK (title <> '')
);

CREATE INDEX idx_studio_arcs_revision ON curriculum_studio.learning_arcs(plan_revision_id);

CREATE TABLE curriculum_studio.units (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    plan_revision_id    UUID NOT NULL REFERENCES curriculum_studio.plan_revisions(id) ON DELETE CASCADE,
    learning_arc_id     UUID REFERENCES curriculum_studio.learning_arcs(id) ON DELETE SET NULL,
    code                TEXT NOT NULL,
    title               TEXT NOT NULL,
    essential_questions TEXT[] NOT NULL DEFAULT '{}',
    estimated_minutes   INTEGER,
    position            INTEGER NOT NULL DEFAULT 0,
    blueprint           JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (plan_revision_id, code),
    CHECK (code <> ''),
    CHECK (title <> ''),
    CHECK (estimated_minutes IS NULL OR estimated_minutes > 0),
    CHECK (jsonb_typeof(blueprint) = 'object')
);

CREATE INDEX idx_studio_units_revision ON curriculum_studio.units(plan_revision_id);
CREATE INDEX idx_studio_units_arc ON curriculum_studio.units(learning_arc_id);

CREATE TABLE curriculum_studio.projects (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    plan_revision_id    UUID NOT NULL REFERENCES curriculum_studio.plan_revisions(id) ON DELETE CASCADE,
    unit_id             UUID REFERENCES curriculum_studio.units(id) ON DELETE SET NULL,
    code                TEXT NOT NULL,
    title               TEXT NOT NULL,
    description         TEXT NOT NULL DEFAULT '',
    phases              JSONB NOT NULL DEFAULT '[]'::jsonb,
    estimated_minutes   INTEGER,
    position            INTEGER NOT NULL DEFAULT 0,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (plan_revision_id, code),
    CHECK (code <> ''),
    CHECK (title <> ''),
    CHECK (estimated_minutes IS NULL OR estimated_minutes > 0),
    CHECK (jsonb_typeof(phases) = 'array')
);

CREATE INDEX idx_studio_projects_revision ON curriculum_studio.projects(plan_revision_id);
CREATE INDEX idx_studio_projects_unit ON curriculum_studio.projects(unit_id);

CREATE TABLE curriculum_studio.unit_outcomes (
    unit_id     UUID NOT NULL REFERENCES curriculum_studio.units(id) ON DELETE CASCADE,
    outcome_id  UUID NOT NULL REFERENCES curriculum_studio.outcomes(id) ON DELETE CASCADE,
    role        TEXT NOT NULL DEFAULT 'target',
    PRIMARY KEY (unit_id, outcome_id),
    CHECK (role IN ('target', 'prior', 'stretch'))
);

CREATE TABLE curriculum_studio.project_outcomes (
    project_id  UUID NOT NULL REFERENCES curriculum_studio.projects(id) ON DELETE CASCADE,
    outcome_id  UUID NOT NULL REFERENCES curriculum_studio.outcomes(id) ON DELETE CASCADE,
    role        TEXT NOT NULL DEFAULT 'target',
    PRIMARY KEY (project_id, outcome_id),
    CHECK (role IN ('target', 'prior', 'stretch'))
);

CREATE TABLE curriculum_studio.evidence_requirements (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    plan_revision_id    UUID NOT NULL REFERENCES curriculum_studio.plan_revisions(id) ON DELETE CASCADE,
    outcome_id          UUID NOT NULL REFERENCES curriculum_studio.outcomes(id) ON DELETE CASCADE,
    kind                TEXT NOT NULL,
    description         TEXT NOT NULL DEFAULT '',
    criteria            JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (outcome_id, kind),
    CHECK (kind IN ('continuous', 'formal', 'project', 'portfolio', 'discussion', 'performance')),
    CHECK (jsonb_typeof(criteria) = 'object')
);

CREATE INDEX idx_studio_evidence_revision ON curriculum_studio.evidence_requirements(plan_revision_id);
CREATE INDEX idx_studio_evidence_outcome ON curriculum_studio.evidence_requirements(outcome_id);

CREATE TABLE curriculum_studio.scheduling_constraints (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    plan_revision_id    UUID NOT NULL REFERENCES curriculum_studio.plan_revisions(id) ON DELETE CASCADE,
    kind                TEXT NOT NULL,
    payload             JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (kind IN (
        'available_minutes', 'calendar_window', 'blackout',
        'testing_window', 'philosophy', 'non_negotiable', 'workload_cap'
    )),
    CHECK (jsonb_typeof(payload) = 'object')
);

CREATE INDEX idx_studio_sched_constraints_revision ON curriculum_studio.scheduling_constraints(plan_revision_id);

CREATE TABLE curriculum_studio.plan_resources (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    plan_revision_id    UUID NOT NULL REFERENCES curriculum_studio.plan_revisions(id) ON DELETE CASCADE,
    resource_id         UUID NOT NULL REFERENCES curriculum_studio.resources(id) ON DELETE RESTRICT,
    unit_id             UUID REFERENCES curriculum_studio.units(id) ON DELETE CASCADE,
    project_id          UUID REFERENCES curriculum_studio.projects(id) ON DELETE CASCADE,
    role                TEXT NOT NULL DEFAULT 'required',
    notes               TEXT NOT NULL DEFAULT '',
    UNIQUE (plan_revision_id, resource_id, unit_id, project_id),
    CHECK (role IN ('required', 'optional', 'teacher', 'extension'))
);

CREATE INDEX idx_studio_plan_resources_revision ON curriculum_studio.plan_resources(plan_revision_id);
CREATE INDEX idx_studio_plan_resources_resource ON curriculum_studio.plan_resources(resource_id);

CREATE TABLE curriculum_studio.validation_reports (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    plan_revision_id    UUID NOT NULL REFERENCES curriculum_studio.plan_revisions(id) ON DELETE CASCADE,
    status              TEXT NOT NULL DEFAULT 'pending',
    summary             JSONB NOT NULL DEFAULT '{}'::jsonb,
    generated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (status IN ('pending', 'passed', 'failed', 'warning')),
    CHECK (jsonb_typeof(summary) = 'object')
);

CREATE INDEX idx_studio_validation_reports_revision ON curriculum_studio.validation_reports(plan_revision_id, generated_at DESC);

CREATE TABLE curriculum_studio.validation_findings (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    report_id       UUID NOT NULL REFERENCES curriculum_studio.validation_reports(id) ON DELETE CASCADE,
    severity        TEXT NOT NULL,
    code            TEXT NOT NULL,
    message         TEXT NOT NULL,
    node_kind       TEXT NOT NULL DEFAULT '',
    node_id         UUID,
    details         JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (severity IN ('error', 'warning', 'info')),
    CHECK (code <> ''),
    CHECK (message <> ''),
    CHECK (jsonb_typeof(details) = 'object')
);

CREATE INDEX idx_studio_validation_findings_report ON curriculum_studio.validation_findings(report_id);
CREATE INDEX idx_studio_validation_findings_severity ON curriculum_studio.validation_findings(severity);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS curriculum_studio.validation_findings;
DROP TABLE IF EXISTS curriculum_studio.validation_reports;
DROP TABLE IF EXISTS curriculum_studio.plan_resources;
DROP TABLE IF EXISTS curriculum_studio.scheduling_constraints;
DROP TABLE IF EXISTS curriculum_studio.evidence_requirements;
DROP TABLE IF EXISTS curriculum_studio.project_outcomes;
DROP TABLE IF EXISTS curriculum_studio.unit_outcomes;
DROP TABLE IF EXISTS curriculum_studio.projects;
DROP TABLE IF EXISTS curriculum_studio.units;
DROP TABLE IF EXISTS curriculum_studio.learning_arcs;
DROP TABLE IF EXISTS curriculum_studio.outcome_prerequisites;
DROP TABLE IF EXISTS curriculum_studio.outcome_standard_mappings;
DROP TABLE IF EXISTS curriculum_studio.outcomes;
DROP TABLE IF EXISTS curriculum_studio.objectives;
ALTER TABLE curriculum_studio.curricula
    DROP CONSTRAINT IF EXISTS curricula_published_revision_fk,
    DROP CONSTRAINT IF EXISTS curricula_current_draft_fk;
DROP TABLE IF EXISTS curriculum_studio.plan_revisions;
DROP TABLE IF EXISTS curriculum_studio.curricula;
-- +goose StatementEnd
