-- Remote management is a bounded context in the Tasks database. It deliberately
-- does not reuse Tasks student/session/device credentials and does not cascade on
-- student archival.
CREATE TABLE IF NOT EXISTS management_enrollment_codes (
    id uuid PRIMARY KEY,
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    created_by text NOT NULL,
    label text NOT NULL DEFAULT '',
    code_hash bytea NOT NULL UNIQUE,
    expires_at timestamptz NOT NULL,
    consumed_at timestamptz,
    consumed_device_id uuid,
    revoked_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS management_enrollment_codes_tenant_created
    ON management_enrollment_codes(tenant_id, created_at DESC);

CREATE TABLE IF NOT EXISTS management_devices (
    id uuid PRIMARY KEY,
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    enrollment_id uuid REFERENCES management_enrollment_codes(id),
    display_name text NOT NULL,
    device_model text NOT NULL DEFAULT '',
    stable_device_key_hash bytea,
    capabilities jsonb NOT NULL DEFAULT '{}'::jsonb,
    state text NOT NULL DEFAULT 'active' CHECK (state IN ('active','quarantined','revoked','decommissioned')),
    desired_revision bigint NOT NULL DEFAULT 0,
    applied_revision bigint NOT NULL DEFAULT 0,
    last_seen_at timestamptz,
    quarantined_at timestamptz,
    revoked_at timestamptz,
    decommissioned_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, id)
);
CREATE INDEX IF NOT EXISTS management_devices_tenant_state
    ON management_devices(tenant_id, state, created_at DESC);

ALTER TABLE management_enrollment_codes
    DROP CONSTRAINT IF EXISTS management_enrollment_codes_consumed_device_fk;
ALTER TABLE management_enrollment_codes
    ADD CONSTRAINT management_enrollment_codes_consumed_device_fk
    FOREIGN KEY (tenant_id, consumed_device_id) REFERENCES management_devices(tenant_id, id);

CREATE TABLE IF NOT EXISTS management_device_credentials (
    id uuid PRIMARY KEY,
    tenant_id uuid NOT NULL,
    device_id uuid NOT NULL,
    token_hash bytea NOT NULL UNIQUE,
    created_at timestamptz NOT NULL DEFAULT now(),
    last_used_at timestamptz,
    revoked_at timestamptz,
    FOREIGN KEY (tenant_id, device_id) REFERENCES management_devices(tenant_id, id)
);
CREATE INDEX IF NOT EXISTS management_device_credentials_device_active
    ON management_device_credentials(tenant_id, device_id) WHERE revoked_at IS NULL;

CREATE TABLE IF NOT EXISTS management_policy_revisions (
    id uuid PRIMARY KEY,
    tenant_id uuid NOT NULL,
    device_id uuid NOT NULL,
    revision bigint NOT NULL,
    created_by text NOT NULL,
    policy jsonb NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, device_id, revision),
    FOREIGN KEY (tenant_id, device_id) REFERENCES management_devices(tenant_id, id)
);
CREATE INDEX IF NOT EXISTS management_policy_revisions_device_revision
    ON management_policy_revisions(tenant_id, device_id, revision DESC);

CREATE TABLE IF NOT EXISTS management_policy_reports (
    id uuid PRIMARY KEY,
    tenant_id uuid NOT NULL,
    device_id uuid NOT NULL,
    report_id uuid NOT NULL,
    revision bigint NOT NULL,
    status text NOT NULL CHECK (status IN ('requested','applied','partial','failed','stale')),
    installed_student_version text NOT NULL DEFAULT '',
    device_reported_at timestamptz,
    received_at timestamptz NOT NULL DEFAULT now(),
    stale boolean NOT NULL DEFAULT false,
    report jsonb NOT NULL DEFAULT '{}'::jsonb,
    UNIQUE(tenant_id, device_id, report_id),
    FOREIGN KEY (tenant_id, device_id) REFERENCES management_devices(tenant_id, id)
);
CREATE INDEX IF NOT EXISTS management_policy_reports_device_received
    ON management_policy_reports(tenant_id, device_id, received_at DESC);

CREATE TABLE IF NOT EXISTS management_recovery_intents (
    id uuid PRIMARY KEY,
    tenant_id uuid NOT NULL,
    device_id uuid NOT NULL,
    kind text NOT NULL CHECK (kind IN ('maintenance_lease','rotate_recovery_code')),
    status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','applied','expired','revoked')),
    created_by text NOT NULL,
    expires_at timestamptz NOT NULL,
    material_ciphertext text NOT NULL DEFAULT '',
    material_hash bytea,
    applied_report_id uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    applied_at timestamptz,
    FOREIGN KEY (tenant_id, device_id) REFERENCES management_devices(tenant_id, id)
);
CREATE INDEX IF NOT EXISTS management_recovery_intents_pending
    ON management_recovery_intents(tenant_id, device_id, expires_at) WHERE status='pending';

CREATE TABLE IF NOT EXISTS management_audit_records (
    id bigserial PRIMARY KEY,
    tenant_id uuid REFERENCES tenants(id),
    device_id uuid,
    actor_kind text NOT NULL CHECK (actor_kind IN ('parent','management_device','release_publisher','system')),
    actor_ref text NOT NULL,
    action text NOT NULL,
    metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS management_audit_tenant_created
    ON management_audit_records(tenant_id, created_at DESC);
