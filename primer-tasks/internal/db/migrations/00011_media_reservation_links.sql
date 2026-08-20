-- Backfill-safe upgrade for deployments that applied the initial media schema
-- before reservation ownership links were added.
ALTER TABLE artifact_upload_reservations ADD COLUMN IF NOT EXISTS occurrence_id uuid;
ALTER TABLE artifact_upload_reservations ADD COLUMN IF NOT EXISTS requirement_id uuid;
DO $$ BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='artifact_upload_reservations_occurrence_fk') THEN
    ALTER TABLE artifact_upload_reservations ADD CONSTRAINT artifact_upload_reservations_occurrence_fk FOREIGN KEY (tenant_id, occurrence_id) REFERENCES task_occurrences(tenant_id, id);
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='artifact_upload_reservations_requirement_fk') THEN
    ALTER TABLE artifact_upload_reservations ADD CONSTRAINT artifact_upload_reservations_requirement_fk FOREIGN KEY (tenant_id, requirement_id) REFERENCES verification_requirements(tenant_id, id);
  END IF;
END $$;
