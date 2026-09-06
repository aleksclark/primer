-- Original P4 00009 criteria evidence, adapted to retained/append-only policy.
-- Question text comes only from the immutable server-authorized issued plan.
ALTER TABLE dialogue_revision_policies ADD CONSTRAINT dialogue_revision_question_plan CHECK ((
 snapshot->>'questionPlanVersion'='dialogue.questions.v1' AND
 jsonb_typeof(snapshot->'questions')='array' AND jsonb_array_length(snapshot->'questions')=3) IS TRUE);
ALTER TABLE dialogue_attempts ADD CONSTRAINT dialogue_attempt_question_plan CHECK ((
 config_snapshot->>'questionPlanVersion'='dialogue.questions.v1' AND
 jsonb_typeof(config_snapshot->'questions')='array' AND jsonb_array_length(config_snapshot->'questions')=3) IS TRUE);
CREATE FUNCTION tasks_dialogue_question_plan_binding() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE planned jsonb;
BEGIN
 SELECT config_snapshot->'questions'->(NEW.ordinal-1) INTO planned FROM dialogue_attempts
  WHERE tenant_id=NEW.tenant_id AND attempt_id=NEW.attempt_id;
 IF (NEW.question_key=planned->>'key' AND NEW.prompt=planned->>'prompt') IS NOT TRUE THEN
  RAISE EXCEPTION 'unapproved dialogue question' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END;
$$;
CREATE TRIGGER dialogue_question_plan_binding BEFORE INSERT ON dialogue_questions
 FOR EACH ROW EXECUTE FUNCTION tasks_dialogue_question_plan_binding();

ALTER TABLE verification_evaluations ADD COLUMN criteria jsonb NOT NULL DEFAULT '[]'::jsonb
 CHECK (jsonb_typeof(criteria)='array' AND jsonb_array_length(criteria) <= 20
        AND (NOT accepted OR jsonb_array_length(criteria) > 0));

CREATE FUNCTION tasks_dialogue_evaluation_criteria() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE allowed jsonb;
BEGIN
 SELECT config_snapshot->'config'->'rubric' INTO allowed FROM dialogue_attempts
  WHERE tenant_id=NEW.tenant_id AND attempt_id=NEW.attempt_id;
 IF EXISTS (SELECT 1 FROM jsonb_array_elements(NEW.criteria) c WHERE jsonb_typeof(c)<>'string') OR
    NOT (allowed @> NEW.criteria) OR
    (SELECT count(*) FROM jsonb_array_elements(NEW.criteria)) <>
    (SELECT count(DISTINCT c) FROM jsonb_array_elements(NEW.criteria) c) THEN
  RAISE EXCEPTION 'unbound dialogue criteria' USING ERRCODE = '23514';
 END IF;
 RETURN NEW;
END;
$$;
CREATE TRIGGER dialogue_evaluation_criteria BEFORE INSERT ON verification_evaluations
 FOR EACH ROW EXECUTE FUNCTION tasks_dialogue_evaluation_criteria();

CREATE FUNCTION tasks_dialogue_override_binding() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NOT EXISTS (SELECT 1 FROM dialogue_attempts d WHERE d.tenant_id=NEW.tenant_id AND d.attempt_id=NEW.attempt_id
  AND d.occurrence_id=NEW.occurrence_id AND d.requirement_id=NEW.requirement_id) THEN
  RAISE EXCEPTION 'unbound dialogue override' USING ERRCODE = '23514';
 END IF;
 RETURN NEW;
END;
$$;
CREATE TRIGGER dialogue_override_binding BEFORE INSERT ON verification_overrides
 FOR EACH ROW EXECUTE FUNCTION tasks_dialogue_override_binding();

-- Retention is a real v1 invariant, not a UI promise. Mutable projections/jobs
-- remain separate from immutable source, message, question and decision rows.
CREATE FUNCTION tasks_dialogue_immutable_evidence() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 RAISE EXCEPTION 'dialogue evidence is retained and immutable' USING ERRCODE = '23514';
END;
$$;
CREATE TRIGGER dialogue_revision_policies_immutable BEFORE UPDATE OR DELETE ON dialogue_revision_policies
 FOR EACH ROW EXECUTE FUNCTION tasks_dialogue_immutable_evidence();
CREATE TRIGGER dialogue_policy_retained BEFORE DELETE ON dialogue_attempts
 FOR EACH ROW EXECUTE FUNCTION tasks_dialogue_immutable_evidence();
CREATE TRIGGER dialogue_questions_immutable BEFORE UPDATE OR DELETE ON dialogue_questions
 FOR EACH ROW EXECUTE FUNCTION tasks_dialogue_immutable_evidence();
CREATE TRIGGER verification_messages_immutable BEFORE UPDATE OR DELETE ON verification_messages
 FOR EACH ROW EXECUTE FUNCTION tasks_dialogue_immutable_evidence();
CREATE TRIGGER verification_evaluations_immutable BEFORE UPDATE OR DELETE ON verification_evaluations
 FOR EACH ROW EXECUTE FUNCTION tasks_dialogue_immutable_evidence();
CREATE TRIGGER verification_overrides_immutable BEFORE UPDATE OR DELETE ON verification_overrides
 FOR EACH ROW EXECUTE FUNCTION tasks_dialogue_immutable_evidence();
CREATE TRIGGER verification_events_immutable BEFORE UPDATE OR DELETE ON verification_events
 FOR EACH ROW EXECUTE FUNCTION tasks_dialogue_immutable_evidence();

-- Do not change the legacy/manual decision contract. Only decisions associated
-- with an incoming dialogue attempt receive this additional retention fence.
CREATE FUNCTION tasks_dialogue_decision_retained() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF EXISTS (SELECT 1 FROM dialogue_attempts WHERE tenant_id=OLD.tenant_id AND attempt_id=OLD.attempt_id) THEN
  RAISE EXCEPTION 'dialogue decision is retained and immutable' USING ERRCODE = '23514';
 END IF;
 IF TG_OP='DELETE' THEN RETURN OLD; END IF;
 RETURN NEW;
END;
$$;
CREATE TRIGGER dialogue_decision_retained BEFORE UPDATE OR DELETE ON verification_decisions
 FOR EACH ROW EXECUTE FUNCTION tasks_dialogue_decision_retained();

-- Ownership of an issued dialogue cannot change behind a bounded private
-- frame. Delivery locks student/session revocation rows, not mutable progress
-- rows, so slow network readers cannot block evaluation/decision commits.
CREATE FUNCTION tasks_dialogue_occurrence_binding_retained() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF (NEW.id,NEW.tenant_id,NEW.student_id,NEW.revision_id) IS DISTINCT FROM
    (OLD.id,OLD.tenant_id,OLD.student_id,OLD.revision_id) AND
    EXISTS(SELECT 1 FROM dialogue_attempts WHERE tenant_id=OLD.tenant_id AND occurrence_id=OLD.id) THEN
  RAISE EXCEPTION 'immutable dialogue occurrence binding' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END;
$$;
CREATE TRIGGER dialogue_occurrence_binding_retained BEFORE UPDATE ON task_occurrences
 FOR EACH ROW EXECUTE FUNCTION tasks_dialogue_occurrence_binding_retained();
