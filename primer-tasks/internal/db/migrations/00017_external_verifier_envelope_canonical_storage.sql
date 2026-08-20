-- Preserve the exact canonical JSON bytes that are signed and digest-bound.
-- jsonb reorders object keys, which would change the signed request body after
-- a restart and make an otherwise valid delivery fail closed.
ALTER TABLE external_verifier_outbox
  ALTER COLUMN envelope TYPE json USING envelope::text::json;
