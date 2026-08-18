-- +goose Up
-- +goose StatementBegin

-- Serialize all writers touching the two outcome endpoints before walking the
-- graph. Without this lock, two concurrent reverse edges can each observe the
-- other's uncommitted graph as absent and commit a cycle under PostgreSQL MVCC.
CREATE OR REPLACE FUNCTION curriculum_studio.assert_outcome_prereq_acyclic_serialized()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    first_key bigint;
    second_key bigint;
BEGIN
    first_key := hashtextextended(LEAST(NEW.outcome_id::text, NEW.prerequisite_id::text), 0);
    second_key := hashtextextended(GREATEST(NEW.outcome_id::text, NEW.prerequisite_id::text), 0);
    PERFORM pg_advisory_xact_lock(first_key);
    IF second_key <> first_key THEN
        PERFORM pg_advisory_xact_lock(second_key);
    END IF;

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

DROP TRIGGER IF EXISTS trg_outcome_prereq_acyclic
    ON curriculum_studio.outcome_prerequisites;
CREATE TRIGGER trg_outcome_prereq_acyclic
    BEFORE INSERT OR UPDATE ON curriculum_studio.outcome_prerequisites
    FOR EACH ROW EXECUTE FUNCTION curriculum_studio.assert_outcome_prereq_acyclic_serialized();

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TRIGGER IF EXISTS trg_outcome_prereq_acyclic
    ON curriculum_studio.outcome_prerequisites;
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
DROP FUNCTION IF EXISTS curriculum_studio.assert_outcome_prereq_acyclic_serialized();
-- +goose StatementEnd
