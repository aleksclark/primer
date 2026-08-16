-- +goose Up
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION identity_canonical_text_array() RETURNS trigger AS $$
DECLARE item text; previous text := NULL; values text[];
BEGIN
  IF TG_TABLE_NAME = 'oauth_clients' THEN values := NEW.allowed_grants;
  ELSIF TG_TABLE_NAME = 'oauth_client_redirects' THEN values := NEW.allowed_scopes;
  ELSIF TG_TABLE_NAME = 'oauth_grants' THEN values := NEW.scopes;
  ELSE values := NEW.scopes;
  END IF;
  IF cardinality(values) IS NULL OR cardinality(values) > 32 THEN RAISE EXCEPTION 'array cardinality invalid'; END IF;
  FOREACH item IN ARRAY values LOOP
    IF octet_length(item) < 1 OR octet_length(item) > 128 OR item ~ '[[:cntrl:]]' OR (previous IS NOT NULL AND previous >= item) THEN RAISE EXCEPTION 'array must be sorted, unique, bounded and control-free'; END IF;
    previous := item;
  END LOOP;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TABLE oauth_clients (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), client_id varchar(128) NOT NULL UNIQUE, name varchar(200) NOT NULL,
 client_type varchar(16) NOT NULL, token_endpoint_auth_method varchar(32) NOT NULL,
 client_secret_hash bytea NULL, client_secret_pepper_version smallint NULL, allowed_grants text[] NOT NULL,
 enabled boolean NOT NULL DEFAULT true, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), disabled_at timestamptz NULL,
 CONSTRAINT oauth_clients_auth_method_value_ck CHECK (token_endpoint_auth_method IN ('none','client_secret_basic','private_key_jwt')),
 CONSTRAINT oauth_clients_type_ck CHECK (client_type IN ('public','confidential')),
 CONSTRAINT oauth_clients_grant_count_ck CHECK (cardinality(allowed_grants) BETWEEN 1 AND 3),
 CONSTRAINT oauth_clients_client_id_ck CHECK (client_id !~ '[[:cntrl:]]'),
 CONSTRAINT oauth_clients_grants_null_ck CHECK (array_position(allowed_grants, NULL) IS NULL),
 CONSTRAINT oauth_clients_allowed_grants_ck CHECK (allowed_grants <@ ARRAY['authorization_code','refresh_token','client_credentials']::text[]),
 CONSTRAINT oauth_clients_enabled_ck CHECK ((enabled AND disabled_at IS NULL) OR (NOT enabled AND disabled_at IS NOT NULL)),
 CONSTRAINT oauth_clients_auth_method_ck CHECK (((client_type='public' AND token_endpoint_auth_method='none' AND client_secret_hash IS NULL AND client_secret_pepper_version IS NULL) OR (client_type='confidential' AND token_endpoint_auth_method='client_secret_basic' AND client_secret_hash IS NOT NULL AND octet_length(client_secret_hash)=32 AND client_secret_pepper_version IS NOT NULL AND client_secret_pepper_version>0) OR (client_type='confidential' AND token_endpoint_auth_method='private_key_jwt' AND client_secret_hash IS NULL AND client_secret_pepper_version IS NULL)) IS TRUE)
);
-- IB2 owns oauth_clients_private_key_presence_ck because it owns oauth_client_keys.
CREATE TABLE oauth_client_redirects (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), oauth_client_id uuid NOT NULL, redirect_uri varchar(2048) NOT NULL, resource_uri varchar(2048) NOT NULL, audience varchar(128) NOT NULL, allowed_scopes text[] NOT NULL, enabled boolean NOT NULL DEFAULT true, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 CONSTRAINT oauth_client_redirects_oauth_client_fk FOREIGN KEY (oauth_client_id) REFERENCES oauth_clients(id) ON DELETE RESTRICT,
 CONSTRAINT oauth_client_redirects_tuple_uq UNIQUE (oauth_client_id,redirect_uri,resource_uri,audience),
 CONSTRAINT oauth_client_redirects_values_ck CHECK ((octet_length(redirect_uri) BETWEEN 1 AND 2048 AND octet_length(resource_uri) BETWEEN 1 AND 2048 AND octet_length(audience) BETWEEN 1 AND 128 AND redirect_uri !~ '[[:cntrl:]]' AND resource_uri !~ '[[:cntrl:]]' AND audience !~ '[[:cntrl:]]') IS TRUE),
 CONSTRAINT oauth_client_redirects_scopes_ck CHECK (cardinality(allowed_scopes) BETWEEN 1 AND 32 AND array_position(allowed_scopes,NULL) IS NULL)
);
CREATE CONSTRAINT TRIGGER oauth_clients_grants_canonical_ck AFTER INSERT OR UPDATE OF allowed_grants ON oauth_clients DEFERRABLE INITIALLY IMMEDIATE FOR EACH ROW EXECUTE FUNCTION identity_canonical_text_array();
CREATE CONSTRAINT TRIGGER oauth_client_redirects_scopes_canonical_ck AFTER INSERT OR UPDATE OF allowed_scopes ON oauth_client_redirects DEFERRABLE INITIALLY IMMEDIATE FOR EACH ROW EXECUTE FUNCTION identity_canonical_text_array();
-- +goose StatementEnd
-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS oauth_client_redirects;
DROP TABLE IF EXISTS oauth_clients;
DROP FUNCTION IF EXISTS identity_canonical_text_array();
-- +goose StatementEnd
