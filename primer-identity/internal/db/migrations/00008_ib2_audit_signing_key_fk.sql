-- +goose Up
-- +goose StatementBegin
ALTER TABLE token_issuance_audit
  ADD COLUMN signing_key_id uuid;

-- Existing committed audit rows must resolve to the durable signing-key row
-- before the new NOT NULL, RESTRICT foreign key is installed.
UPDATE token_issuance_audit a
SET signing_key_id = k.id
FROM signing_keys k
WHERE k.kid = a.kid;

DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM token_issuance_audit WHERE signing_key_id IS NULL) THEN
    RAISE EXCEPTION 'token issuance audit contains kid without a durable signing key';
  END IF;
END;
$$;

ALTER TABLE token_issuance_audit
  ALTER COLUMN signing_key_id SET NOT NULL,
  ADD CONSTRAINT token_issuance_audit_signing_key_fk
    FOREIGN KEY (signing_key_id) REFERENCES signing_keys(id) ON DELETE RESTRICT;

CREATE OR REPLACE FUNCTION identity_refresh_family_lifecycle() RETURNS trigger AS $$
DECLARE
  live_count integer;
  family_status varchar(24);
BEGIN
  IF TG_TABLE_NAME = 'oauth_refresh_families' THEN
    SELECT count(*) INTO live_count
    FROM oauth_refresh_tokens
    WHERE family_id=NEW.id AND consumed_at IS NULL AND revoked_at IS NULL;
    IF NEW.status = 'active' AND live_count <> 1 THEN
      RAISE EXCEPTION 'active refresh family must have exactly one current token';
    END IF;
    IF NEW.status <> 'active' AND live_count <> 0 THEN
      RAISE EXCEPTION 'terminal refresh family must have no current token';
    END IF;
    RETURN NEW;
  END IF;

  SELECT status INTO family_status
  FROM oauth_refresh_families
  WHERE id = COALESCE(NEW.family_id, OLD.family_id)
  FOR UPDATE;
  SELECT count(*) INTO live_count
  FROM oauth_refresh_tokens
  WHERE family_id = COALESCE(NEW.family_id, OLD.family_id)
    AND consumed_at IS NULL AND revoked_at IS NULL;
  IF family_status = 'active' AND live_count <> 1 THEN
    RAISE EXCEPTION 'active refresh family must have exactly one current token';
  END IF;
  IF family_status <> 'active' AND live_count <> 0 THEN
    RAISE EXCEPTION 'terminal refresh family must have no current token';
  END IF;
  RETURN COALESCE(NEW, OLD);
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE token_issuance_audit
  DROP CONSTRAINT IF EXISTS token_issuance_audit_signing_key_fk,
  DROP COLUMN IF EXISTS signing_key_id;

CREATE OR REPLACE FUNCTION identity_refresh_family_lifecycle() RETURNS trigger AS $$
DECLARE live_count integer;
BEGIN
  IF TG_TABLE_NAME = 'oauth_refresh_families' THEN
    SELECT count(*) INTO live_count FROM oauth_refresh_tokens WHERE family_id=NEW.id AND consumed_at IS NULL AND revoked_at IS NULL;
    IF NEW.status = 'active' AND live_count <> 1 THEN
      RAISE EXCEPTION 'active refresh family must have exactly one current token';
    END IF;
    IF NEW.status <> 'active' AND live_count <> 0 THEN
      RAISE EXCEPTION 'terminal refresh family must have no current token';
    END IF;
    RETURN NEW;
  END IF;
  PERFORM 1 FROM oauth_refresh_families WHERE id = COALESCE(NEW.family_id, OLD.family_id) FOR UPDATE;
  RETURN COALESCE(NEW, OLD);
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd
