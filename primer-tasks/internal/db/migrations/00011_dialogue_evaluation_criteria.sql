-- Original P4 00009 criteria evidence, adapted to retained/append-only policy.
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
