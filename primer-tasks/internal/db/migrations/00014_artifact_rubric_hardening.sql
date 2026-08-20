-- Phase 5 artifact evaluator hardening. Progress is durable and replayable;
-- provider attempts are bounded independently of Fantasy's in-call retries.
ALTER TABLE artifact_rubric_jobs ADD COLUMN IF NOT EXISTS max_attempts integer NOT NULL DEFAULT 2;
ALTER TABLE artifact_rubric_jobs ADD COLUMN IF NOT EXISTS progress_sequence bigint NOT NULL DEFAULT 0;
CREATE UNIQUE INDEX IF NOT EXISTS artifact_rubric_jobs_tenant_id ON artifact_rubric_jobs(tenant_id, id);
CREATE TABLE IF NOT EXISTS artifact_rubric_events (
  tenant_id uuid NOT NULL,
  job_id uuid NOT NULL,
  submission_id uuid NOT NULL,
  sequence bigint NOT NULL,
  kind text NOT NULL,
  payload jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, job_id, sequence),
  FOREIGN KEY (tenant_id, job_id) REFERENCES artifact_rubric_jobs(tenant_id, id) ON DELETE CASCADE,
  FOREIGN KEY (tenant_id, submission_id) REFERENCES artifact_submissions(tenant_id, id)
);
CREATE INDEX IF NOT EXISTS artifact_rubric_events_replay
  ON artifact_rubric_events(tenant_id, submission_id, sequence);
