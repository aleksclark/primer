-- Explicit operator linking only. Existing local subject_ref/tenant IDs and all
-- student credentials remain untouched; provider email and org claims are not keys.
ALTER TABLE parent_memberships ADD COLUMN revoked_at timestamptz;
CREATE TABLE parent_identities (
 issuer text NOT NULL,
 subject text NOT NULL,
 tenant_id uuid NOT NULL,
 subject_ref text NOT NULL,
 revoked_at timestamptz,
 created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY (issuer, subject),
 FOREIGN KEY (tenant_id, subject_ref) REFERENCES parent_memberships(tenant_id, subject_ref)
);
CREATE TABLE parent_session_revocations (
 issuer text NOT NULL,
 session_id text NOT NULL,
 revoked_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY (issuer, session_id)
);
