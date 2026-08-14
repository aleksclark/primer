-- +goose Up
-- +goose StatementBegin

-- Generic learner / class profiles. Primer learner IDs appear only as
-- opaque refs inside snapshot JSON or integration_identities.
CREATE TABLE curriculum_studio.learner_profiles (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id    UUID NOT NULL REFERENCES curriculum_studio.workspaces(id) ON DELETE CASCADE,
    kind            TEXT NOT NULL DEFAULT 'learner',
    label           TEXT NOT NULL,
    grade_band      TEXT NOT NULL DEFAULT '',
    profile         JSONB NOT NULL DEFAULT '{}'::jsonb,
    integration_identity_id UUID REFERENCES curriculum_studio.integration_identities(id) ON DELETE SET NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (kind IN ('learner', 'class')),
    CHECK (label <> ''),
    CHECK (jsonb_typeof(profile) = 'object')
);

CREATE INDEX idx_studio_learner_profiles_workspace ON curriculum_studio.learner_profiles(workspace_id);
CREATE INDEX idx_studio_learner_profiles_kind ON curriculum_studio.learner_profiles(kind);

-- A production run against a plan revision. input_snapshot is the complete
-- frozen input; input_fingerprint is a durable hash of that snapshot.
CREATE TABLE curriculum_studio.materialization_runs (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id        UUID NOT NULL REFERENCES curriculum_studio.workspaces(id) ON DELETE CASCADE,
    plan_revision_id    UUID NOT NULL REFERENCES curriculum_studio.plan_revisions(id) ON DELETE RESTRICT,
    learner_profile_id  UUID REFERENCES curriculum_studio.learner_profiles(id) ON DELETE SET NULL,
    window_start        DATE,
    window_end          DATE,
    status              TEXT NOT NULL DEFAULT 'requested',
    input_snapshot      JSONB NOT NULL,
    input_fingerprint   TEXT NOT NULL,
    requested_by_subject_ref TEXT NOT NULL DEFAULT '',
    started_at          TIMESTAMPTZ,
    completed_at        TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (status IN ('requested', 'running', 'ready', 'failed', 'cancelled')),
    CHECK (input_fingerprint <> ''),
    CHECK (jsonb_typeof(input_snapshot) = 'object'),
    CHECK (window_end IS NULL OR window_start IS NULL OR window_end >= window_start)
);

CREATE INDEX idx_studio_mat_runs_workspace ON curriculum_studio.materialization_runs(workspace_id);
CREATE INDEX idx_studio_mat_runs_revision ON curriculum_studio.materialization_runs(plan_revision_id);
CREATE INDEX idx_studio_mat_runs_fingerprint ON curriculum_studio.materialization_runs(plan_revision_id, input_fingerprint);
CREATE INDEX idx_studio_mat_runs_status ON curriculum_studio.materialization_runs(status);

-- Resumable workflow stages. A failed assessment-generation step resumes here.
CREATE TABLE curriculum_studio.workflow_stages (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    run_id          UUID NOT NULL REFERENCES curriculum_studio.materialization_runs(id) ON DELETE CASCADE,
    stage_key       TEXT NOT NULL,
    position        INTEGER NOT NULL,
    status          TEXT NOT NULL DEFAULT 'pending',
    input           JSONB NOT NULL DEFAULT '{}'::jsonb,
    output          JSONB NOT NULL DEFAULT '{}'::jsonb,
    started_at      TIMESTAMPTZ,
    completed_at    TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (run_id, stage_key),
    UNIQUE (run_id, position),
    CHECK (stage_key <> ''),
    CHECK (position >= 1),
    CHECK (status IN ('pending', 'running', 'succeeded', 'failed', 'skipped')),
    CHECK (jsonb_typeof(input) = 'object'),
    CHECK (jsonb_typeof(output) = 'object')
);

CREATE INDEX idx_studio_workflow_stages_run ON curriculum_studio.workflow_stages(run_id, position);

CREATE TABLE curriculum_studio.workflow_attempts (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    stage_id        UUID NOT NULL REFERENCES curriculum_studio.workflow_stages(id) ON DELETE CASCADE,
    attempt_number  INTEGER NOT NULL,
    status          TEXT NOT NULL DEFAULT 'running',
    agent           TEXT NOT NULL DEFAULT '',
    error           TEXT NOT NULL DEFAULT '',
    started_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    finished_at     TIMESTAMPTZ,
    UNIQUE (stage_id, attempt_number),
    CHECK (attempt_number >= 1),
    CHECK (status IN ('running', 'succeeded', 'failed'))
);

CREATE INDEX idx_studio_workflow_attempts_stage ON curriculum_studio.workflow_attempts(stage_id, attempt_number DESC);

-- Generated / materialized items. Locked items cannot be overwritten.
CREATE TABLE curriculum_studio.materialized_items (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    run_id              UUID NOT NULL REFERENCES curriculum_studio.materialization_runs(id) ON DELETE CASCADE,
    plan_revision_id    UUID NOT NULL REFERENCES curriculum_studio.plan_revisions(id) ON DELETE RESTRICT,
    unit_id             UUID REFERENCES curriculum_studio.units(id) ON DELETE SET NULL,
    project_id          UUID REFERENCES curriculum_studio.projects(id) ON DELETE SET NULL,
    outcome_id          UUID REFERENCES curriculum_studio.outcomes(id) ON DELETE SET NULL,
    kind                TEXT NOT NULL,
    title               TEXT NOT NULL,
    body                JSONB NOT NULL DEFAULT '{}'::jsonb,
    status              TEXT NOT NULL DEFAULT 'draft',
    locked              BOOLEAN NOT NULL DEFAULT FALSE,
    locked_at           TIMESTAMPTZ,
    locked_by_subject_ref TEXT NOT NULL DEFAULT '',
    supersedes_item_id  UUID REFERENCES curriculum_studio.materialized_items(id) ON DELETE SET NULL,
    provenance          JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (kind IN (
        'lesson', 'teacher_guide', 'student_instructions', 'practice',
        'assignment', 'discussion_guide', 'worksheet', 'assessment',
        'rubric', 'project_task', 'answer_key', 'media_prompt',
        'printable_packet', 'session_spec'
    )),
    CHECK (title <> ''),
    CHECK (status IN ('draft', 'ready', 'published', 'superseded')),
    CHECK (jsonb_typeof(body) = 'object'),
    CHECK (jsonb_typeof(provenance) = 'object'),
    CHECK ((locked = FALSE AND locked_at IS NULL) OR (locked = TRUE AND locked_at IS NOT NULL))
);

CREATE INDEX idx_studio_mat_items_run ON curriculum_studio.materialized_items(run_id);
CREATE INDEX idx_studio_mat_items_revision ON curriculum_studio.materialized_items(plan_revision_id);
CREATE INDEX idx_studio_mat_items_kind ON curriculum_studio.materialized_items(kind);
CREATE INDEX idx_studio_mat_items_supersedes ON curriculum_studio.materialized_items(supersedes_item_id);

CREATE TABLE curriculum_studio.materialized_item_edits (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    item_id         UUID NOT NULL REFERENCES curriculum_studio.materialized_items(id) ON DELETE CASCADE,
    editor_subject_ref TEXT NOT NULL DEFAULT '',
    patch           JSONB NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (jsonb_typeof(patch) = 'object')
);

CREATE INDEX idx_studio_mat_item_edits_item ON curriculum_studio.materialized_item_edits(item_id, created_at DESC);

-- Assessment publication requires a linked rubric or answer key (trigger).
CREATE TABLE curriculum_studio.assessment_supports (
    assessment_item_id  UUID NOT NULL REFERENCES curriculum_studio.materialized_items(id) ON DELETE CASCADE,
    support_item_id     UUID NOT NULL REFERENCES curriculum_studio.materialized_items(id) ON DELETE CASCADE,
    PRIMARY KEY (assessment_item_id, support_item_id),
    CHECK (assessment_item_id <> support_item_id)
);

CREATE INDEX idx_studio_assessment_supports_support ON curriculum_studio.assessment_supports(support_item_id);

CREATE TABLE curriculum_studio.exports (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id    UUID NOT NULL REFERENCES curriculum_studio.workspaces(id) ON DELETE CASCADE,
    run_id          UUID REFERENCES curriculum_studio.materialization_runs(id) ON DELETE SET NULL,
    plan_revision_id UUID REFERENCES curriculum_studio.plan_revisions(id) ON DELETE SET NULL,
    format          TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'requested',
    artifact_ref    TEXT NOT NULL DEFAULT '',
    checksum        TEXT NOT NULL DEFAULT '',
    requested_by_subject_ref TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at    TIMESTAMPTZ,
    CHECK (format IN ('pdf', 'markdown', 'docx', 'csv', 'json', 'ical')),
    CHECK (status IN ('requested', 'ready', 'failed'))
);

CREATE INDEX idx_studio_exports_workspace ON curriculum_studio.exports(workspace_id);
CREATE INDEX idx_studio_exports_run ON curriculum_studio.exports(run_id);

-- Durable outbox for domain events. Consumers poll; never dual-write only in-memory.
CREATE TABLE curriculum_studio.outbox_events (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id    UUID REFERENCES curriculum_studio.workspaces(id) ON DELETE SET NULL,
    event_type      TEXT NOT NULL,
    aggregate_kind  TEXT NOT NULL,
    aggregate_id    UUID NOT NULL,
    payload         JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    published_at    TIMESTAMPTZ,
    CHECK (event_type <> ''),
    CHECK (aggregate_kind <> ''),
    CHECK (jsonb_typeof(payload) = 'object')
);

CREATE INDEX idx_studio_outbox_unpublished ON curriculum_studio.outbox_events(created_at) WHERE published_at IS NULL;
CREATE INDEX idx_studio_outbox_type ON curriculum_studio.outbox_events(event_type);
CREATE INDEX idx_studio_outbox_aggregate ON curriculum_studio.outbox_events(aggregate_kind, aggregate_id);

CREATE TABLE curriculum_studio.webhook_endpoints (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id    UUID NOT NULL REFERENCES curriculum_studio.workspaces(id) ON DELETE CASCADE,
    url             TEXT NOT NULL,
    secret_ref      TEXT NOT NULL DEFAULT '',
    event_types     TEXT[] NOT NULL DEFAULT '{}',
    status          TEXT NOT NULL DEFAULT 'active',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (url <> ''),
    CHECK (status IN ('active', 'paused', 'retired'))
);

CREATE INDEX idx_studio_webhook_endpoints_workspace ON curriculum_studio.webhook_endpoints(workspace_id);

-- Idempotency: one delivery row per (endpoint, event).
CREATE TABLE curriculum_studio.webhook_deliveries (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    endpoint_id     UUID NOT NULL REFERENCES curriculum_studio.webhook_endpoints(id) ON DELETE CASCADE,
    event_id        UUID NOT NULL REFERENCES curriculum_studio.outbox_events(id) ON DELETE CASCADE,
    idempotency_key TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'pending',
    attempt_count   INTEGER NOT NULL DEFAULT 0,
    last_error      TEXT NOT NULL DEFAULT '',
    delivered_at    TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (endpoint_id, event_id),
    UNIQUE (idempotency_key),
    CHECK (idempotency_key <> ''),
    CHECK (status IN ('pending', 'delivered', 'failed')),
    CHECK (attempt_count >= 0)
);

CREATE INDEX idx_studio_webhook_deliveries_status ON curriculum_studio.webhook_deliveries(status);

-- Generic inbound idempotency keys (API writes, Primer callbacks).
CREATE TABLE curriculum_studio.idempotency_keys (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id    UUID NOT NULL REFERENCES curriculum_studio.workspaces(id) ON DELETE CASCADE,
    scope           TEXT NOT NULL,
    key             TEXT NOT NULL,
    request_hash    TEXT NOT NULL DEFAULT '',
    response_ref    TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (workspace_id, scope, key),
    CHECK (scope <> ''),
    CHECK (key <> '')
);

CREATE TABLE curriculum_studio.audit_events (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id    UUID REFERENCES curriculum_studio.workspaces(id) ON DELETE SET NULL,
    actor_subject_ref TEXT NOT NULL DEFAULT '',
    action          TEXT NOT NULL,
    entity_kind     TEXT NOT NULL,
    entity_id       UUID,
    before          JSONB NOT NULL DEFAULT '{}'::jsonb,
    after           JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (action <> ''),
    CHECK (entity_kind <> ''),
    CHECK (jsonb_typeof(before) = 'object'),
    CHECK (jsonb_typeof(after) = 'object')
);

CREATE INDEX idx_studio_audit_workspace ON curriculum_studio.audit_events(workspace_id, created_at DESC);
CREATE INDEX idx_studio_audit_entity ON curriculum_studio.audit_events(entity_kind, entity_id);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS curriculum_studio.audit_events;
DROP TABLE IF EXISTS curriculum_studio.idempotency_keys;
DROP TABLE IF EXISTS curriculum_studio.webhook_deliveries;
DROP TABLE IF EXISTS curriculum_studio.webhook_endpoints;
DROP TABLE IF EXISTS curriculum_studio.outbox_events;
DROP TABLE IF EXISTS curriculum_studio.exports;
DROP TABLE IF EXISTS curriculum_studio.assessment_supports;
DROP TABLE IF EXISTS curriculum_studio.materialized_item_edits;
DROP TABLE IF EXISTS curriculum_studio.materialized_items;
DROP TABLE IF EXISTS curriculum_studio.workflow_attempts;
DROP TABLE IF EXISTS curriculum_studio.workflow_stages;
DROP TABLE IF EXISTS curriculum_studio.materialization_runs;
DROP TABLE IF EXISTS curriculum_studio.learner_profiles;
-- +goose StatementEnd
