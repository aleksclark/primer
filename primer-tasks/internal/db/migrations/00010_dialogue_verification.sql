-- Original P4 00008 dialogue projection, reconciled AFTER canonical P3 00009.
-- Branch-local allocation: do not push/merge before coordinated allocation.
-- Existing requirement-array storage and all canonical migration bytes remain.

-- Publish-time source resolution is durable even if a student starts later.
-- This is additive storage; the canonical requirement config array is retained.
CREATE TABLE dialogue_revision_policies (
 tenant_id uuid NOT NULL,
 revision_id uuid NOT NULL,
 requirement_id uuid NOT NULL,
 snapshot_digest text NOT NULL CHECK (snapshot_digest ~ '^[a-f0-9]{64}$'),
 snapshot jsonb NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY (tenant_id,requirement_id),
 UNIQUE (tenant_id,revision_id,requirement_id,snapshot_digest),
 FOREIGN KEY (tenant_id,revision_id) REFERENCES task_revisions(tenant_id,id),
 FOREIGN KEY (tenant_id,requirement_id) REFERENCES verification_requirements(tenant_id,id),
 CHECK ((snapshot->>'revisionId'=revision_id::text) IS TRUE),
 CHECK ((snapshot->>'requirementId'=requirement_id::text) IS TRUE),
 CHECK ((snapshot->>'digest'=snapshot_digest) IS TRUE),
 CHECK ((snapshot->>'policyVersion'='dialogue.v1') IS TRUE),
 CHECK ((snapshot->'config'->>'retentionPolicy'='retain') IS TRUE),
 CHECK ((snapshot->'config'->>'requiredQuestions'='3') IS TRUE)
);

CREATE FUNCTION tasks_dialogue_revision_binding() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NOT EXISTS (SELECT 1 FROM verification_requirements r JOIN task_revisions v
  ON v.tenant_id=r.tenant_id AND v.id=r.revision_id
  WHERE r.tenant_id=NEW.tenant_id AND r.id=NEW.requirement_id AND r.revision_id=NEW.revision_id
   AND r.kind='agent_dialogue' AND r.config_version=1 AND r.interaction='chat' AND r.executor='fantasy'
   AND v.status='published' AND (NEW.snapshot->>'revisionVersion')::integer=v.version
   AND NEW.snapshot->'config'=r.config->0->'config') THEN
  RAISE EXCEPTION 'invalid published dialogue policy binding' USING ERRCODE = '23514';
 END IF;
 RETURN NEW;
END;
$$;
CREATE TRIGGER dialogue_revision_binding BEFORE INSERT ON dialogue_revision_policies
 FOR EACH ROW EXECUTE FUNCTION tasks_dialogue_revision_binding();

CREATE TABLE dialogue_attempts (
 tenant_id uuid NOT NULL,
 attempt_id uuid NOT NULL,
 occurrence_id uuid NOT NULL,
 requirement_id uuid NOT NULL,
 student_id uuid NOT NULL,
 revision_id uuid NOT NULL,
 policy_version text NOT NULL CHECK (policy_version = 'dialogue.v1'),
 snapshot_digest text NOT NULL CHECK (snapshot_digest ~ '^[a-f0-9]{64}$'),
 config_snapshot jsonb NOT NULL,
 version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
 next_sequence bigint NOT NULL DEFAULT 1 CHECK (next_sequence > 0),
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY (tenant_id,attempt_id),
 UNIQUE (tenant_id,attempt_id,policy_version,snapshot_digest),
 FOREIGN KEY (tenant_id,attempt_id) REFERENCES verification_attempts(tenant_id,id),
 FOREIGN KEY (tenant_id,occurrence_id) REFERENCES task_occurrences(tenant_id,id),
 FOREIGN KEY (tenant_id,requirement_id) REFERENCES verification_requirements(tenant_id,id),
 FOREIGN KEY (tenant_id,student_id) REFERENCES students(tenant_id,id),
 FOREIGN KEY (tenant_id,revision_id) REFERENCES task_revisions(tenant_id,id),
 FOREIGN KEY (tenant_id,revision_id,requirement_id,snapshot_digest)
  REFERENCES dialogue_revision_policies(tenant_id,revision_id,requirement_id,snapshot_digest),
 CHECK (jsonb_typeof(config_snapshot) = 'object'),
 CHECK ((config_snapshot->>'revisionId' = revision_id::text) IS TRUE),
 CHECK ((config_snapshot->>'requirementId' = requirement_id::text) IS TRUE),
 CHECK ((config_snapshot->>'policyVersion' = policy_version) IS TRUE),
 CHECK ((config_snapshot->>'digest' = snapshot_digest) IS TRUE),
 CHECK ((config_snapshot->'config'->>'retentionPolicy' = 'retain') IS TRUE),
 CHECK ((config_snapshot->'config'->>'requiredQuestions' = '3') IS TRUE)
);

-- Existing generic tables have individual tenant FKs. Verify that this new
-- projection binds the SAME attempt/occurrence/student/issued revision and
-- requirement, not independent existing IDs in one tenant.
CREATE FUNCTION tasks_dialogue_attempt_binding() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP = 'UPDATE' THEN
  IF (to_jsonb(NEW) - ARRAY['version','next_sequence','updated_at']) IS DISTINCT FROM
     (to_jsonb(OLD) - ARRAY['version','next_sequence','updated_at']) OR
     NEW.version < OLD.version OR NEW.next_sequence < OLD.next_sequence THEN
   RAISE EXCEPTION 'immutable dialogue policy binding' USING ERRCODE = '23514';
  END IF;
  RETURN NEW;
 END IF;
 IF NOT EXISTS (
  SELECT 1 FROM verification_attempts a
  JOIN task_occurrences o ON o.tenant_id=a.tenant_id AND o.id=a.occurrence_id
  JOIN verification_requirements r ON r.tenant_id=a.tenant_id AND r.id=a.requirement_id
  JOIN task_revisions v ON v.tenant_id=r.tenant_id AND v.id=r.revision_id
  WHERE a.tenant_id=NEW.tenant_id AND a.id=NEW.attempt_id
   AND a.occurrence_id=NEW.occurrence_id AND a.requirement_id=NEW.requirement_id
   AND o.student_id=NEW.student_id AND o.revision_id=NEW.revision_id
   AND r.revision_id=o.revision_id AND r.kind='agent_dialogue' AND r.config_version=1
   AND r.interaction='chat' AND r.executor='fantasy' AND v.status='published'
   AND (NEW.config_snapshot->>'revisionVersion')::integer=v.version
   AND NEW.config_snapshot->'config'=r.config->0->'config'
   AND EXISTS (SELECT 1 FROM dialogue_revision_policies p WHERE p.tenant_id=NEW.tenant_id
    AND p.requirement_id=NEW.requirement_id AND p.snapshot=NEW.config_snapshot)
 ) THEN
  RAISE EXCEPTION 'invalid dialogue attempt binding' USING ERRCODE = '23514';
 END IF;
 RETURN NEW;
END;
$$;
CREATE TRIGGER dialogue_attempt_binding BEFORE INSERT OR UPDATE ON dialogue_attempts
 FOR EACH ROW EXECUTE FUNCTION tasks_dialogue_attempt_binding();

CREATE TABLE dialogue_questions (
 id uuid PRIMARY KEY,
 tenant_id uuid NOT NULL,
 attempt_id uuid NOT NULL,
 question_key text NOT NULL CHECK (length(question_key) BETWEEN 1 AND 128),
 ordinal integer NOT NULL CHECK (ordinal BETWEEN 1 AND 3),
 version bigint NOT NULL CHECK (version > 1),
 prompt text NOT NULL CHECK (length(btrim(prompt)) BETWEEN 1 AND 2000),
 created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE (tenant_id,attempt_id,id),
 UNIQUE (tenant_id,attempt_id,question_key),
 UNIQUE (tenant_id,attempt_id,ordinal),
 UNIQUE (tenant_id,attempt_id,version),
 FOREIGN KEY (tenant_id,attempt_id) REFERENCES dialogue_attempts(tenant_id,attempt_id)
);
CREATE UNIQUE INDEX dialogue_question_distinct_prompt ON dialogue_questions
 (tenant_id,attempt_id,lower(btrim(prompt)));

CREATE TABLE verification_messages (
 id uuid PRIMARY KEY,
 tenant_id uuid NOT NULL,
 attempt_id uuid NOT NULL,
 question_id uuid NOT NULL,
 policy_version text NOT NULL,
 snapshot_digest text NOT NULL,
 sequence bigint NOT NULL CHECK (sequence > 0),
 expected_version bigint NOT NULL CHECK (expected_version > 0),
 role text NOT NULL CHECK (role IN ('student','agent','system')),
 content text NOT NULL CHECK (length(btrim(content)) > 0 AND octet_length(content) <= 12000),
 client_message_id text NOT NULL CHECK (length(client_message_id) BETWEEN 1 AND 128),
 created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE (tenant_id,attempt_id,id),
 UNIQUE (tenant_id,attempt_id,id,question_id,role),
 UNIQUE (tenant_id,attempt_id,sequence),
 UNIQUE (tenant_id,attempt_id,client_message_id),
 FOREIGN KEY (tenant_id,attempt_id,question_id) REFERENCES dialogue_questions(tenant_id,attempt_id,id),
 FOREIGN KEY (tenant_id,attempt_id,policy_version,snapshot_digest)
  REFERENCES dialogue_attempts(tenant_id,attempt_id,policy_version,snapshot_digest)
);

CREATE TABLE verification_jobs (
 id uuid PRIMARY KEY,
 tenant_id uuid NOT NULL,
 attempt_id uuid NOT NULL,
 message_id uuid,
 session_id uuid NOT NULL REFERENCES student_sessions(id),
 job_key text NOT NULL CHECK (length(job_key) BETWEEN 1 AND 128),
 status text NOT NULL DEFAULT 'queued' CHECK (status IN ('queued','running','succeeded','failed')),
 stage text NOT NULL DEFAULT 'question' CHECK (stage IN ('question','evaluation','publication','done')),
 attempts integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
 max_attempts integer NOT NULL CHECK (max_attempts BETWEEN 1 AND 10),
 lease_generation bigint NOT NULL DEFAULT 0 CHECK (lease_generation >= 0),
 lease_owner text,
 lease_until timestamptz,
 available_at timestamptz NOT NULL DEFAULT now(),
 deadline timestamptz NOT NULL,
 last_error text CHECK (last_error IN ('provider_unavailable','lease_lost','revoked','exhausted','invalid_evaluation')),
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE (tenant_id,attempt_id,id),
 UNIQUE (tenant_id,attempt_id,id,message_id),
 UNIQUE (tenant_id,attempt_id,job_key),
 UNIQUE (tenant_id,attempt_id,message_id),
 CHECK (attempts <= max_attempts),
 CHECK ((status='running' AND lease_owner IS NOT NULL AND lease_until IS NOT NULL) OR
        (status<>'running' AND lease_owner IS NULL AND lease_until IS NULL)),
 FOREIGN KEY (tenant_id,attempt_id) REFERENCES dialogue_attempts(tenant_id,attempt_id),
 FOREIGN KEY (tenant_id,attempt_id,message_id) REFERENCES verification_messages(tenant_id,attempt_id,id)
);
CREATE UNIQUE INDEX verification_jobs_one_active ON verification_jobs(tenant_id,attempt_id)
 WHERE status IN ('queued','running');
CREATE INDEX verification_jobs_claim ON verification_jobs(status,available_at,lease_until);

CREATE FUNCTION tasks_dialogue_job_binding() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NOT EXISTS (
  SELECT 1 FROM dialogue_attempts d JOIN student_sessions s
   ON s.tenant_id=d.tenant_id AND s.student_id=d.student_id
  WHERE d.tenant_id=NEW.tenant_id AND d.attempt_id=NEW.attempt_id AND s.id=NEW.session_id
 ) THEN
  RAISE EXCEPTION 'invalid dialogue session binding' USING ERRCODE = '23514';
 END IF;
 IF TG_OP='UPDATE' AND (
  (NEW.id,NEW.tenant_id,NEW.attempt_id,NEW.message_id,NEW.session_id,NEW.job_key,NEW.max_attempts,NEW.deadline,NEW.created_at)
   IS DISTINCT FROM
  (OLD.id,OLD.tenant_id,OLD.attempt_id,OLD.message_id,OLD.session_id,OLD.job_key,OLD.max_attempts,OLD.deadline,OLD.created_at)
  OR NEW.attempts < OLD.attempts OR NEW.lease_generation < OLD.lease_generation
 ) THEN
  RAISE EXCEPTION 'immutable dialogue job admission' USING ERRCODE = '23514';
 END IF;
 RETURN NEW;
END;
$$;
CREATE TRIGGER dialogue_job_binding BEFORE INSERT OR UPDATE ON verification_jobs
 FOR EACH ROW EXECUTE FUNCTION tasks_dialogue_job_binding();

CREATE TABLE verification_evaluations (
 id uuid PRIMARY KEY,
 tenant_id uuid NOT NULL,
 attempt_id uuid NOT NULL,
 question_id uuid NOT NULL,
 message_id uuid NOT NULL,
 message_role text NOT NULL DEFAULT 'student' CHECK (message_role='student'),
 job_id uuid NOT NULL,
 policy_version text NOT NULL,
 snapshot_digest text NOT NULL,
 version bigint NOT NULL CHECK (version > 1),
 accepted boolean NOT NULL,
 rationale text NOT NULL,
 provider text NOT NULL CHECK (length(provider) BETWEEN 1 AND 128),
 model text NOT NULL CHECK (length(model) BETWEEN 1 AND 128),
 input_tokens bigint NOT NULL DEFAULT 0 CHECK (input_tokens >= 0),
 output_tokens bigint NOT NULL DEFAULT 0 CHECK (output_tokens >= 0),
 created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE (tenant_id,attempt_id,id),
 UNIQUE (tenant_id,attempt_id,message_id),
 CHECK ((accepted AND rationale='Answer addresses the current question using the assigned source.') OR
        (NOT accepted AND rationale='Add a specific detail or reason from the assigned source.')),
 FOREIGN KEY (tenant_id,attempt_id,question_id) REFERENCES dialogue_questions(tenant_id,attempt_id,id),
 FOREIGN KEY (tenant_id,attempt_id,message_id,question_id,message_role)
  REFERENCES verification_messages(tenant_id,attempt_id,id,question_id,role),
 FOREIGN KEY (tenant_id,attempt_id,job_id,message_id) REFERENCES verification_jobs(tenant_id,attempt_id,id,message_id),
 FOREIGN KEY (tenant_id,attempt_id,policy_version,snapshot_digest)
  REFERENCES dialogue_attempts(tenant_id,attempt_id,policy_version,snapshot_digest)
);
CREATE UNIQUE INDEX verification_evaluations_distinct_accepted_question
 ON verification_evaluations(tenant_id,attempt_id,question_id) WHERE accepted;

CREATE TABLE verification_overrides (
 id uuid PRIMARY KEY,
 tenant_id uuid NOT NULL,
 occurrence_id uuid NOT NULL,
 requirement_id uuid NOT NULL,
 attempt_id uuid NOT NULL,
 client_request_id text NOT NULL CHECK (length(client_request_id) BETWEEN 1 AND 128),
 expected_version bigint NOT NULL CHECK (expected_version > 0),
 result_version bigint NOT NULL CHECK (result_version = expected_version + 1),
 result_status text NOT NULL CHECK (result_status IN ('pending','awaiting_verification','completed')),
 accepted boolean NOT NULL,
 reason text NOT NULL CHECK (length(btrim(reason)) BETWEEN 1 AND 1000),
 actor_id text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE (tenant_id,occurrence_id,client_request_id),
 UNIQUE (tenant_id,attempt_id,result_version),
 FOREIGN KEY (tenant_id,attempt_id) REFERENCES dialogue_attempts(tenant_id,attempt_id),
 FOREIGN KEY (tenant_id,occurrence_id) REFERENCES task_occurrences(tenant_id,id),
 FOREIGN KEY (tenant_id,requirement_id) REFERENCES verification_requirements(tenant_id,id),
 FOREIGN KEY (tenant_id,actor_id) REFERENCES parent_memberships(tenant_id,subject_ref)
);

CREATE TABLE verification_events (
 tenant_id uuid NOT NULL,
 attempt_id uuid NOT NULL,
 sequence bigint NOT NULL CHECK (sequence > 0),
 event_key text NOT NULL CHECK (length(event_key) BETWEEN 1 AND 192),
 kind text NOT NULL CHECK (kind IN ('message_ack','question','progress','answer_evaluation','complete','error','override')),
 payload jsonb NOT NULL CHECK (jsonb_typeof(payload)='object' AND octet_length(payload::text) <= 32768),
 created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY (tenant_id,attempt_id,sequence),
 UNIQUE (tenant_id,attempt_id,event_key),
 FOREIGN KEY (tenant_id,attempt_id) REFERENCES dialogue_attempts(tenant_id,attempt_id)
);
CREATE UNIQUE INDEX verification_events_one_completion ON verification_events(tenant_id,attempt_id) WHERE kind='complete';
