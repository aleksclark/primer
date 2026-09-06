-- Incoming P3 hardening, after the immutable applied Clerk migration 00005.
ALTER TABLE parent_confirmation_previews ADD COLUMN run_id uuid;
ALTER TABLE parent_confirmation_previews ADD COLUMN tool_step integer;
ALTER TABLE parent_confirmation_previews ADD CONSTRAINT parent_preview_run_fk
 FOREIGN KEY (tenant_id,run_id) REFERENCES agent_runs(tenant_id,id);
CREATE UNIQUE INDEX parent_preview_run_step ON parent_confirmation_previews(tenant_id,run_id,tool_step) WHERE run_id IS NOT NULL;
CREATE UNIQUE INDEX agent_run_user_message ON agent_runs(tenant_id,user_message_id);

-- Normalized provenance only, never provider credentials or claims. Executing
-- a Clerk run also requires a currently verified ephemeral credential; these
-- identifiers alone confer no authority after restart.
CREATE TABLE agent_run_parent_authority (
 tenant_id uuid NOT NULL,
 run_id uuid NOT NULL,
 issuer text,
 subject text,
 session_id text,
 bff_hash bytea,
 CHECK ((issuer IS NOT NULL AND subject IS NOT NULL AND session_id IS NOT NULL AND bff_hash IS NULL)
     OR (issuer IS NULL AND subject IS NULL AND session_id IS NULL AND bff_hash IS NOT NULL)),
 PRIMARY KEY (tenant_id,run_id),
 FOREIGN KEY (tenant_id,run_id) REFERENCES agent_runs(tenant_id,id),
 FOREIGN KEY (issuer,subject) REFERENCES parent_identities(issuer,subject)
);
