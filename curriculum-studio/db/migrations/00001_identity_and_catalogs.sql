-- +goose Up
-- +goose StatementBegin

-- Curriculum Studio is a standalone service with its own database.
-- Tables live in the curriculum_studio schema so an accidental migrate
-- against the Primer LMS or TV instance cannot collide with public tables.
-- There are no foreign keys, views, or dblink/FDW reads into other databases.
-- Primer learner IDs and auth subjects are stored only as opaque text.

CREATE SCHEMA IF NOT EXISTS curriculum_studio;

CREATE TABLE curriculum_studio.tenants (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    slug        TEXT NOT NULL UNIQUE,
    name        TEXT NOT NULL,
    status      TEXT NOT NULL DEFAULT 'active',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (status IN ('active', 'suspended', 'retired')),
    CHECK (slug <> ''),
    CHECK (name <> '')
);

-- Workspaces: school, family, co-op, teacher, or authoring organization.
CREATE TABLE curriculum_studio.workspaces (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL REFERENCES curriculum_studio.tenants(id) ON DELETE CASCADE,
    slug        TEXT NOT NULL,
    name        TEXT NOT NULL,
    kind        TEXT NOT NULL DEFAULT 'teacher',
    status      TEXT NOT NULL DEFAULT 'active',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, slug),
    CHECK (kind IN ('school', 'family', 'coop', 'teacher', 'organization')),
    CHECK (status IN ('active', 'archived')),
    CHECK (slug <> ''),
    CHECK (name <> '')
);

CREATE INDEX idx_studio_workspaces_tenant ON curriculum_studio.workspaces(tenant_id);
CREATE INDEX idx_studio_workspaces_kind ON curriculum_studio.workspaces(kind);

-- Authorization projections only. No passwords, tokens, or credential hashes.
-- subject_ref is opaque text. Canonical Identity humans: identity:<uuid>;
-- services: identity:svc:<id>. See curriculum-studio-foundation-crosswalk.md.
CREATE TABLE curriculum_studio.workspace_memberships (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id    UUID NOT NULL REFERENCES curriculum_studio.workspaces(id) ON DELETE CASCADE,
    subject_ref     TEXT NOT NULL,
    subject_kind    TEXT NOT NULL DEFAULT 'human',
    display_name    TEXT NOT NULL DEFAULT '',
    role            TEXT NOT NULL DEFAULT 'viewer',
    status          TEXT NOT NULL DEFAULT 'active',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (workspace_id, subject_ref),
    CHECK (subject_ref <> ''),
    CHECK (subject_kind IN ('human', 'service')),
    CHECK (role IN ('owner', 'admin', 'author', 'reviewer', 'viewer')),
    CHECK (status IN ('active', 'invited', 'revoked'))
);

CREATE INDEX idx_studio_memberships_workspace ON curriculum_studio.workspace_memberships(workspace_id);
CREATE INDEX idx_studio_memberships_subject ON curriculum_studio.workspace_memberships(subject_ref);

-- Bounded snapshots of external Primer / IdP identifiers. Never FKs.
CREATE TABLE curriculum_studio.integration_identities (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id    UUID NOT NULL REFERENCES curriculum_studio.workspaces(id) ON DELETE CASCADE,
    system          TEXT NOT NULL,
    external_kind   TEXT NOT NULL,
    external_ref    TEXT NOT NULL,
    display_label   TEXT NOT NULL DEFAULT '',
    snapshot        JSONB NOT NULL DEFAULT '{}'::jsonb,
    last_seen_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (workspace_id, system, external_kind, external_ref),
    CHECK (system IN ('primer_lms', 'primer_identity', 'oidc', 'other')),
    CHECK (external_kind IN ('learner', 'educator', 'class', 'auth_subject', 'service')),
    CHECK (external_ref <> ''),
    CHECK (jsonb_typeof(snapshot) = 'object')
);

CREATE INDEX idx_studio_integration_workspace ON curriculum_studio.integration_identities(workspace_id);
CREATE INDEX idx_studio_integration_ref ON curriculum_studio.integration_identities(system, external_kind, external_ref);

-- Shared / custom standards catalogs. Independent of the LMS standards table.
CREATE TABLE curriculum_studio.standard_frameworks (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id    UUID REFERENCES curriculum_studio.workspaces(id) ON DELETE CASCADE,
    code            TEXT NOT NULL,
    name            TEXT NOT NULL,
    jurisdiction    TEXT NOT NULL DEFAULT '',
    version         TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (code <> ''),
    CHECK (name <> '')
);

-- Global frameworks (workspace_id IS NULL) are unique on code; custom
-- workspace frameworks are unique per workspace.
CREATE UNIQUE INDEX idx_studio_frameworks_global_code
    ON curriculum_studio.standard_frameworks(code)
    WHERE workspace_id IS NULL;
CREATE UNIQUE INDEX idx_studio_frameworks_workspace_code
    ON curriculum_studio.standard_frameworks(workspace_id, code)
    WHERE workspace_id IS NOT NULL;

CREATE TABLE curriculum_studio.catalog_standards (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    framework_id    UUID NOT NULL REFERENCES curriculum_studio.standard_frameworks(id) ON DELETE CASCADE,
    parent_id       UUID REFERENCES curriculum_studio.catalog_standards(id) ON DELETE SET NULL,
    code            TEXT NOT NULL,
    subject_code    TEXT NOT NULL DEFAULT '',
    grade_band      TEXT NOT NULL DEFAULT '',
    domain          TEXT NOT NULL DEFAULT '',
    cluster         TEXT NOT NULL DEFAULT '',
    description     TEXT NOT NULL DEFAULT '',
    metadata        JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (framework_id, code),
    CHECK (code <> ''),
    CHECK (jsonb_typeof(metadata) = 'object')
);

CREATE INDEX idx_studio_catalog_standards_framework ON curriculum_studio.catalog_standards(framework_id);
CREATE INDEX idx_studio_catalog_standards_parent ON curriculum_studio.catalog_standards(parent_id);
CREATE INDEX idx_studio_catalog_standards_subject ON curriculum_studio.catalog_standards(subject_code);

CREATE TABLE curriculum_studio.standard_crosswalks (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    from_standard_id    UUID NOT NULL REFERENCES curriculum_studio.catalog_standards(id) ON DELETE CASCADE,
    to_standard_id      UUID NOT NULL REFERENCES curriculum_studio.catalog_standards(id) ON DELETE CASCADE,
    relationship        TEXT NOT NULL DEFAULT 'related',
    notes               TEXT NOT NULL DEFAULT '',
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (from_standard_id, to_standard_id),
    CHECK (from_standard_id <> to_standard_id),
    CHECK (relationship IN ('equivalent', 'broader', 'narrower', 'related'))
);

CREATE INDEX idx_studio_crosswalks_to ON curriculum_studio.standard_crosswalks(to_standard_id);

-- Catalog-level prerequisite DAG. Cycles are rejected by trigger (00004).
CREATE TABLE curriculum_studio.catalog_standard_prerequisites (
    standard_id         UUID NOT NULL REFERENCES curriculum_studio.catalog_standards(id) ON DELETE CASCADE,
    prerequisite_id     UUID NOT NULL REFERENCES curriculum_studio.catalog_standards(id) ON DELETE CASCADE,
    PRIMARY KEY (standard_id, prerequisite_id),
    CHECK (standard_id <> prerequisite_id)
);

-- Resource catalog: metadata and references only. Files live elsewhere.
CREATE TABLE curriculum_studio.resources (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID NOT NULL REFERENCES curriculum_studio.tenants(id) ON DELETE CASCADE,
    workspace_id    UUID REFERENCES curriculum_studio.workspaces(id) ON DELETE CASCADE,
    kind            TEXT NOT NULL DEFAULT 'other',
    title           TEXT NOT NULL,
    authors         TEXT NOT NULL DEFAULT '',
    isbn            TEXT NOT NULL DEFAULT '',
    url             TEXT NOT NULL DEFAULT '',
    artifact_ref    TEXT NOT NULL DEFAULT '',
    metadata        JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (kind IN ('book', 'document', 'video', 'tool', 'project_supply', 'url', 'other')),
    CHECK (title <> ''),
    CHECK (jsonb_typeof(metadata) = 'object')
);

CREATE INDEX idx_studio_resources_tenant ON curriculum_studio.resources(tenant_id);
CREATE INDEX idx_studio_resources_workspace ON curriculum_studio.resources(workspace_id);
CREATE INDEX idx_studio_resources_kind ON curriculum_studio.resources(kind);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS curriculum_studio.resources;
DROP TABLE IF EXISTS curriculum_studio.catalog_standard_prerequisites;
DROP TABLE IF EXISTS curriculum_studio.standard_crosswalks;
DROP TABLE IF EXISTS curriculum_studio.catalog_standards;
DROP TABLE IF EXISTS curriculum_studio.standard_frameworks;
DROP TABLE IF EXISTS curriculum_studio.integration_identities;
DROP TABLE IF EXISTS curriculum_studio.workspace_memberships;
DROP TABLE IF EXISTS curriculum_studio.workspaces;
DROP TABLE IF EXISTS curriculum_studio.tenants;
DROP SCHEMA IF EXISTS curriculum_studio;
-- +goose StatementEnd
