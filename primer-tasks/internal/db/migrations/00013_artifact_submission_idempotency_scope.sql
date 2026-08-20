ALTER TABLE artifact_submissions
  DROP CONSTRAINT IF EXISTS artifact_submissions_tenant_id_student_id_idempotency_key_key;
ALTER TABLE artifact_submissions
  ADD CONSTRAINT artifact_submissions_occurrence_idempotency_key_key
  UNIQUE (tenant_id, student_id, occurrence_id, idempotency_key);
