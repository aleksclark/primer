-- Upgrade the initial effect ledger to immutable per-run invocation steps.
-- Existing rows are ordered deterministically before the new primary key is
-- installed; future retries cannot evade a boundary by changing input.
WITH ranked AS (
  SELECT ctid, row_number() OVER (PARTITION BY tenant_id, run_id ORDER BY created_at, tool_name, action_digest) AS ordinal
  FROM agent_tool_effects
)
UPDATE agent_tool_effects e
SET step = ranked.ordinal
FROM ranked
WHERE e.ctid = ranked.ctid;

ALTER TABLE agent_tool_effects DROP CONSTRAINT IF EXISTS agent_tool_effects_pkey;
ALTER TABLE agent_tool_effects ADD CONSTRAINT agent_tool_effects_pkey PRIMARY KEY (tenant_id, run_id, step);
ALTER TABLE agent_tool_effects ADD CONSTRAINT agent_tool_effects_tool_digest_key UNIQUE (tenant_id, run_id, tool_name, action_digest);
