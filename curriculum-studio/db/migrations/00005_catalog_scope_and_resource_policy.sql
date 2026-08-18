-- +goose Up
-- +goose StatementBegin

-- Catalog hierarchy must not cross framework boundaries. The foreign key on
-- parent_id proves that the parent exists; this trigger proves it belongs to
-- the same framework, including for direct SQL callers.
CREATE OR REPLACE FUNCTION curriculum_studio.assert_catalog_parent_same_framework()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    parent_framework uuid;
BEGIN
    IF NEW.parent_id IS NULL THEN
        RETURN NEW;
    END IF;

    SELECT framework_id INTO parent_framework
    FROM curriculum_studio.catalog_standards
    WHERE id = NEW.parent_id;

    IF parent_framework IS DISTINCT FROM NEW.framework_id THEN
        RAISE EXCEPTION 'catalog standard parent must belong to the same framework'
            USING ERRCODE = 'check_violation';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_catalog_parent_same_framework
    BEFORE INSERT OR UPDATE OF framework_id, parent_id
    ON curriculum_studio.catalog_standards
    FOR EACH ROW
    EXECUTE FUNCTION curriculum_studio.assert_catalog_parent_same_framework();

-- A resource may be tenant-global (workspace_id NULL), or owned by a
-- workspace of the same tenant. Keep this invariant in the database so raw
-- SQL cannot attach a resource to a foreign tenant's workspace.
CREATE OR REPLACE FUNCTION curriculum_studio.assert_resource_workspace_tenant()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    workspace_tenant uuid;
BEGIN
    IF NEW.workspace_id IS NULL THEN
        RETURN NEW;
    END IF;

    SELECT tenant_id INTO workspace_tenant
    FROM curriculum_studio.workspaces
    WHERE id = NEW.workspace_id;

    IF workspace_tenant IS DISTINCT FROM NEW.tenant_id THEN
        RAISE EXCEPTION 'resource workspace must belong to resource tenant'
            USING ERRCODE = 'check_violation';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_resource_workspace_tenant
    BEFORE INSERT OR UPDATE OF tenant_id, workspace_id
    ON curriculum_studio.resources
    FOR EACH ROW
    EXECUTE FUNCTION curriculum_studio.assert_resource_workspace_tenant();

-- Resource metadata is an explicit, bounded metadata shape, not an arbitrary
-- JSON document. In particular, nested objects and unknown keys are refused;
-- this prevents file bytes or base64 payloads from being smuggled through a
-- denylist spelling/casing/nesting bypass. Files are never stored here.
CREATE OR REPLACE FUNCTION curriculum_studio.assert_resource_metadata_policy()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    key text;
    value jsonb;
    tag jsonb;
BEGIN
    IF NEW.artifact_ref <> ''
       AND (length(NEW.artifact_ref) > 512
            OR NEW.artifact_ref !~ '^(obj|urn):[A-Za-z0-9][A-Za-z0-9._:/-]*$') THEN
        RAISE EXCEPTION 'resource artifact_ref must be an object or URN reference'
            USING ERRCODE = 'check_violation';
    END IF;

    IF jsonb_typeof(NEW.metadata) IS DISTINCT FROM 'object' THEN
        RAISE EXCEPTION 'resource metadata must be a JSON object'
            USING ERRCODE = 'check_violation';
    END IF;

    FOR key IN SELECT jsonb_object_keys(NEW.metadata) LOOP
        IF key NOT IN ('description', 'publisher', 'language', 'license', 'note', 'published_year', 'tags') THEN
            RAISE EXCEPTION 'resource metadata key % is not permitted', key
                USING ERRCODE = 'check_violation';
        END IF;
        value := NEW.metadata -> key;
        IF key IN ('description', 'publisher', 'language', 'license', 'note') THEN
            IF jsonb_typeof(value) IS DISTINCT FROM 'string'
               OR length(value #>> '{}') > 4096 THEN
                RAISE EXCEPTION 'resource metadata field % must be a bounded string', key
                    USING ERRCODE = 'check_violation';
            END IF;
        ELSIF key = 'published_year' THEN
            IF jsonb_typeof(value) IS DISTINCT FROM 'number'
               OR (value::text)::numeric <> trunc((value::text)::numeric)
               OR (value::text)::numeric < 1000
               OR (value::text)::numeric > 3000 THEN
                RAISE EXCEPTION 'resource metadata published_year is invalid'
                    USING ERRCODE = 'check_violation';
            END IF;
        ELSIF jsonb_typeof(value) IS DISTINCT FROM 'array'
              OR jsonb_array_length(value) > 64 THEN
            RAISE EXCEPTION 'resource metadata tags must be a bounded string array'
                USING ERRCODE = 'check_violation';
        ELSE
            FOR tag IN SELECT jsonb_array_elements(value) LOOP
                IF jsonb_typeof(tag) IS DISTINCT FROM 'string'
                   OR length(tag #>> '{}') > 128 THEN
                    RAISE EXCEPTION 'resource metadata tags must contain bounded strings'
                        USING ERRCODE = 'check_violation';
                END IF;
            END LOOP;
        END IF;
    END LOOP;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_resource_metadata_policy
    BEFORE INSERT OR UPDATE OF metadata
    ON curriculum_studio.resources
    FOR EACH ROW
    EXECUTE FUNCTION curriculum_studio.assert_resource_metadata_policy();

-- The recursive walk must be serialized in the trigger itself. Repository
-- endpoint locks are only defense-in-depth and cannot protect direct SQL.
-- Lock both UUID endpoints in lexical order for the duration of the
-- transaction; hashtextextended gives a deterministic bigint advisory key.
CREATE OR REPLACE FUNCTION curriculum_studio.assert_catalog_prereq_acyclic_serialized()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    first_key bigint;
    second_key bigint;
BEGIN
    first_key := hashtextextended(LEAST(NEW.standard_id::text, NEW.prerequisite_id::text), 0);
    second_key := hashtextextended(GREATEST(NEW.standard_id::text, NEW.prerequisite_id::text), 0);
    PERFORM pg_advisory_xact_lock(first_key);
    IF second_key <> first_key THEN
        PERFORM pg_advisory_xact_lock(second_key);
    END IF;

    IF EXISTS (
        WITH RECURSIVE walk AS (
            SELECT NEW.prerequisite_id AS id
            UNION ALL
            SELECT p.prerequisite_id
            FROM curriculum_studio.catalog_standard_prerequisites p
            JOIN walk w ON p.standard_id = w.id
        )
        SELECT 1 FROM walk WHERE id = NEW.standard_id
    ) THEN
        RAISE EXCEPTION 'catalog standard prerequisite cycle detected for %', NEW.standard_id
            USING ERRCODE = 'integrity_constraint_violation';
    END IF;
    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS trg_catalog_prereq_acyclic
    ON curriculum_studio.catalog_standard_prerequisites;
CREATE TRIGGER trg_catalog_prereq_acyclic
    BEFORE INSERT OR UPDATE ON curriculum_studio.catalog_standard_prerequisites
    FOR EACH ROW EXECUTE FUNCTION curriculum_studio.assert_catalog_prereq_acyclic_serialized();

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TRIGGER IF EXISTS trg_catalog_prereq_acyclic ON curriculum_studio.catalog_standard_prerequisites;
CREATE TRIGGER trg_catalog_prereq_acyclic
    BEFORE INSERT OR UPDATE ON curriculum_studio.catalog_standard_prerequisites
    FOR EACH ROW EXECUTE FUNCTION curriculum_studio.assert_catalog_prereq_acyclic();
DROP FUNCTION IF EXISTS curriculum_studio.assert_catalog_prereq_acyclic_serialized();

DROP TRIGGER IF EXISTS trg_resource_metadata_policy ON curriculum_studio.resources;
DROP FUNCTION IF EXISTS curriculum_studio.assert_resource_metadata_policy();
DROP TRIGGER IF EXISTS trg_resource_workspace_tenant ON curriculum_studio.resources;
DROP FUNCTION IF EXISTS curriculum_studio.assert_resource_workspace_tenant();
DROP TRIGGER IF EXISTS trg_catalog_parent_same_framework ON curriculum_studio.catalog_standards;
DROP FUNCTION IF EXISTS curriculum_studio.assert_catalog_parent_same_framework();
-- +goose StatementEnd
