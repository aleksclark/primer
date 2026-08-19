-- Contract for the Phase 3 runtime repositories. This is deliberately not a
-- migration: the Tasks migration owner must integrate it with the product's
-- migration numbering and foundation tenant foreign keys.
CREATE TABLE IF NOT EXISTS agent_conversations (
 id uuid PRIMARY KEY, tenant_id uuid NOT NULL, actor_id uuid NOT NULL,
 status text NOT NULL CHECK (status IN ('active','archived')),
 policy_version text NOT NULL, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS agent_conversations_scope ON agent_conversations(tenant_id,updated_at DESC);
CREATE TABLE IF NOT EXISTS agent_messages (
 id uuid PRIMARY KEY, tenant_id uuid NOT NULL, conversation_id uuid NOT NULL,
 client_message_id text NOT NULL, role text NOT NULL CHECK(role IN ('user','assistant','system')),
 content text NOT NULL, sequence bigint NOT NULL, created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(tenant_id,conversation_id,client_message_id), UNIQUE(tenant_id,conversation_id,sequence)
);
CREATE TABLE IF NOT EXISTS agent_runs (
 id uuid PRIMARY KEY, tenant_id uuid NOT NULL, conversation_id uuid NOT NULL, user_message_id uuid NOT NULL,
 status text NOT NULL CHECK(status IN ('queued','running','cancel_requested','succeeded','failed','canceled')),
 durable_step integer NOT NULL DEFAULT 0, attempt integer NOT NULL DEFAULT 0,
 max_steps integer NOT NULL, max_tokens integer NOT NULL, deadline timestamptz NOT NULL,
 lease_owner text, lease_until timestamptz, cancel_requested boolean NOT NULL DEFAULT false,
 input_tokens bigint NOT NULL DEFAULT 0, output_tokens bigint NOT NULL DEFAULT 0,
 total_tokens bigint NOT NULL DEFAULT 0, reasoning_tokens bigint NOT NULL DEFAULT 0,
 provider text NOT NULL, model text NOT NULL, policy_version text NOT NULL, prompt_digest text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS agent_runs_claim ON agent_runs(status,lease_until,created_at);
CREATE TABLE IF NOT EXISTS agent_jobs (
 id uuid PRIMARY KEY, tenant_id uuid NOT NULL, run_id uuid NOT NULL,
 status text NOT NULL CHECK(status IN ('queued','running','done','failed')),
 attempts integer NOT NULL DEFAULT 0, max_attempts integer NOT NULL,
 available_at timestamptz NOT NULL DEFAULT now(), lease_owner text, lease_until timestamptz,
 last_error text, updated_at timestamptz NOT NULL DEFAULT now(), UNIQUE(run_id)
);
CREATE INDEX IF NOT EXISTS agent_jobs_claim ON agent_jobs(status,available_at,lease_until);
CREATE TABLE IF NOT EXISTS agent_run_events (
 run_id uuid NOT NULL, tenant_id uuid NOT NULL, sequence bigint NOT NULL,
 event_type text NOT NULL, payload jsonb NOT NULL, created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(run_id,sequence)
);
CREATE TABLE IF NOT EXISTS agent_confirmation_previews (
 id uuid PRIMARY KEY, tenant_id uuid NOT NULL, actor_id uuid NOT NULL,
 action text NOT NULL, action_digest text NOT NULL, payload jsonb NOT NULL,
 expires_at timestamptz NOT NULL, used_at timestamptz
);
CREATE INDEX IF NOT EXISTS agent_events_replay ON agent_run_events(tenant_id,run_id,sequence);
CREATE TABLE IF NOT EXISTS agent_tool_effects (
 tenant_id uuid NOT NULL, run_id uuid NOT NULL, step integer NOT NULL CHECK(step > 0),
 tool_name text NOT NULL, action_digest text NOT NULL, status text NOT NULL CHECK(status IN ('reserved','applied')),
 result jsonb NOT NULL DEFAULT '{}'::jsonb, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(tenant_id,run_id,tool_name,action_digest)
);
