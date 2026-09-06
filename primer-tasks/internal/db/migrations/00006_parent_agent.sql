-- Phase 3 durable parent agent state.  These tables are the source of truth for
-- conversations, replay, jobs, leases, usage/provenance, and confirmations;
-- no socket or process-local map may substitute for them.
CREATE TABLE IF NOT EXISTS agent_conversations (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  actor_id text NOT NULL,
  status text NOT NULL CHECK (status IN ('active','archived')),
  policy_version text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, id)
);

CREATE TABLE IF NOT EXISTS agent_messages (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  conversation_id uuid NOT NULL,
  client_message_id text NOT NULL,
  role text NOT NULL CHECK (role IN ('user','assistant','system')),
  content text NOT NULL,
  sequence bigint NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT agent_messages_conversation_tenant_fk
    FOREIGN KEY (tenant_id, conversation_id)
    REFERENCES agent_conversations(tenant_id, id),
  UNIQUE (tenant_id, id),
  UNIQUE (tenant_id, conversation_id, client_message_id),
  UNIQUE (tenant_id, conversation_id, sequence)
);
CREATE INDEX IF NOT EXISTS agent_messages_replay_idx
  ON agent_messages (tenant_id, conversation_id, sequence);

CREATE TABLE IF NOT EXISTS agent_runs (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  conversation_id uuid NOT NULL,
  user_message_id uuid NOT NULL,
  status text NOT NULL CHECK (status IN ('queued','running','cancel_requested','awaiting_confirmation','succeeded','failed','canceled')),
  durable_step integer NOT NULL DEFAULT 0 CHECK (durable_step >= 0),
  attempt integer NOT NULL DEFAULT 0 CHECK (attempt >= 0),
  max_steps integer NOT NULL CHECK (max_steps BETWEEN 1 AND 100),
  max_tokens integer NOT NULL CHECK (max_tokens BETWEEN 1 AND 200000),
  deadline timestamptz NOT NULL,
  lease_owner text,
  lease_until timestamptz,
  cancel_requested boolean NOT NULL DEFAULT false,
  input_tokens bigint NOT NULL DEFAULT 0,
  output_tokens bigint NOT NULL DEFAULT 0,
  total_tokens bigint NOT NULL DEFAULT 0,
  reasoning_tokens bigint NOT NULL DEFAULT 0,
  provider text NOT NULL DEFAULT '',
  model text NOT NULL DEFAULT '',
  policy_version text NOT NULL DEFAULT '',
  prompt_digest text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT agent_runs_conversation_tenant_fk
    FOREIGN KEY (tenant_id, conversation_id)
    REFERENCES agent_conversations(tenant_id, id),
  CONSTRAINT agent_runs_message_tenant_fk
    FOREIGN KEY (tenant_id, user_message_id)
    REFERENCES agent_messages(tenant_id, id),
  UNIQUE (tenant_id, id)
);
CREATE INDEX IF NOT EXISTS agent_runs_lease_idx
  ON agent_runs (tenant_id, status, lease_until, created_at);

CREATE TABLE IF NOT EXISTS agent_jobs (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  run_id uuid NOT NULL,
  kind text NOT NULL CHECK (kind IN ('agent_run','reconcile')),
  status text NOT NULL CHECK (status IN ('queued','running','done','failed','canceled')),
  attempts integer NOT NULL DEFAULT 0,
  max_attempts integer NOT NULL CHECK (max_attempts BETWEEN 1 AND 10),
  lease_owner text,
  lease_until timestamptz,
  available_at timestamptz NOT NULL DEFAULT now(),
  last_error text,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT agent_jobs_run_tenant_fk FOREIGN KEY (tenant_id, run_id)
    REFERENCES agent_runs(tenant_id, id),
  UNIQUE (tenant_id, run_id, kind)
);
CREATE INDEX IF NOT EXISTS agent_jobs_claim_idx
  ON agent_jobs (status, available_at, lease_until);

CREATE TABLE IF NOT EXISTS agent_run_events (
  run_id uuid NOT NULL,
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  sequence bigint NOT NULL CHECK (sequence > 0),
  event_type text NOT NULL,
  payload jsonb NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (run_id, sequence),
  CONSTRAINT agent_events_run_tenant_fk FOREIGN KEY (tenant_id, run_id)
    REFERENCES agent_runs(tenant_id, id)
);
CREATE INDEX IF NOT EXISTS agent_run_events_replay_idx
  ON agent_run_events (tenant_id, run_id, sequence);

CREATE TABLE IF NOT EXISTS agent_confirmation_previews (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  actor_id text NOT NULL,
  action text NOT NULL,
  action_digest text NOT NULL CHECK (length(action_digest) = 64),
  payload jsonb NOT NULL,
  expires_at timestamptz NOT NULL,
  used_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  CHECK (expires_at > created_at AND expires_at <= created_at + interval '15 minutes')
);
CREATE INDEX IF NOT EXISTS agent_confirmation_expiry_idx
  ON agent_confirmation_previews (expires_at) WHERE used_at IS NULL;

CREATE TABLE IF NOT EXISTS parent_confirmation_previews (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  actor_id text NOT NULL,
  handle_hash bytea NOT NULL UNIQUE,
  action_kind text NOT NULL,
  action_digest bytea NOT NULL CHECK (octet_length(action_digest) = 32),
  action jsonb NOT NULL,
  summary text NOT NULL DEFAULT '',
  expires_at timestamptz NOT NULL,
  consumed_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  CHECK (expires_at > created_at AND expires_at <= created_at + interval '15 minutes')
);
CREATE INDEX IF NOT EXISTS parent_confirmation_expiry_idx
  ON parent_confirmation_previews (expires_at) WHERE consumed_at IS NULL;
