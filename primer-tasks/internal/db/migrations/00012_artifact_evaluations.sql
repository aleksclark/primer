CREATE TABLE IF NOT EXISTS artifact_criterion_evaluations (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL,
  submission_id uuid NOT NULL,
  criterion_id text NOT NULL,
  required boolean NOT NULL,
  status text NOT NULL CHECK (status IN ('accepted','rejected','unavailable','pending')),
  evidence text NOT NULL DEFAULT '',
  feedback text NOT NULL DEFAULT '',
  provider text NOT NULL DEFAULT '',
  model text NOT NULL DEFAULT '',
  policy_version text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, submission_id, criterion_id),
  FOREIGN KEY (tenant_id, submission_id) REFERENCES artifact_submissions(tenant_id, id)
);
CREATE INDEX IF NOT EXISTS artifact_criterion_evaluations_submission ON artifact_criterion_evaluations(tenant_id, submission_id);
