-- Tighten management tenancy, recovery envelope, and parent-visible reports.
ALTER TABLE management_enrollment_codes
    ADD COLUMN IF NOT EXISTS base_url text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS abandoned_at timestamptz;

ALTER TABLE management_devices
    ADD COLUMN IF NOT EXISTS latest_report_id uuid,
    ADD COLUMN IF NOT EXISTS enrollment_public_key text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS enrollment_key_id text NOT NULL DEFAULT '';

ALTER TABLE management_recovery_intents
    ADD COLUMN IF NOT EXISTS delivery_expires_at timestamptz,
    ADD COLUMN IF NOT EXISTS lease_expires_at timestamptz,
    ADD COLUMN IF NOT EXISTS envelope jsonb NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN IF NOT EXISTS parent_acknowledged_at timestamptz;

UPDATE management_recovery_intents
SET delivery_expires_at = COALESCE(delivery_expires_at, expires_at)
WHERE delivery_expires_at IS NULL;

ALTER TABLE management_recovery_intents
    ALTER COLUMN delivery_expires_at SET NOT NULL;

ALTER TABLE management_enrollment_codes
    DROP CONSTRAINT IF EXISTS management_enrollment_codes_tenant_id_id_key;
ALTER TABLE management_enrollment_codes
    ADD CONSTRAINT management_enrollment_codes_tenant_id_id_key UNIQUE (tenant_id, id);

ALTER TABLE management_devices
    DROP CONSTRAINT IF EXISTS management_devices_enrollment_tenant_fk;
ALTER TABLE management_devices
    ADD CONSTRAINT management_devices_enrollment_tenant_fk
    FOREIGN KEY (tenant_id, enrollment_id) REFERENCES management_enrollment_codes(tenant_id, id);

ALTER TABLE management_device_credentials
    DROP CONSTRAINT IF EXISTS management_device_credentials_tenant_id_id_key;
ALTER TABLE management_device_credentials
    ADD CONSTRAINT management_device_credentials_tenant_id_id_key UNIQUE (tenant_id, id);

ALTER TABLE management_policy_revisions
    DROP CONSTRAINT IF EXISTS management_policy_revisions_tenant_id_id_key;
ALTER TABLE management_policy_revisions
    ADD CONSTRAINT management_policy_revisions_tenant_id_id_key UNIQUE (tenant_id, id);

ALTER TABLE management_policy_reports
    DROP CONSTRAINT IF EXISTS management_policy_reports_tenant_id_id_key;
ALTER TABLE management_policy_reports
    ADD CONSTRAINT management_policy_reports_tenant_id_id_key UNIQUE (tenant_id, id);

ALTER TABLE management_recovery_intents
    DROP CONSTRAINT IF EXISTS management_recovery_intents_tenant_id_id_key;
ALTER TABLE management_recovery_intents
    ADD CONSTRAINT management_recovery_intents_tenant_id_id_key UNIQUE (tenant_id, id);

ALTER TABLE management_audit_records
    DROP CONSTRAINT IF EXISTS management_audit_records_device_tenant_fk;
ALTER TABLE management_audit_records
    ADD CONSTRAINT management_audit_records_device_tenant_fk
    FOREIGN KEY (tenant_id, device_id) REFERENCES management_devices(tenant_id, id);

ALTER TABLE management_devices
    DROP CONSTRAINT IF EXISTS management_devices_latest_report_fk;
ALTER TABLE management_devices
    ADD CONSTRAINT management_devices_latest_report_fk
    FOREIGN KEY (tenant_id, latest_report_id) REFERENCES management_policy_reports(tenant_id, id);

ALTER TABLE management_release_targets
    DROP CONSTRAINT IF EXISTS management_release_targets_tenant_id_id_key;
ALTER TABLE management_release_targets
    ADD CONSTRAINT management_release_targets_tenant_id_id_key UNIQUE (tenant_id, id);

ALTER TABLE management_release_receipts
    DROP CONSTRAINT IF EXISTS management_release_receipts_target_tenant_fk;
ALTER TABLE management_release_receipts
    ADD CONSTRAINT management_release_receipts_target_tenant_fk
    FOREIGN KEY (tenant_id, target_id) REFERENCES management_release_targets(tenant_id, id);
