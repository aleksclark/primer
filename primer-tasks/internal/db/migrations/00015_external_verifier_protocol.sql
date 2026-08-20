-- Phase 6 external verifier protocol. Catalog rows are administrator-owned;
-- task revisions store only the verifier id and public options, never endpoints
-- or credentials. Delivery and callback rows are tenant-scoped.
CREATE TABLE IF NOT EXISTS external_verifier_catalog (
  id uuid PRIMARY KEY,
  name text NOT NULL,
  endpoint_url text NOT NULL,
  active boolean NOT NULL DEFAULT true,
  schema_versions jsonb NOT NULL DEFAULT '[]'::jsonb,
  capabilities jsonb NOT NULL DEFAULT '[]'::jsonb,
  secret_ref text NOT NULL,
  secret_version text NOT NULL,
  timeout_seconds integer NOT NULL DEFAULT 30 CHECK (timeout_seconds > 0 AND timeout_seconds <= 300),
  max_attempts integer NOT NULL DEFAULT 5 CHECK (max_attempts > 0 AND max_attempts <= 20),
  max_age_seconds integer NOT NULL DEFAULT 86400 CHECK (max_age_seconds > 0 AND max_age_seconds <= 604800),
  egress_policy jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS external_verifier_catalog_name ON external_verifier_catalog(lower(name));

CREATE TABLE IF NOT EXISTS external_verifier_outbox (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL,
  attempt_id uuid NOT NULL,
  requirement_id uuid NOT NULL,
  verifier_id uuid NOT NULL REFERENCES external_verifier_catalog(id),
  request_id uuid NOT NULL,
  idempotency_key text NOT NULL,
  schema_version text NOT NULL,
  callback_path text NOT NULL,
  envelope jsonb NOT NULL,
  payload_digest text NOT NULL,
  status text NOT NULL DEFAULT 'queued' CHECK (status IN ('queued','running','waiting','accepted','rejected','retryable_error','terminal_error','dead','canceled')),
  attempts integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
  max_attempts integer NOT NULL CHECK (max_attempts > 0),
  available_at timestamptz NOT NULL DEFAULT now(),
  expires_at timestamptz NOT NULL,
  lease_owner text,
  lease_until timestamptz,
  last_error_code text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, id), UNIQUE (tenant_id, request_id), UNIQUE (tenant_id, idempotency_key),
  FOREIGN KEY (tenant_id, attempt_id) REFERENCES verification_attempts(tenant_id, id),
  FOREIGN KEY (tenant_id, requirement_id) REFERENCES verification_requirements(tenant_id, id)
);
CREATE INDEX IF NOT EXISTS external_verifier_outbox_claim ON external_verifier_outbox(status, available_at, lease_until);

CREATE TABLE IF NOT EXISTS external_verifier_callbacks (
  tenant_id uuid NOT NULL,
  callback_id uuid NOT NULL,
  request_id uuid NOT NULL,
  attempt_id uuid NOT NULL,
  verifier_id uuid NOT NULL REFERENCES external_verifier_catalog(id),
  sequence bigint NOT NULL CHECK (sequence > 0),
  result_type text NOT NULL CHECK (result_type IN ('progress','accepted','rejected','retryable_error','terminal_error')),
  request_digest text NOT NULL,
  body_digest text NOT NULL,
  payload jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, callback_id),
  UNIQUE (tenant_id, request_id, sequence),
  FOREIGN KEY (tenant_id, attempt_id) REFERENCES verification_attempts(tenant_id, id)
);
CREATE INDEX IF NOT EXISTS external_verifier_callbacks_attempt ON external_verifier_callbacks(tenant_id, attempt_id, sequence);

CREATE TABLE IF NOT EXISTS external_verifier_events (
  tenant_id uuid NOT NULL,
  request_id uuid NOT NULL,
  sequence bigint NOT NULL,
  kind text NOT NULL,
  payload jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, request_id, sequence),
  FOREIGN KEY (tenant_id, request_id) REFERENCES external_verifier_outbox(tenant_id, request_id)
);
CREATE INDEX IF NOT EXISTS external_verifier_events_replay ON external_verifier_events(tenant_id, request_id, sequence);

CREATE TABLE IF NOT EXISTS external_verifier_security_events (
  id bigserial PRIMARY KEY,
  tenant_id uuid,
  verifier_id uuid REFERENCES external_verifier_catalog(id),
  request_id uuid,
  failure_class text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);
