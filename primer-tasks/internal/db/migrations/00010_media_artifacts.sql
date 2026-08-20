-- Phase 5 media evidence. Object bytes live in an object store; PostgreSQL only
-- records tenant-scoped identifiers, metadata, digests, and lifecycle state.
CREATE TABLE IF NOT EXISTS artifacts (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL,
  student_id uuid NOT NULL,
  object_key text NOT NULL,
  kind text NOT NULL CHECK (kind IN ('image','video','audio')),
  original_name text NOT NULL DEFAULT '',
  declared_content_type text NOT NULL DEFAULT '',
  detected_content_type text NOT NULL DEFAULT '',
  byte_size bigint NOT NULL DEFAULT 0 CHECK (byte_size >= 0),
  sha256 text NOT NULL DEFAULT '',
  width integer,
  height integer,
  duration_ms bigint,
  status text NOT NULL DEFAULT 'reserved' CHECK (status IN ('reserved','uploaded','finalized','rejected','tombstoned')),
  created_at timestamptz NOT NULL DEFAULT now(),
  finalized_at timestamptz,
  expires_at timestamptz NOT NULL,
  deleted_at timestamptz,
  UNIQUE (tenant_id, id),
  UNIQUE (object_key),
  FOREIGN KEY (tenant_id, student_id) REFERENCES students(tenant_id, id)
);
CREATE INDEX IF NOT EXISTS artifacts_owner ON artifacts(tenant_id, student_id, created_at DESC);
CREATE TABLE IF NOT EXISTS artifact_upload_reservations (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL,
  student_id uuid NOT NULL,
  artifact_id uuid NOT NULL,
  occurrence_id uuid,
  requirement_id uuid,
  idempotency_key text NOT NULL,
  part_count integer NOT NULL DEFAULT 1 CHECK (part_count > 0 AND part_count <= 10000),
  status text NOT NULL DEFAULT 'reserved' CHECK (status IN ('reserved','finalized','expired','canceled')),
  expires_at timestamptz NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, student_id, idempotency_key),
  UNIQUE (tenant_id, id),
  FOREIGN KEY (tenant_id, student_id) REFERENCES students(tenant_id, id),
  FOREIGN KEY (tenant_id, artifact_id) REFERENCES artifacts(tenant_id, id),
  FOREIGN KEY (tenant_id, occurrence_id) REFERENCES task_occurrences(tenant_id, id),
  FOREIGN KEY (tenant_id, requirement_id) REFERENCES verification_requirements(tenant_id, id)
);
CREATE TABLE IF NOT EXISTS artifact_upload_parts (
  tenant_id uuid NOT NULL,
  reservation_id uuid NOT NULL,
  part_number integer NOT NULL CHECK (part_number > 0),
  object_key text NOT NULL,
  byte_size bigint NOT NULL DEFAULT 0 CHECK (byte_size >= 0),
  sha256 text NOT NULL DEFAULT '',
  uploaded_at timestamptz,
  PRIMARY KEY (tenant_id, reservation_id, part_number),
  UNIQUE (object_key),
  FOREIGN KEY (tenant_id, reservation_id) REFERENCES artifact_upload_reservations(tenant_id, id) ON DELETE CASCADE
);
CREATE TABLE IF NOT EXISTS artifact_submissions (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL,
  student_id uuid NOT NULL,
  occurrence_id uuid NOT NULL,
  requirement_id uuid NOT NULL,
  attempt_id uuid NOT NULL,
  artifact_id uuid NOT NULL,
  idempotency_key text NOT NULL,
  status text NOT NULL DEFAULT 'submitted' CHECK (status IN ('submitted','evaluating','accepted','rejected','review')),
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, student_id, idempotency_key),
  UNIQUE (tenant_id, id),
  FOREIGN KEY (tenant_id, student_id) REFERENCES students(tenant_id, id),
  FOREIGN KEY (tenant_id, occurrence_id) REFERENCES task_occurrences(tenant_id, id),
  FOREIGN KEY (tenant_id, requirement_id) REFERENCES verification_requirements(tenant_id, id),
  FOREIGN KEY (tenant_id, attempt_id) REFERENCES verification_attempts(tenant_id, id),
  FOREIGN KEY (tenant_id, artifact_id) REFERENCES artifacts(tenant_id, id)
);
CREATE INDEX IF NOT EXISTS artifact_submissions_occurrence ON artifact_submissions(tenant_id, occurrence_id, requirement_id, created_at DESC);
CREATE TABLE IF NOT EXISTS artifact_derivatives (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL,
  artifact_id uuid NOT NULL,
  derivative_kind text NOT NULL CHECK (derivative_kind IN ('thumbnail','preview')),
  object_key text NOT NULL,
  content_type text NOT NULL,
  byte_size bigint NOT NULL CHECK (byte_size >= 0),
  sha256 text NOT NULL,
  width integer,
  height integer,
  metadata_stripped boolean NOT NULL DEFAULT true,
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, artifact_id, derivative_kind),
  UNIQUE (object_key),
  FOREIGN KEY (tenant_id, artifact_id) REFERENCES artifacts(tenant_id, id)
);
CREATE TABLE IF NOT EXISTS artifact_scans (
  tenant_id uuid NOT NULL,
  artifact_id uuid NOT NULL,
  scanner text NOT NULL,
  status text NOT NULL CHECK (status IN ('pending','clean','infected','error','not_configured')),
  detail text NOT NULL DEFAULT '',
  scanned_at timestamptz,
  PRIMARY KEY (tenant_id, artifact_id),
  FOREIGN KEY (tenant_id, artifact_id) REFERENCES artifacts(tenant_id, id)
);
CREATE TABLE IF NOT EXISTS artifact_retention (
  tenant_id uuid NOT NULL,
  artifact_id uuid NOT NULL,
  retain_original_until timestamptz NOT NULL,
  retain_derivatives_until timestamptz NOT NULL,
  tombstoned_at timestamptz,
  PRIMARY KEY (tenant_id, artifact_id),
  FOREIGN KEY (tenant_id, artifact_id) REFERENCES artifacts(tenant_id, id)
);
CREATE TABLE IF NOT EXISTS artifact_rubric_jobs (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL,
  submission_id uuid NOT NULL,
  status text NOT NULL CHECK (status IN ('queued','running','succeeded','failed','review')) DEFAULT 'queued',
  attempts integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
  available_at timestamptz NOT NULL DEFAULT now(),
  lease_owner text,
  lease_until timestamptz,
  provider text NOT NULL DEFAULT '',
  model text NOT NULL DEFAULT '',
  rubric_snapshot jsonb NOT NULL DEFAULT '{}',
  last_error text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, submission_id),
  FOREIGN KEY (tenant_id, submission_id) REFERENCES artifact_submissions(tenant_id, id)
);
CREATE INDEX IF NOT EXISTS artifact_rubric_jobs_claim ON artifact_rubric_jobs(status, available_at, lease_until);
