-- +goose Up
-- +goose StatementBegin

-- Catalog ownership is part of the scope encoded by every existing edge.
-- There are no D4 repository reassignment APIs, so reject direct ownership
-- changes rather than allowing existing edges to become ambiguous.
CREATE OR REPLACE FUNCTION curriculum_studio.assert_framework_workspace_immutable()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF NEW.workspace_id IS DISTINCT FROM OLD.workspace_id THEN
        RAISE EXCEPTION 'standard framework workspace ownership is immutable'
            USING ERRCODE = 'check_violation';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_framework_workspace_immutable
    BEFORE UPDATE OF workspace_id
    ON curriculum_studio.standard_frameworks
    FOR EACH ROW
    EXECUTE FUNCTION curriculum_studio.assert_framework_workspace_immutable();

CREATE OR REPLACE FUNCTION curriculum_studio.assert_catalog_standard_framework_immutable()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF NEW.framework_id IS DISTINCT FROM OLD.framework_id THEN
        RAISE EXCEPTION 'catalog standard framework assignment is immutable'
            USING ERRCODE = 'check_violation';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_catalog_standard_framework_immutable
    BEFORE UPDATE OF framework_id
    ON curriculum_studio.catalog_standards
    FOR EACH ROW
    EXECUTE FUNCTION curriculum_studio.assert_catalog_standard_framework_immutable();

-- Resources validate workspace/tenant ownership when resources change. Keep
-- the other side immutable too, so an existing workspace-owned resource cannot
-- become cross-tenant through a raw workspace reassignment.
CREATE OR REPLACE FUNCTION curriculum_studio.assert_workspace_tenant_immutable()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF NEW.tenant_id IS DISTINCT FROM OLD.tenant_id THEN
        RAISE EXCEPTION 'workspace tenant ownership is immutable'
            USING ERRCODE = 'check_violation';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_workspace_tenant_immutable
    BEFORE UPDATE OF tenant_id
    ON curriculum_studio.workspaces
    FOR EACH ROW
    EXECUTE FUNCTION curriculum_studio.assert_workspace_tenant_immutable();

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TRIGGER IF EXISTS trg_workspace_tenant_immutable ON curriculum_studio.workspaces;
DROP FUNCTION IF EXISTS curriculum_studio.assert_workspace_tenant_immutable();
DROP TRIGGER IF EXISTS trg_catalog_standard_framework_immutable ON curriculum_studio.catalog_standards;
DROP FUNCTION IF EXISTS curriculum_studio.assert_catalog_standard_framework_immutable();
DROP TRIGGER IF EXISTS trg_framework_workspace_immutable ON curriculum_studio.standard_frameworks;
DROP FUNCTION IF EXISTS curriculum_studio.assert_framework_workspace_immutable();
-- +goose StatementEnd
