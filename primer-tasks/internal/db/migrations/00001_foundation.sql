CREATE TABLE IF NOT EXISTS tenants (
 id uuid PRIMARY KEY, name text NOT NULL, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS parent_memberships (
 tenant_id uuid NOT NULL REFERENCES tenants(id), subject_ref text NOT NULL, role text NOT NULL CHECK (role='admin'),
 PRIMARY KEY (tenant_id, subject_ref)
);
CREATE TABLE IF NOT EXISTS bff_sessions (
 handle_hash bytea PRIMARY KEY, tenant_id uuid NOT NULL REFERENCES tenants(id), subject_ref text NOT NULL,
 expires_at timestamptz NOT NULL, revoked_at timestamptz
);
CREATE TABLE IF NOT EXISTS students (
 id uuid PRIMARY KEY, tenant_id uuid NOT NULL REFERENCES tenants(id), display_name text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(), archived_at timestamptz,
 UNIQUE (tenant_id, id)
);
CREATE UNIQUE INDEX IF NOT EXISTS students_tenant_name_active ON students(tenant_id, lower(display_name)) WHERE archived_at IS NULL;
CREATE TABLE IF NOT EXISTS pairing_codes (
 id uuid PRIMARY KEY, tenant_id uuid NOT NULL REFERENCES tenants(id), student_id uuid NOT NULL,
 CONSTRAINT pairing_codes_student_tenant_fk FOREIGN KEY (tenant_id, student_id) REFERENCES students(tenant_id, id),
 code_hash bytea NOT NULL UNIQUE, expires_at timestamptz NOT NULL, claimed_at timestamptz, revoked_at timestamptz,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS student_devices (
 id uuid PRIMARY KEY, tenant_id uuid NOT NULL REFERENCES tenants(id), student_id uuid NOT NULL,
 CONSTRAINT student_devices_student_tenant_fk FOREIGN KEY (tenant_id, student_id) REFERENCES students(tenant_id, id),
 token_hash bytea NOT NULL UNIQUE, created_at timestamptz NOT NULL DEFAULT now(), revoked_at timestamptz
);
CREATE TABLE IF NOT EXISTS audit_records (
 id bigserial PRIMARY KEY, tenant_id uuid REFERENCES tenants(id), subject_ref text, action text NOT NULL,
 entity_id uuid, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS students_tenant_created ON students(tenant_id, created_at DESC);
