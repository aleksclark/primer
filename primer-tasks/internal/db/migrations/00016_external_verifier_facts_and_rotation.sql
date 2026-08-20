-- Phase 6 follow-up: durable attempt snapshots, versioned facts, and secret
-- rotation are additive so deployments that already applied 00015 converge.
CREATE TABLE IF NOT EXISTS external_verifier_secret_versions (
  verifier_id uuid NOT NULL REFERENCES external_verifier_catalog(id),
  secret_ref text NOT NULL,
  secret_version text NOT NULL,
  active_from timestamptz NOT NULL DEFAULT now(),
  retired_at timestamptz,
  PRIMARY KEY (verifier_id, secret_version)
);

CREATE TABLE IF NOT EXISTS external_verifier_attempts (
  tenant_id uuid NOT NULL,
  attempt_id uuid NOT NULL,
  verifier_id uuid NOT NULL REFERENCES external_verifier_catalog(id),
  capability text NOT NULL,
  schema_version text NOT NULL,
  public_options jsonb NOT NULL DEFAULT '{}'::jsonb,
  manifest_snapshot jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, attempt_id),
  FOREIGN KEY (tenant_id, attempt_id) REFERENCES verification_attempts(tenant_id, id)
);

CREATE TABLE IF NOT EXISTS external_verifier_facts (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL,
  aggregate_type text NOT NULL,
  aggregate_id uuid NOT NULL,
  fact_type text NOT NULL,
  schema_version integer NOT NULL CHECK (schema_version > 0),
  payload jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, aggregate_type, aggregate_id, fact_type),
  FOREIGN KEY (tenant_id) REFERENCES tenants(id)
);
CREATE INDEX IF NOT EXISTS external_verifier_attempts_verifier ON external_verifier_attempts(tenant_id, verifier_id);
CREATE INDEX IF NOT EXISTS external_verifier_facts_aggregate ON external_verifier_facts(tenant_id, aggregate_type, aggregate_id, created_at);
ALTER TABLE external_verifier_outbox ALTER COLUMN envelope TYPE json USING envelope::text::json;
