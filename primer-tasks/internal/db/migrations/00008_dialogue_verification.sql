-- Phase 4: generic verification evidence plus the minimal ordered dialogue projection.
-- All rows remain tenant-scoped and point back to the Phase 2 attempt; the
-- projection never owns a completion transition.
CREATE TABLE IF NOT EXISTS verification_messages (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL,
  attempt_id uuid NOT NULL,
  sequence bigint NOT NULL,
  role text NOT NULL CHECK (role IN ('student','agent','system')),
  content text NOT NULL,
  client_message_id text,
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, id),
  UNIQUE (tenant_id, attempt_id, sequence),
  UNIQUE (tenant_id, attempt_id, client_message_id),
  FOREIGN KEY (tenant_id, attempt_id) REFERENCES verification_attempts(tenant_id, id)
);
CREATE INDEX IF NOT EXISTS verification_messages_replay
  ON verification_messages(tenant_id, attempt_id, sequence);

CREATE TABLE IF NOT EXISTS dialogue_attempts (
  tenant_id uuid NOT NULL,
  attempt_id uuid NOT NULL,
  occurrence_id uuid NOT NULL,
  requirement_id uuid NOT NULL,
  policy_version text NOT NULL,
  config_snapshot jsonb NOT NULL,
  accepted_count integer NOT NULL DEFAULT 0 CHECK (accepted_count >= 0),
  turn_count integer NOT NULL DEFAULT 0 CHECK (turn_count >= 0),
  next_sequence bigint NOT NULL DEFAULT 1 CHECK (next_sequence > 0),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, attempt_id),
  UNIQUE (tenant_id, occurrence_id, requirement_id, attempt_id),
  FOREIGN KEY (tenant_id, attempt_id) REFERENCES verification_attempts(tenant_id, id),
  FOREIGN KEY (tenant_id, occurrence_id) REFERENCES task_occurrences(tenant_id, id),
  FOREIGN KEY (tenant_id, requirement_id) REFERENCES verification_requirements(tenant_id, id)
);

CREATE TABLE IF NOT EXISTS dialogue_questions (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL,
  attempt_id uuid NOT NULL,
  question_key text NOT NULL,
  ordinal integer NOT NULL CHECK (ordinal > 0),
  prompt text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, id),
  UNIQUE (tenant_id, attempt_id, question_key),
  UNIQUE (tenant_id, attempt_id, ordinal),
  FOREIGN KEY (tenant_id, attempt_id) REFERENCES verification_attempts(tenant_id, id)
);

CREATE TABLE IF NOT EXISTS verification_evaluations (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL,
  attempt_id uuid NOT NULL,
  question_id uuid NOT NULL,
  message_id uuid NOT NULL,
  accepted boolean NOT NULL,
  rationale text NOT NULL DEFAULT '',
  provider text NOT NULL DEFAULT '',
  model text NOT NULL DEFAULT '',
  policy_version text NOT NULL,
  usage jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, id),
  UNIQUE (tenant_id, attempt_id, question_id, message_id),
  FOREIGN KEY (tenant_id, attempt_id) REFERENCES verification_attempts(tenant_id, id),
  FOREIGN KEY (tenant_id, question_id) REFERENCES dialogue_questions(tenant_id, id),
  FOREIGN KEY (tenant_id, message_id) REFERENCES verification_messages(tenant_id, id)
);
CREATE INDEX IF NOT EXISTS verification_evaluations_attempt
  ON verification_evaluations(tenant_id, attempt_id, created_at);

CREATE TABLE IF NOT EXISTS verification_overrides (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL,
  occurrence_id uuid NOT NULL,
  requirement_id uuid NOT NULL,
  attempt_id uuid,
  accepted boolean NOT NULL,
  reason text NOT NULL,
  actor_id text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  FOREIGN KEY (tenant_id, occurrence_id) REFERENCES task_occurrences(tenant_id, id),
  FOREIGN KEY (tenant_id, requirement_id) REFERENCES verification_requirements(tenant_id, id),
  FOREIGN KEY (tenant_id, attempt_id) REFERENCES verification_attempts(tenant_id, id)
);
CREATE INDEX IF NOT EXISTS verification_overrides_scope
  ON verification_overrides(tenant_id, occurrence_id, created_at);

CREATE TABLE IF NOT EXISTS verification_jobs (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL,
  attempt_id uuid NOT NULL,
  message_id uuid,
  status text NOT NULL CHECK (status IN ('queued','running','succeeded','failed')) DEFAULT 'queued',
  attempts integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
  max_attempts integer NOT NULL CHECK (max_attempts > 0),
  available_at timestamptz NOT NULL DEFAULT now(),
  lease_owner text,
  lease_until timestamptz,
  last_error text,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, message_id),
  FOREIGN KEY (tenant_id, attempt_id) REFERENCES verification_attempts(tenant_id, id),
  FOREIGN KEY (tenant_id, message_id) REFERENCES verification_messages(tenant_id, id)
);
CREATE INDEX IF NOT EXISTS verification_jobs_claim
  ON verification_jobs(status, available_at, lease_until);

CREATE TABLE IF NOT EXISTS verification_events (
  tenant_id uuid NOT NULL,
  attempt_id uuid NOT NULL,
  sequence bigint NOT NULL,
  kind text NOT NULL,
  payload jsonb NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, attempt_id, sequence),
  FOREIGN KEY (tenant_id, attempt_id) REFERENCES verification_attempts(tenant_id, id)
);
CREATE INDEX IF NOT EXISTS verification_events_replay
  ON verification_events(tenant_id, attempt_id, sequence);
