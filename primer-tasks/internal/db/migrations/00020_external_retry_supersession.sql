-- A retry after expiry gets a fresh signed request identity. The original
-- envelope/body remains immutable for replay and audit purposes.
ALTER TABLE external_verifier_outbox
  ADD COLUMN IF NOT EXISTS supersedes_request_id uuid;
CREATE INDEX IF NOT EXISTS external_verifier_outbox_supersedes
  ON external_verifier_outbox(tenant_id, supersedes_request_id);
