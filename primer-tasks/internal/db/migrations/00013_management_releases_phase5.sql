CREATE TABLE IF NOT EXISTS management_releases (
    id uuid PRIMARY KEY,
    package_name text NOT NULL,
    channel text NOT NULL,
    version_code bigint NOT NULL CHECK (version_code > 0),
    version_name text NOT NULL,
    min_sdk integer NOT NULL CHECK (min_sdk > 0),
    supported_abis text[] NOT NULL DEFAULT '{}'::text[],
    signer_sha256 text NOT NULL,
    sha256 text NOT NULL,
    byte_size bigint NOT NULL CHECK (byte_size > 0),
    artifact_path text NOT NULL,
    manifest jsonb NOT NULL,
    manifest_signature text NOT NULL,
    status text NOT NULL DEFAULT 'published' CHECK (status IN ('published','paused')),
    published_by text NOT NULL,
    published_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(package_name, channel, version_code),
    UNIQUE(sha256)
);
CREATE INDEX IF NOT EXISTS management_releases_package_channel
    ON management_releases(package_name, channel, version_code DESC);

CREATE TABLE IF NOT EXISTS management_release_targets (
    id uuid PRIMARY KEY,
    tenant_id uuid NOT NULL,
    device_id uuid NOT NULL,
    release_id uuid NOT NULL REFERENCES management_releases(id),
    package_name text NOT NULL,
    channel text NOT NULL,
    status text NOT NULL DEFAULT 'queued' CHECK (status IN ('queued','downloading','verifying','installing','confirmed','blocked','failed')),
    target_version bigint NOT NULL DEFAULT 1,
    desired_by text NOT NULL,
    desired_at timestamptz NOT NULL DEFAULT now(),
    last_receipt_at timestamptz,
    last_error text NOT NULL DEFAULT '',
    UNIQUE(tenant_id, device_id, package_name, channel),
    FOREIGN KEY (tenant_id, device_id) REFERENCES management_devices(tenant_id, id)
);
CREATE INDEX IF NOT EXISTS management_release_targets_device
    ON management_release_targets(tenant_id, device_id, desired_at DESC);

CREATE TABLE IF NOT EXISTS management_release_receipts (
    id uuid PRIMARY KEY,
    tenant_id uuid NOT NULL,
    device_id uuid NOT NULL,
    target_id uuid NOT NULL,
    report_id uuid NOT NULL,
    status text NOT NULL CHECK (status IN ('queued','downloading','verifying','installing','confirmed','blocked','failed')),
    installed_version_code bigint,
    installed_version_name text NOT NULL DEFAULT '',
    error text NOT NULL DEFAULT '',
    raw jsonb NOT NULL DEFAULT '{}'::jsonb,
    received_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, device_id, report_id),
    FOREIGN KEY (tenant_id, device_id) REFERENCES management_devices(tenant_id, id),
    FOREIGN KEY (target_id) REFERENCES management_release_targets(id)
);
CREATE INDEX IF NOT EXISTS management_release_receipts_target
    ON management_release_receipts(target_id, received_at DESC);
