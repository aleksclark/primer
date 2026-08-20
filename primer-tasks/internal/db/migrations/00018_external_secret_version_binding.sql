-- Bind each request to the secret version selected at delivery time so
-- rotation can accept in-flight callbacks without accepting arbitrary keys.
ALTER TABLE external_verifier_outbox ADD COLUMN IF NOT EXISTS secret_version text NOT NULL DEFAULT '';
ALTER TABLE external_verifier_attempts ADD COLUMN IF NOT EXISTS secret_version text NOT NULL DEFAULT '';
UPDATE external_verifier_outbox o SET secret_version=c.secret_version FROM external_verifier_catalog c WHERE o.verifier_id=c.id AND o.secret_version='';
UPDATE external_verifier_attempts a SET secret_version=c.secret_version FROM external_verifier_catalog c WHERE a.verifier_id=c.id AND a.secret_version='';
