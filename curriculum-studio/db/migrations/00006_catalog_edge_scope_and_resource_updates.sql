-- +goose Up
-- +goose StatementBegin

-- Crosswalks and prerequisites do not carry a workspace column. Their
-- endpoint standards therefore must be workspace-compatible: two workspace
-- owned frameworks may not be mixed, while a global framework may connect to
-- any workspace-owned framework. This is enforced for direct SQL as well as
-- repository calls.
CREATE OR REPLACE FUNCTION curriculum_studio.assert_catalog_edge_workspace_compatible()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    first_id uuid;
    second_id uuid;
    first_workspace uuid;
    second_workspace uuid;
BEGIN
    IF TG_TABLE_NAME = 'standard_crosswalks' THEN
        first_id := NEW.from_standard_id;
        second_id := NEW.to_standard_id;
    ELSE
        first_id := NEW.standard_id;
        second_id := NEW.prerequisite_id;
    END IF;

    SELECT f.workspace_id INTO first_workspace
    FROM curriculum_studio.catalog_standards s
    JOIN curriculum_studio.standard_frameworks f ON f.id = s.framework_id
    WHERE s.id = first_id;
    SELECT f.workspace_id INTO second_workspace
    FROM curriculum_studio.catalog_standards s
    JOIN curriculum_studio.standard_frameworks f ON f.id = s.framework_id
    WHERE s.id = second_id;

    IF first_workspace IS NOT NULL
       AND second_workspace IS NOT NULL
       AND first_workspace IS DISTINCT FROM second_workspace THEN
        RAISE EXCEPTION 'catalog edge endpoints must belong to compatible workspaces'
            USING ERRCODE = 'check_violation';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_crosswalk_workspace_compatible
    BEFORE INSERT OR UPDATE ON curriculum_studio.standard_crosswalks
    FOR EACH ROW
    EXECUTE FUNCTION curriculum_studio.assert_catalog_edge_workspace_compatible();

CREATE TRIGGER trg_prereq_workspace_compatible
    BEFORE INSERT OR UPDATE ON curriculum_studio.catalog_standard_prerequisites
    FOR EACH ROW
    EXECUTE FUNCTION curriculum_studio.assert_catalog_edge_workspace_compatible();

-- The resource policy is authoritative for every update, not only metadata
-- updates. This covers direct SQL changes to artifact_ref and url as well as
-- updates that replace the whole row's metadata.
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

    IF NEW.url <> '' AND NEW.url !~ '^https?://[^[:space:]]+$' THEN
        RAISE EXCEPTION 'resource url must be an http or https reference'
            USING ERRCODE = 'check_violation';
    END IF;

    IF octet_length(NEW.metadata::text) > 16 * 1024 THEN
        RAISE EXCEPTION 'resource metadata exceeds 16384 octets'
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

DROP TRIGGER IF EXISTS trg_resource_metadata_policy ON curriculum_studio.resources;
CREATE TRIGGER trg_resource_metadata_policy
    BEFORE INSERT OR UPDATE
    ON curriculum_studio.resources
    FOR EACH ROW
    EXECUTE FUNCTION curriculum_studio.assert_resource_metadata_policy();

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TRIGGER IF EXISTS trg_resource_metadata_policy ON curriculum_studio.resources;
-- Restore the 00005 function and narrower trigger shape.
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
DROP TRIGGER IF EXISTS trg_prereq_workspace_compatible ON curriculum_studio.catalog_standard_prerequisites;
DROP TRIGGER IF EXISTS trg_crosswalk_workspace_compatible ON curriculum_studio.standard_crosswalks;
DROP FUNCTION IF EXISTS curriculum_studio.assert_catalog_edge_workspace_compatible();
-- +goose StatementEnd
