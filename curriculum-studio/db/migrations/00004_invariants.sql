-- +goose Up
-- +goose StatementBegin

-- ---------------------------------------------------------------------------
-- Published plan revisions are immutable. Drafts remain editable.
-- ---------------------------------------------------------------------------
CREATE OR REPLACE FUNCTION curriculum_studio.forbid_published_plan_mutation()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        IF OLD.status IN ('published', 'superseded') THEN
            RAISE EXCEPTION 'published plan revision % is immutable', OLD.id
                USING ERRCODE = 'restrict_violation';
        END IF;
        RETURN OLD;
    END IF;

    IF OLD.status IN ('published', 'superseded') THEN
        -- Allow only the terminal draft -> published -> superseded transitions.
        IF TG_OP = 'UPDATE'
           AND OLD.status = 'published'
           AND NEW.status = 'superseded'
           AND NEW.revision = OLD.revision
           AND NEW.curriculum_id = OLD.curriculum_id
           AND NEW.title = OLD.title
           AND NEW.brief = OLD.brief
           AND NEW.published_at = OLD.published_at
           AND NEW.published_by_subject_ref = OLD.published_by_subject_ref
           AND NEW.created_at = OLD.created_at
        THEN
            RETURN NEW;
        END IF;
        RAISE EXCEPTION 'published plan revision % is immutable', OLD.id
            USING ERRCODE = 'restrict_violation';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_plan_revisions_immutable
    BEFORE UPDATE OR DELETE ON curriculum_studio.plan_revisions
    FOR EACH ROW
    EXECUTE FUNCTION curriculum_studio.forbid_published_plan_mutation();

-- Children of a published revision cannot be inserted, updated, or deleted.
CREATE OR REPLACE FUNCTION curriculum_studio.forbid_published_plan_child_mutation()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    rev_id uuid;
    rev_status text;
BEGIN
    IF TG_OP = 'DELETE' THEN
        rev_id := OLD.plan_revision_id;
    ELSE
        rev_id := NEW.plan_revision_id;
    END IF;

    SELECT status INTO rev_status
    FROM curriculum_studio.plan_revisions
    WHERE id = rev_id;

    IF rev_status IN ('published', 'superseded') THEN
        RAISE EXCEPTION 'cannot mutate % of immutable plan revision %', TG_TABLE_NAME, rev_id
            USING ERRCODE = 'restrict_violation';
    END IF;

    IF TG_OP = 'DELETE' THEN
        RETURN OLD;
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_objectives_published_lock
    BEFORE INSERT OR UPDATE OR DELETE ON curriculum_studio.objectives
    FOR EACH ROW EXECUTE FUNCTION curriculum_studio.forbid_published_plan_child_mutation();
CREATE TRIGGER trg_outcomes_published_lock
    BEFORE INSERT OR UPDATE OR DELETE ON curriculum_studio.outcomes
    FOR EACH ROW EXECUTE FUNCTION curriculum_studio.forbid_published_plan_child_mutation();
CREATE TRIGGER trg_arcs_published_lock
    BEFORE INSERT OR UPDATE OR DELETE ON curriculum_studio.learning_arcs
    FOR EACH ROW EXECUTE FUNCTION curriculum_studio.forbid_published_plan_child_mutation();
CREATE TRIGGER trg_units_published_lock
    BEFORE INSERT OR UPDATE OR DELETE ON curriculum_studio.units
    FOR EACH ROW EXECUTE FUNCTION curriculum_studio.forbid_published_plan_child_mutation();
CREATE TRIGGER trg_projects_published_lock
    BEFORE INSERT OR UPDATE OR DELETE ON curriculum_studio.projects
    FOR EACH ROW EXECUTE FUNCTION curriculum_studio.forbid_published_plan_child_mutation();
CREATE TRIGGER trg_evidence_published_lock
    BEFORE INSERT OR UPDATE OR DELETE ON curriculum_studio.evidence_requirements
    FOR EACH ROW EXECUTE FUNCTION curriculum_studio.forbid_published_plan_child_mutation();
CREATE TRIGGER trg_sched_published_lock
    BEFORE INSERT OR UPDATE OR DELETE ON curriculum_studio.scheduling_constraints
    FOR EACH ROW EXECUTE FUNCTION curriculum_studio.forbid_published_plan_child_mutation();
CREATE TRIGGER trg_plan_resources_published_lock
    BEFORE INSERT OR UPDATE OR DELETE ON curriculum_studio.plan_resources
    FOR EACH ROW EXECUTE FUNCTION curriculum_studio.forbid_published_plan_child_mutation();
CREATE TRIGGER trg_outcome_prereq_published_lock
    BEFORE INSERT OR UPDATE OR DELETE ON curriculum_studio.outcome_prerequisites
    FOR EACH ROW EXECUTE FUNCTION curriculum_studio.forbid_published_plan_child_mutation();

CREATE OR REPLACE FUNCTION curriculum_studio.forbid_published_mapping_mutation()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    rev_status text;
    outcome uuid;
BEGIN
    IF TG_OP = 'DELETE' THEN
        outcome := OLD.outcome_id;
    ELSE
        outcome := NEW.outcome_id;
    END IF;

    SELECT r.status INTO rev_status
    FROM curriculum_studio.outcomes o
    JOIN curriculum_studio.plan_revisions r ON r.id = o.plan_revision_id
    WHERE o.id = outcome;

    IF rev_status IN ('published', 'superseded') THEN
        RAISE EXCEPTION 'cannot mutate mappings of immutable plan revision'
            USING ERRCODE = 'restrict_violation';
    END IF;

    IF TG_OP = 'DELETE' THEN
        RETURN OLD;
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_outcome_std_published_lock
    BEFORE INSERT OR UPDATE OR DELETE ON curriculum_studio.outcome_standard_mappings
    FOR EACH ROW EXECUTE FUNCTION curriculum_studio.forbid_published_mapping_mutation();

CREATE OR REPLACE FUNCTION curriculum_studio.forbid_published_join_mutation()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    rev_status text;
    owner uuid;
BEGIN
    IF TG_TABLE_NAME = 'unit_outcomes' THEN
        IF TG_OP = 'DELETE' THEN owner := OLD.unit_id; ELSE owner := NEW.unit_id; END IF;
        SELECT r.status INTO rev_status
        FROM curriculum_studio.units u
        JOIN curriculum_studio.plan_revisions r ON r.id = u.plan_revision_id
        WHERE u.id = owner;
    ELSE
        IF TG_OP = 'DELETE' THEN owner := OLD.project_id; ELSE owner := NEW.project_id; END IF;
        SELECT r.status INTO rev_status
        FROM curriculum_studio.projects p
        JOIN curriculum_studio.plan_revisions r ON r.id = p.plan_revision_id
        WHERE p.id = owner;
    END IF;

    IF rev_status IN ('published', 'superseded') THEN
        RAISE EXCEPTION 'cannot mutate membership of immutable plan revision'
            USING ERRCODE = 'restrict_violation';
    END IF;

    IF TG_OP = 'DELETE' THEN
        RETURN OLD;
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_unit_outcomes_published_lock
    BEFORE INSERT OR UPDATE OR DELETE ON curriculum_studio.unit_outcomes
    FOR EACH ROW EXECUTE FUNCTION curriculum_studio.forbid_published_join_mutation();
CREATE TRIGGER trg_project_outcomes_published_lock
    BEFORE INSERT OR UPDATE OR DELETE ON curriculum_studio.project_outcomes
    FOR EACH ROW EXECUTE FUNCTION curriculum_studio.forbid_published_join_mutation();

-- ---------------------------------------------------------------------------
-- Acyclic prerequisite DAGs (outcomes within a revision; catalog standards).
-- Recursive walk is practical for curriculum-scale graphs.
-- ---------------------------------------------------------------------------
CREATE OR REPLACE FUNCTION curriculum_studio.assert_outcome_prereq_acyclic()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF EXISTS (
        WITH RECURSIVE walk AS (
            SELECT NEW.prerequisite_id AS id
            UNION ALL
            SELECT p.prerequisite_id
            FROM curriculum_studio.outcome_prerequisites p
            JOIN walk w ON p.outcome_id = w.id
            WHERE p.plan_revision_id = NEW.plan_revision_id
        )
        SELECT 1 FROM walk WHERE id = NEW.outcome_id
    ) THEN
        RAISE EXCEPTION 'outcome prerequisite cycle detected for %', NEW.outcome_id
            USING ERRCODE = 'integrity_constraint_violation';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_outcome_prereq_acyclic
    BEFORE INSERT OR UPDATE ON curriculum_studio.outcome_prerequisites
    FOR EACH ROW EXECUTE FUNCTION curriculum_studio.assert_outcome_prereq_acyclic();

CREATE OR REPLACE FUNCTION curriculum_studio.assert_catalog_prereq_acyclic()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
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

CREATE TRIGGER trg_catalog_prereq_acyclic
    BEFORE INSERT OR UPDATE ON curriculum_studio.catalog_standard_prerequisites
    FOR EACH ROW EXECUTE FUNCTION curriculum_studio.assert_catalog_prereq_acyclic();

-- Same-revision membership for outcome prerequisites.
CREATE OR REPLACE FUNCTION curriculum_studio.assert_outcome_prereq_same_revision()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    out_rev uuid;
    pre_rev uuid;
BEGIN
    SELECT plan_revision_id INTO out_rev FROM curriculum_studio.outcomes WHERE id = NEW.outcome_id;
    SELECT plan_revision_id INTO pre_rev FROM curriculum_studio.outcomes WHERE id = NEW.prerequisite_id;
    IF out_rev IS DISTINCT FROM NEW.plan_revision_id
       OR pre_rev IS DISTINCT FROM NEW.plan_revision_id THEN
        RAISE EXCEPTION 'outcome prerequisites must stay inside plan revision %', NEW.plan_revision_id
            USING ERRCODE = 'integrity_constraint_violation';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_outcome_prereq_same_revision
    BEFORE INSERT OR UPDATE ON curriculum_studio.outcome_prerequisites
    FOR EACH ROW EXECUTE FUNCTION curriculum_studio.assert_outcome_prereq_same_revision();

-- ---------------------------------------------------------------------------
-- Locked materialized items cannot be overwritten or deleted.
-- Unlock is allowed (locked TRUE -> FALSE) so authors can resume editing.
-- ---------------------------------------------------------------------------
CREATE OR REPLACE FUNCTION curriculum_studio.protect_locked_materialized_item()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        IF OLD.locked THEN
            RAISE EXCEPTION 'locked materialized item % cannot be deleted', OLD.id
                USING ERRCODE = 'restrict_violation';
        END IF;
        RETURN OLD;
    END IF;

    IF OLD.locked THEN
        -- Permit unlock and bookkeeping of lock metadata; refuse content changes.
        IF NEW.locked = FALSE
           AND NEW.body = OLD.body
           AND NEW.title = OLD.title
           AND NEW.kind = OLD.kind
           AND NEW.plan_revision_id = OLD.plan_revision_id
           AND NEW.run_id = OLD.run_id
           AND NEW.provenance = OLD.provenance
        THEN
            RETURN NEW;
        END IF;
        IF NEW.locked = TRUE
           AND NEW.body = OLD.body
           AND NEW.title = OLD.title
           AND NEW.kind = OLD.kind
           AND NEW.status IS NOT DISTINCT FROM OLD.status
           AND NEW.supersedes_item_id IS NOT DISTINCT FROM OLD.supersedes_item_id
        THEN
            -- lock metadata refresh only
            RETURN NEW;
        END IF;
        RAISE EXCEPTION 'locked materialized item % cannot be overwritten', OLD.id
            USING ERRCODE = 'restrict_violation';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_materialized_items_locked
    BEFORE UPDATE OR DELETE ON curriculum_studio.materialized_items
    FOR EACH ROW EXECUTE FUNCTION curriculum_studio.protect_locked_materialized_item();

CREATE OR REPLACE FUNCTION curriculum_studio.forbid_locked_item_edits()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    is_locked boolean;
BEGIN
    SELECT locked INTO is_locked
    FROM curriculum_studio.materialized_items
    WHERE id = NEW.item_id;

    IF is_locked THEN
        RAISE EXCEPTION 'cannot edit locked materialized item %', NEW.item_id
            USING ERRCODE = 'restrict_violation';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_mat_item_edits_locked
    BEFORE INSERT ON curriculum_studio.materialized_item_edits
    FOR EACH ROW EXECUTE FUNCTION curriculum_studio.forbid_locked_item_edits();

-- ---------------------------------------------------------------------------
-- No assessment publication without a rubric or answer key.
-- ---------------------------------------------------------------------------
CREATE OR REPLACE FUNCTION curriculum_studio.assert_assessment_has_support()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF NEW.kind = 'assessment' AND NEW.status = 'published' THEN
        IF NOT EXISTS (
            SELECT 1
            FROM curriculum_studio.assessment_supports s
            JOIN curriculum_studio.materialized_items support
              ON support.id = s.support_item_id
            WHERE s.assessment_item_id = NEW.id
              AND support.kind IN ('rubric', 'answer_key')
              AND support.status IN ('ready', 'published')
        ) THEN
            RAISE EXCEPTION 'assessment % cannot be published without a rubric or answer key', NEW.id
                USING ERRCODE = 'integrity_constraint_violation';
        END IF;
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_assessment_requires_support
    BEFORE INSERT OR UPDATE OF kind, status ON curriculum_studio.materialized_items
    FOR EACH ROW EXECUTE FUNCTION curriculum_studio.assert_assessment_has_support();

CREATE OR REPLACE FUNCTION curriculum_studio.assert_support_kinds()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    assess_kind text;
    support_kind text;
BEGIN
    SELECT kind INTO assess_kind FROM curriculum_studio.materialized_items WHERE id = NEW.assessment_item_id;
    SELECT kind INTO support_kind FROM curriculum_studio.materialized_items WHERE id = NEW.support_item_id;
    IF assess_kind IS DISTINCT FROM 'assessment' THEN
        RAISE EXCEPTION 'assessment_supports.assessment_item_id must reference an assessment'
            USING ERRCODE = 'integrity_constraint_violation';
    END IF;
    IF support_kind NOT IN ('rubric', 'answer_key') THEN
        RAISE EXCEPTION 'assessment_supports.support_item_id must be a rubric or answer_key'
            USING ERRCODE = 'integrity_constraint_violation';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_assessment_supports_kinds
    BEFORE INSERT OR UPDATE ON curriculum_studio.assessment_supports
    FOR EACH ROW EXECUTE FUNCTION curriculum_studio.assert_support_kinds();

-- Materialized items must belong to the same plan revision as their run.
CREATE OR REPLACE FUNCTION curriculum_studio.assert_item_run_revision()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    run_rev uuid;
BEGIN
    SELECT plan_revision_id INTO run_rev
    FROM curriculum_studio.materialization_runs
    WHERE id = NEW.run_id;

    IF run_rev IS DISTINCT FROM NEW.plan_revision_id THEN
        RAISE EXCEPTION 'materialized item revision must match run revision'
            USING ERRCODE = 'integrity_constraint_violation';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_mat_item_run_revision
    BEFORE INSERT OR UPDATE OF run_id, plan_revision_id ON curriculum_studio.materialized_items
    FOR EACH ROW EXECUTE FUNCTION curriculum_studio.assert_item_run_revision();

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TRIGGER IF EXISTS trg_mat_item_run_revision ON curriculum_studio.materialized_items;
DROP TRIGGER IF EXISTS trg_assessment_supports_kinds ON curriculum_studio.assessment_supports;
DROP TRIGGER IF EXISTS trg_assessment_requires_support ON curriculum_studio.materialized_items;
DROP TRIGGER IF EXISTS trg_mat_item_edits_locked ON curriculum_studio.materialized_item_edits;
DROP TRIGGER IF EXISTS trg_materialized_items_locked ON curriculum_studio.materialized_items;
DROP TRIGGER IF EXISTS trg_outcome_prereq_same_revision ON curriculum_studio.outcome_prerequisites;
DROP TRIGGER IF EXISTS trg_catalog_prereq_acyclic ON curriculum_studio.catalog_standard_prerequisites;
DROP TRIGGER IF EXISTS trg_outcome_prereq_acyclic ON curriculum_studio.outcome_prerequisites;
DROP TRIGGER IF EXISTS trg_project_outcomes_published_lock ON curriculum_studio.project_outcomes;
DROP TRIGGER IF EXISTS trg_unit_outcomes_published_lock ON curriculum_studio.unit_outcomes;
DROP TRIGGER IF EXISTS trg_outcome_std_published_lock ON curriculum_studio.outcome_standard_mappings;
DROP TRIGGER IF EXISTS trg_outcome_prereq_published_lock ON curriculum_studio.outcome_prerequisites;
DROP TRIGGER IF EXISTS trg_plan_resources_published_lock ON curriculum_studio.plan_resources;
DROP TRIGGER IF EXISTS trg_sched_published_lock ON curriculum_studio.scheduling_constraints;
DROP TRIGGER IF EXISTS trg_evidence_published_lock ON curriculum_studio.evidence_requirements;
DROP TRIGGER IF EXISTS trg_projects_published_lock ON curriculum_studio.projects;
DROP TRIGGER IF EXISTS trg_units_published_lock ON curriculum_studio.units;
DROP TRIGGER IF EXISTS trg_arcs_published_lock ON curriculum_studio.learning_arcs;
DROP TRIGGER IF EXISTS trg_outcomes_published_lock ON curriculum_studio.outcomes;
DROP TRIGGER IF EXISTS trg_objectives_published_lock ON curriculum_studio.objectives;
DROP TRIGGER IF EXISTS trg_plan_revisions_immutable ON curriculum_studio.plan_revisions;

DROP FUNCTION IF EXISTS curriculum_studio.assert_item_run_revision();
DROP FUNCTION IF EXISTS curriculum_studio.assert_support_kinds();
DROP FUNCTION IF EXISTS curriculum_studio.assert_assessment_has_support();
DROP FUNCTION IF EXISTS curriculum_studio.forbid_locked_item_edits();
DROP FUNCTION IF EXISTS curriculum_studio.protect_locked_materialized_item();
DROP FUNCTION IF EXISTS curriculum_studio.assert_outcome_prereq_same_revision();
DROP FUNCTION IF EXISTS curriculum_studio.assert_catalog_prereq_acyclic();
DROP FUNCTION IF EXISTS curriculum_studio.assert_outcome_prereq_acyclic();
DROP FUNCTION IF EXISTS curriculum_studio.forbid_published_join_mutation();
DROP FUNCTION IF EXISTS curriculum_studio.forbid_published_mapping_mutation();
DROP FUNCTION IF EXISTS curriculum_studio.forbid_published_plan_child_mutation();
DROP FUNCTION IF EXISTS curriculum_studio.forbid_published_plan_mutation();
-- +goose StatementEnd
