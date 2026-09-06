-- +goose Up
-- +goose StatementBegin

-- Collaborative authoring (S17): comments, approvals, read-only shares,
-- reusable unit library, plan templates, and workspace policy knobs.
-- Additive only; plan-graph immutability triggers are unchanged.

CREATE TABLE curriculum_studio.plan_comments (
    id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id         UUID NOT NULL REFERENCES curriculum_studio.workspaces(id) ON DELETE CASCADE,
    plan_revision_id     UUID NOT NULL REFERENCES curriculum_studio.plan_revisions(id) ON DELETE CASCADE,
    node_id              TEXT NOT NULL,
    author_subject_ref   TEXT NOT NULL,
    author_display_name  TEXT NOT NULL DEFAULT '',
    body                 TEXT NOT NULL,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (node_id <> ''),
    CHECK (author_subject_ref <> ''),
    CHECK (body <> '')
);

CREATE INDEX idx_studio_plan_comments_revision
    ON curriculum_studio.plan_comments(plan_revision_id, created_at);
CREATE INDEX idx_studio_plan_comments_workspace
    ON curriculum_studio.plan_comments(workspace_id, created_at);

CREATE TABLE curriculum_studio.plan_approvals (
    id                     UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id           UUID NOT NULL REFERENCES curriculum_studio.workspaces(id) ON DELETE CASCADE,
    plan_revision_id       UUID NOT NULL REFERENCES curriculum_studio.plan_revisions(id) ON DELETE CASCADE,
    reviewer_subject_ref   TEXT NOT NULL,
    reviewer_display_name  TEXT NOT NULL DEFAULT '',
    status                 TEXT NOT NULL DEFAULT 'approved',
    content_fingerprint    TEXT NOT NULL,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (plan_revision_id),
    CHECK (reviewer_subject_ref <> ''),
    CHECK (status IN ('approved', 'rejected'))
);

CREATE INDEX idx_studio_plan_approvals_workspace
    ON curriculum_studio.plan_approvals(workspace_id, created_at);

CREATE TABLE curriculum_studio.curriculum_shares (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    curriculum_id           UUID NOT NULL REFERENCES curriculum_studio.curricula(id) ON DELETE CASCADE,
    source_workspace_id     UUID NOT NULL REFERENCES curriculum_studio.workspaces(id) ON DELETE CASCADE,
    target_workspace_id     UUID NOT NULL REFERENCES curriculum_studio.workspaces(id) ON DELETE CASCADE,
    permission              TEXT NOT NULL DEFAULT 'read',
    created_by_subject_ref  TEXT NOT NULL DEFAULT '',
    created_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (curriculum_id, target_workspace_id),
    CHECK (permission = 'read'),
    CHECK (source_workspace_id <> target_workspace_id)
);

CREATE INDEX idx_studio_curriculum_shares_target
    ON curriculum_studio.curriculum_shares(target_workspace_id);

CREATE TABLE curriculum_studio.unit_library_entries (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES curriculum_studio.workspaces(id) ON DELETE CASCADE,
    name         TEXT NOT NULL,
    blueprint    JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (name <> ''),
    CHECK (jsonb_typeof(blueprint) = 'object')
);

CREATE INDEX idx_studio_unit_library_workspace
    ON curriculum_studio.unit_library_entries(workspace_id, name);

CREATE TABLE curriculum_studio.plan_templates (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID REFERENCES curriculum_studio.workspaces(id) ON DELETE CASCADE,
    code         TEXT NOT NULL,
    name         TEXT NOT NULL,
    brief_type   TEXT NOT NULL,
    seed         JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (code <> ''),
    CHECK (name <> ''),
    CHECK (brief_type IN (
        'homeschool_year',
        'single_subject',
        'classroom_semester',
        'project_based_unit',
        'standards_remediation',
        'custom'
    )),
    CHECK (jsonb_typeof(seed) = 'object')
);

CREATE UNIQUE INDEX uq_studio_plan_templates_global_code
    ON curriculum_studio.plan_templates (code)
    WHERE workspace_id IS NULL;
CREATE UNIQUE INDEX uq_studio_plan_templates_workspace_code
    ON curriculum_studio.plan_templates (workspace_id, code)
    WHERE workspace_id IS NOT NULL;

INSERT INTO curriculum_studio.plan_templates (code, name, brief_type, seed) VALUES
    (
        'homeschool_year',
        'Homeschool year',
        'homeschool_year',
        '{"outcomes":[{"code":"mastery","title":"Year-long mastery"}],"units":[{"title":"Homeschool core unit","outcomeCodes":["mastery"]}]}'::jsonb
    ),
    (
        'single_subject',
        'Single-subject course',
        'single_subject',
        '{"outcomes":[{"code":"mastery","title":"Course mastery"}],"units":[{"title":"Subject unit 1","outcomeCodes":["mastery"]}]}'::jsonb
    ),
    (
        'classroom_semester',
        'Classroom semester',
        'classroom_semester',
        '{"outcomes":[{"code":"mastery","title":"Semester outcomes"}],"units":[{"title":"Semester unit 1","outcomeCodes":["mastery"]}]}'::jsonb
    ),
    (
        'project_based_unit',
        'Project-based unit',
        'project_based_unit',
        '{"outcomes":[{"code":"mastery","title":"Project outcome"}],"units":[{"title":"Project unit","outcomeCodes":["mastery"]}]}'::jsonb
    ),
    (
        'standards_remediation',
        'Standards remediation plan',
        'standards_remediation',
        '{"outcomes":[{"code":"mastery","title":"Remediation target"}],"units":[{"title":"Remediation unit","outcomeCodes":["mastery"]}]}'::jsonb
    );

-- One statement snapshot of all authorable plan content. A decision only
-- applies to exactly this snapshot, never to later edits of the same draft.
CREATE FUNCTION curriculum_studio.plan_content_fingerprint(requested_revision_id UUID)
RETURNS TEXT LANGUAGE sql STABLE AS $$
    SELECT md5(jsonb_build_array(
        (SELECT jsonb_build_array(r.title, r.brief) FROM curriculum_studio.plan_revisions r WHERE r.id=requested_revision_id),
        (SELECT jsonb_agg(to_jsonb(t) ORDER BY t.id) FROM curriculum_studio.objectives t WHERE t.plan_revision_id=requested_revision_id),
        (SELECT jsonb_agg(to_jsonb(t) ORDER BY t.id) FROM curriculum_studio.outcomes t WHERE t.plan_revision_id=requested_revision_id),
        (SELECT jsonb_agg(to_jsonb(t) ORDER BY t.id) FROM curriculum_studio.outcome_standard_mappings t JOIN curriculum_studio.outcomes o ON o.id=t.outcome_id WHERE o.plan_revision_id=requested_revision_id),
        (SELECT jsonb_agg(to_jsonb(t) ORDER BY t.id) FROM curriculum_studio.outcome_prerequisites t WHERE t.plan_revision_id=requested_revision_id),
        (SELECT jsonb_agg(to_jsonb(t) ORDER BY t.id) FROM curriculum_studio.learning_arcs t WHERE t.plan_revision_id=requested_revision_id),
        (SELECT jsonb_agg(to_jsonb(t) ORDER BY t.id) FROM curriculum_studio.units t WHERE t.plan_revision_id=requested_revision_id),
        (SELECT jsonb_agg(to_jsonb(t) ORDER BY t.id) FROM curriculum_studio.projects t WHERE t.plan_revision_id=requested_revision_id),
        (SELECT jsonb_agg(to_jsonb(t) ORDER BY t.unit_id,t.outcome_id) FROM curriculum_studio.unit_outcomes t JOIN curriculum_studio.units u ON u.id=t.unit_id WHERE u.plan_revision_id=requested_revision_id),
        (SELECT jsonb_agg(to_jsonb(t) ORDER BY t.project_id,t.outcome_id) FROM curriculum_studio.project_outcomes t JOIN curriculum_studio.projects p ON p.id=t.project_id WHERE p.plan_revision_id=requested_revision_id),
        (SELECT jsonb_agg(to_jsonb(t) ORDER BY t.id) FROM curriculum_studio.evidence_requirements t WHERE t.plan_revision_id=requested_revision_id),
        (SELECT jsonb_agg(to_jsonb(t) ORDER BY t.id) FROM curriculum_studio.scheduling_constraints t WHERE t.plan_revision_id=requested_revision_id),
        (SELECT jsonb_agg(to_jsonb(t) ORDER BY t.id) FROM curriculum_studio.plan_resources t WHERE t.plan_revision_id=requested_revision_id)
    )::text)
$$;

CREATE TABLE curriculum_studio.workspace_policies (
    workspace_id UUID PRIMARY KEY REFERENCES curriculum_studio.workspaces(id) ON DELETE CASCADE,
    policies     JSONB NOT NULL DEFAULT '{}'::jsonb,
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (jsonb_typeof(policies) = 'object')
);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP FUNCTION IF EXISTS curriculum_studio.plan_content_fingerprint(UUID);
DROP TABLE IF EXISTS curriculum_studio.workspace_policies;
DROP TABLE IF EXISTS curriculum_studio.plan_templates;
DROP TABLE IF EXISTS curriculum_studio.unit_library_entries;
DROP TABLE IF EXISTS curriculum_studio.curriculum_shares;
DROP TABLE IF EXISTS curriculum_studio.plan_approvals;
DROP TABLE IF EXISTS curriculum_studio.plan_comments;

-- +goose StatementEnd
