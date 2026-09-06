-- Durable mutation boundaries for parent-agent tool retries. A reserved row
-- prevents a restarted worker from repeating a committed effect; applied rows
-- replay the sanitized domain result.
CREATE TABLE IF NOT EXISTS agent_tool_effects (
  tenant_id uuid NOT NULL,
  run_id uuid NOT NULL,
  step integer NOT NULL CHECK (step > 0),
  tool_name text NOT NULL,
  action_digest text NOT NULL CHECK (length(action_digest) = 64),
  status text NOT NULL CHECK (status IN ('reserved','applied')),
  result jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, run_id, tool_name, action_digest),
  CONSTRAINT agent_tool_effects_run_fk FOREIGN KEY (tenant_id, run_id)
    REFERENCES agent_runs(tenant_id, id)
);
CREATE INDEX IF NOT EXISTS agent_tool_effects_run_idx
  ON agent_tool_effects (tenant_id, run_id, step);
