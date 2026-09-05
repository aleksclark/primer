-- Durable authorization-code state and explicit student session/device history.
CREATE TABLE IF NOT EXISTS auth_states (
    state_hash bytea PRIMARY KEY,
    verifier_ciphertext bytea NOT NULL,
    redirect_uri text NOT NULL,
    return_path text NOT NULL DEFAULT '/parent/students',
    client_id text NOT NULL,
    expires_at timestamptz NOT NULL,
    consumed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS auth_states_expiry ON auth_states (expires_at);

-- These constraints prevent a row from naming a student in another tenant,
-- even if application code later acquires a new query path.
ALTER TABLE pairing_codes DROP CONSTRAINT IF EXISTS pairing_codes_student_id_fkey;
ALTER TABLE student_devices DROP CONSTRAINT IF EXISTS student_devices_student_id_fkey;
ALTER TABLE pairing_codes DROP CONSTRAINT IF EXISTS pairing_codes_student_tenant_fk;
ALTER TABLE student_devices DROP CONSTRAINT IF EXISTS student_devices_student_tenant_fk;
ALTER TABLE students ADD CONSTRAINT students_tenant_id_unique UNIQUE (tenant_id, id);
ALTER TABLE pairing_codes ADD CONSTRAINT pairing_codes_student_tenant_fk FOREIGN KEY (tenant_id, student_id) REFERENCES students(tenant_id, id);
ALTER TABLE student_devices ADD CONSTRAINT student_devices_student_tenant_fk FOREIGN KEY (tenant_id, student_id) REFERENCES students(tenant_id, id);

CREATE TABLE IF NOT EXISTS student_sessions (
    id uuid PRIMARY KEY,
    tenant_id uuid NOT NULL,
    student_id uuid NOT NULL,
    handle_hash bytea NOT NULL UNIQUE,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    revoked_at timestamptz,
    CONSTRAINT student_sessions_student_tenant_fk FOREIGN KEY (tenant_id, student_id) REFERENCES students(tenant_id, id)
);
CREATE INDEX IF NOT EXISTS student_sessions_active ON student_sessions (handle_hash, expires_at) WHERE revoked_at IS NULL;

ALTER TABLE bff_sessions ADD COLUMN IF NOT EXISTS session_kind text NOT NULL DEFAULT 'parent';
ALTER TABLE bff_sessions ADD CONSTRAINT bff_sessions_kind_ck CHECK (session_kind IN ('parent', 'student'));
ALTER TABLE audit_records ADD COLUMN IF NOT EXISTS metadata jsonb;

-- Bounded pairing guess ledger. Secrets are never stored; only SHA-256 hashes.
CREATE TABLE IF NOT EXISTS pairing_guess_attempts (
    id bigserial PRIMARY KEY,
    client_key text NOT NULL,
    code_hash bytea NOT NULL,
    attempted_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS pairing_guess_attempts_client_window
    ON pairing_guess_attempts (client_key, attempted_at);
