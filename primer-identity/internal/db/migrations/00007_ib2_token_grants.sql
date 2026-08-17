-- +goose Up
-- +goose StatementBegin
CREATE TABLE oauth_client_keys (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 oauth_client_id uuid NOT NULL,
 kid varchar(128) NOT NULL,
 jwk_json jsonb NOT NULL,
 alg varchar(16) NOT NULL DEFAULT 'ES256',
 use varchar(16) NOT NULL DEFAULT 'sig',
 enabled boolean NOT NULL DEFAULT true,
 not_before timestamptz NULL,
 not_after timestamptz NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 disabled_at timestamptz NULL,
 CONSTRAINT oauth_client_keys_oauth_client_fk FOREIGN KEY (oauth_client_id) REFERENCES oauth_clients(id) ON DELETE RESTRICT,
 CONSTRAINT oauth_client_keys_client_kid_uq UNIQUE (oauth_client_id,kid),
 CONSTRAINT oauth_client_keys_profile_ck CHECK (alg='ES256' AND use='sig' AND kid !~ '[[:cntrl:]]'),
 CONSTRAINT oauth_client_keys_lifetime_ck CHECK (not_after IS NULL OR not_before IS NULL OR not_after > not_before),
 CONSTRAINT oauth_client_keys_public_jwk_ck CHECK (jsonb_typeof(jwk_json)='object' AND jwk_json ? 'kty' AND NOT (jwk_json ? 'd'))
);

CREATE OR REPLACE FUNCTION identity_private_key_jwt_presence() RETURNS trigger AS $$
DECLARE target uuid;
BEGIN
  IF TG_TABLE_NAME = 'oauth_clients' THEN
    target := NEW.id;
    IF NEW.token_endpoint_auth_method <> 'private_key_jwt' OR NOT NEW.enabled THEN
      RETURN NEW;
    END IF;
  ELSE
    target := COALESCE(NEW.oauth_client_id, OLD.oauth_client_id);
  END IF;
  IF EXISTS (
    SELECT 1 FROM oauth_clients c
    WHERE c.id = target AND c.enabled AND c.token_endpoint_auth_method = 'private_key_jwt'
      AND NOT EXISTS (
        SELECT 1 FROM oauth_client_keys k
        WHERE k.oauth_client_id = c.id AND k.enabled
          AND (k.not_before IS NULL OR k.not_before <= now())
          AND (k.not_after IS NULL OR k.not_after > now())
          AND (k.disabled_at IS NULL)
      )
  ) THEN
    RAISE EXCEPTION 'enabled private_key_jwt client requires a currently valid registered public key';
  END IF;
  RETURN COALESCE(NEW, OLD);
END;
$$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER oauth_clients_private_key_presence_ck
AFTER INSERT OR UPDATE OF token_endpoint_auth_method, enabled, disabled_at ON oauth_clients
DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION identity_private_key_jwt_presence();
CREATE CONSTRAINT TRIGGER oauth_client_keys_private_key_presence_ck
AFTER INSERT OR UPDATE OR DELETE ON oauth_client_keys
DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION identity_private_key_jwt_presence();

CREATE TABLE oauth_refresh_families (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 grant_id uuid NOT NULL,
 oauth_client_id uuid NOT NULL,
 resource_uri varchar(2048) NOT NULL,
 status varchar(24) NOT NULL DEFAULT 'active',
 absolute_expires_at timestamptz NOT NULL,
 idle_expires_at timestamptz NOT NULL,
 last_rotated_at timestamptz NOT NULL DEFAULT now(),
 revoked_at timestamptz NULL,
 expired_at timestamptz NULL,
 reuse_detected_at timestamptz NULL,
 revoke_reason_code varchar(64) NULL,
 version bigint NOT NULL DEFAULT 0,
 CONSTRAINT oauth_refresh_families_grant_fk FOREIGN KEY (grant_id) REFERENCES oauth_grants(id) ON DELETE RESTRICT,
 CONSTRAINT oauth_refresh_families_oauth_client_fk FOREIGN KEY (oauth_client_id) REFERENCES oauth_clients(id) ON DELETE RESTRICT,
 CONSTRAINT oauth_refresh_families_status_lifetime_ck CHECK ((status IN ('active','revoked','expired','reuse_detected') AND absolute_expires_at>last_rotated_at AND idle_expires_at>last_rotated_at AND idle_expires_at<=absolute_expires_at AND idle_expires_at<=last_rotated_at+interval '14 days' AND absolute_expires_at<=last_rotated_at+interval '90 days') IS TRUE),
 CONSTRAINT oauth_refresh_families_state_ck CHECK ((
   (status='active' AND revoked_at IS NULL AND expired_at IS NULL AND reuse_detected_at IS NULL AND revoke_reason_code IS NULL)
   OR (status='revoked' AND revoked_at IS NOT NULL AND expired_at IS NULL AND reuse_detected_at IS NULL AND revoke_reason_code IS NOT NULL)
   OR (status='expired' AND revoked_at IS NULL AND expired_at IS NOT NULL AND reuse_detected_at IS NULL AND revoke_reason_code IS NOT NULL)
   OR (status='reuse_detected' AND revoked_at IS NOT NULL AND expired_at IS NULL AND reuse_detected_at IS NOT NULL AND revoke_reason_code IS NOT NULL)
 ) IS TRUE)
);

CREATE TABLE oauth_refresh_tokens (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 family_id uuid NOT NULL,
 token_hash bytea NOT NULL,
 pepper_version smallint NOT NULL,
 sequence bigint NOT NULL,
 issued_at timestamptz NOT NULL DEFAULT now(),
 expires_at timestamptz NOT NULL,
 consumed_at timestamptz NULL,
 revoked_at timestamptz NULL,
 replaced_by_id uuid NULL,
 reuse_detected_at timestamptz NULL,
 CONSTRAINT oauth_refresh_tokens_family_fk FOREIGN KEY (family_id) REFERENCES oauth_refresh_families(id) ON DELETE RESTRICT,
 CONSTRAINT oauth_refresh_tokens_replaced_by_fk FOREIGN KEY (replaced_by_id) REFERENCES oauth_refresh_tokens(id) ON DELETE RESTRICT,
 CONSTRAINT oauth_refresh_tokens_hash_lifetime_ck CHECK (octet_length(token_hash)=32 AND pepper_version>0 AND sequence>=0 AND expires_at>issued_at),
 CONSTRAINT oauth_refresh_tokens_replace_ck CHECK (replaced_by_id IS NULL OR replaced_by_id<>id),
 CONSTRAINT oauth_refresh_tokens_state_ck CHECK ((
   (consumed_at IS NULL AND revoked_at IS NULL AND replaced_by_id IS NULL AND reuse_detected_at IS NULL)
   OR (consumed_at IS NOT NULL AND revoked_at IS NULL AND replaced_by_id IS NOT NULL)
   OR (consumed_at IS NULL AND revoked_at IS NOT NULL AND replaced_by_id IS NULL AND reuse_detected_at IS NULL)
 ) IS TRUE),
 CONSTRAINT oauth_refresh_tokens_time_ck CHECK ((consumed_at IS NULL OR consumed_at>=issued_at) AND (revoked_at IS NULL OR revoked_at>=issued_at) AND (reuse_detected_at IS NULL OR (consumed_at IS NOT NULL AND replaced_by_id IS NOT NULL AND reuse_detected_at>=consumed_at))),
 CONSTRAINT oauth_refresh_tokens_hash_uq UNIQUE(token_hash),
 CONSTRAINT oauth_refresh_tokens_family_sequence_uq UNIQUE(family_id,sequence),
 CONSTRAINT oauth_refresh_tokens_replaced_by_uq UNIQUE(replaced_by_id)
);
CREATE UNIQUE INDEX oauth_refresh_tokens_one_live_uq ON oauth_refresh_tokens(family_id) WHERE consumed_at IS NULL AND revoked_at IS NULL;

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

CREATE CONSTRAINT TRIGGER oauth_refresh_family_lifecycle_ck
AFTER INSERT OR UPDATE ON oauth_refresh_families
DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION identity_refresh_family_lifecycle();
CREATE CONSTRAINT TRIGGER oauth_refresh_tokens_family_lifecycle_ck
AFTER INSERT OR UPDATE OR DELETE ON oauth_refresh_tokens
DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION identity_refresh_family_lifecycle();

CREATE TABLE oauth_client_assertion_replays (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 oauth_client_id uuid NOT NULL,
 endpoint_kind varchar(16) NOT NULL,
 jti_hash bytea NOT NULL,
 audience varchar(2048) NOT NULL,
 issued_at timestamptz NOT NULL,
 expires_at timestamptz NOT NULL,
 consumed_at timestamptz NOT NULL DEFAULT now(),
 CONSTRAINT oauth_client_assertion_replays_oauth_client_fk FOREIGN KEY (oauth_client_id) REFERENCES oauth_clients(id) ON DELETE RESTRICT,
 CONSTRAINT oauth_client_assertion_replays_ck CHECK (endpoint_kind IN ('token','revocation') AND octet_length(jti_hash)=32 AND expires_at>consumed_at AND expires_at>issued_at AND expires_at<=issued_at+interval '5 minutes' AND audience !~ '[[:cntrl:]]' AND octet_length(audience) BETWEEN 1 AND 2048),
 CONSTRAINT oauth_client_assertion_replays_uq UNIQUE(oauth_client_id,endpoint_kind,jti_hash)
);
CREATE INDEX oauth_client_assertion_replays_expiry_idx ON oauth_client_assertion_replays(expires_at);

CREATE TABLE token_issuance_audit (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 grant_id uuid NOT NULL,
 authorization_code_id uuid NULL,
 authorization_code_hash bytea NULL,
 subject_ref varchar(160) NOT NULL,
 client_id varchar(128) NOT NULL,
 resource_uri varchar(2048) NOT NULL,
 audience varchar(128) NOT NULL,
 scopes text[] NOT NULL,
 jti_hash bytea NOT NULL,
 kid varchar(128) NOT NULL,
 issued_at timestamptz NOT NULL DEFAULT now(),
 expires_at timestamptz NOT NULL,
 outcome varchar(24) NOT NULL DEFAULT 'committed',
 request_correlation_hash bytea NULL,
 CONSTRAINT token_issuance_audit_grant_fk FOREIGN KEY (grant_id) REFERENCES oauth_grants(id) ON DELETE RESTRICT,
 CONSTRAINT token_issuance_audit_code_fk FOREIGN KEY (authorization_code_id) REFERENCES oauth_authorization_codes(id) ON DELETE SET NULL,
 CONSTRAINT token_issuance_audit_ck CHECK (
   octet_length(jti_hash)=32 AND outcome='committed' AND expires_at>issued_at AND expires_at<=issued_at+interval '15 minutes'
   AND (authorization_code_hash IS NULL OR octet_length(authorization_code_hash)=32)
   AND (authorization_code_id IS NULL OR authorization_code_hash IS NOT NULL)
   AND (request_correlation_hash IS NULL OR octet_length(request_correlation_hash)=32)
   AND subject_ref ~ '^identity:[0-9a-f-]{36}$'
   AND octet_length(client_id) BETWEEN 1 AND 128 AND client_id !~ '[[:cntrl:]]'
   AND kid !~ '[[:cntrl:]]'
 )
);
CREATE CONSTRAINT TRIGGER token_issuance_audit_scopes_canonical_ck
AFTER INSERT OR UPDATE OF scopes ON token_issuance_audit
DEFERRABLE INITIALLY IMMEDIATE FOR EACH ROW EXECUTE FUNCTION identity_canonical_text_array();
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS token_issuance_audit;
DROP TABLE IF EXISTS oauth_client_assertion_replays;
DROP TABLE IF EXISTS oauth_refresh_tokens;
DROP TABLE IF EXISTS oauth_refresh_families;
DROP TRIGGER IF EXISTS oauth_client_keys_private_key_presence_ck ON oauth_client_keys;
DROP TRIGGER IF EXISTS oauth_clients_private_key_presence_ck ON oauth_clients;
DROP FUNCTION IF EXISTS identity_refresh_family_lifecycle();
DROP FUNCTION IF EXISTS identity_private_key_jwt_presence();
DROP TABLE IF EXISTS oauth_client_keys;
-- +goose StatementEnd
