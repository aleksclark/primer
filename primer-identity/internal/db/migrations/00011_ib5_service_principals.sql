-- +goose Up
-- +goose StatementBegin
CREATE TABLE oauth_service_principals (
 id uuid PRIMARY KEY,
 subject_ref varchar(160) NOT NULL UNIQUE,
 display_name varchar(200) NOT NULL,
 enabled boolean NOT NULL DEFAULT true,
 created_at timestamptz NOT NULL DEFAULT now(),
 disabled_at timestamptz NULL,
 CONSTRAINT oauth_service_principals_subject_ck CHECK (subject_ref='identity:svc:'||id::text),
 CONSTRAINT oauth_service_principals_enabled_ck CHECK ((enabled AND disabled_at IS NULL) OR (NOT enabled AND disabled_at IS NOT NULL))
);
ALTER TABLE oauth_grants ADD CONSTRAINT oauth_grants_service_principal_fk FOREIGN KEY (service_principal_id) REFERENCES oauth_service_principals(id) ON DELETE RESTRICT;
ALTER TABLE token_issuance_audit DROP CONSTRAINT token_issuance_audit_ck;
ALTER TABLE token_issuance_audit ADD CONSTRAINT token_issuance_audit_ck CHECK (octet_length(jti_hash)=32 AND outcome='committed' AND expires_at>issued_at AND expires_at<=issued_at+interval '15 minutes' AND (authorization_code_hash IS NULL OR octet_length(authorization_code_hash)=32) AND (authorization_code_id IS NULL OR authorization_code_hash IS NOT NULL) AND (request_correlation_hash IS NULL OR octet_length(request_correlation_hash)=32) AND subject_ref ~ '^identity:(svc:)?[0-9a-f-]{36}$' AND octet_length(client_id) BETWEEN 1 AND 128 AND client_id !~ '[[:cntrl:]]' AND kid !~ '[[:cntrl:]]');
CREATE TABLE oauth_service_credentials (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 service_principal_id uuid NOT NULL REFERENCES oauth_service_principals(id) ON DELETE RESTRICT,
 oauth_client_id uuid NOT NULL REFERENCES oauth_clients(id) ON DELETE RESTRICT,
 secret_hash bytea NOT NULL,
 pepper_version smallint NOT NULL,
 allowed_resources text[] NOT NULL,
 allowed_audiences text[] NOT NULL,
 allowed_scopes text[] NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 rotated_at timestamptz NULL,
 revoked_at timestamptz NULL,
 CONSTRAINT oauth_service_credentials_hash_ck CHECK (octet_length(secret_hash)=32 AND pepper_version>0),
 CONSTRAINT oauth_service_credentials_arrays_ck CHECK (cardinality(allowed_resources) BETWEEN 1 AND 32 AND cardinality(allowed_audiences) BETWEEN 1 AND 32 AND cardinality(allowed_scopes) BETWEEN 1 AND 32),
 CONSTRAINT oauth_service_credentials_uq UNIQUE(service_principal_id,oauth_client_id)
);
CREATE INDEX oauth_service_credentials_client_idx ON oauth_service_credentials(oauth_client_id) WHERE revoked_at IS NULL;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS oauth_service_credentials;
ALTER TABLE token_issuance_audit DROP CONSTRAINT IF EXISTS token_issuance_audit_ck;
ALTER TABLE token_issuance_audit ADD CONSTRAINT token_issuance_audit_ck CHECK (octet_length(jti_hash)=32 AND outcome='committed' AND expires_at>issued_at AND expires_at<=issued_at+interval '15 minutes' AND (authorization_code_hash IS NULL OR octet_length(authorization_code_hash)=32) AND (authorization_code_id IS NULL OR authorization_code_hash IS NOT NULL) AND (request_correlation_hash IS NULL OR octet_length(request_correlation_hash)=32) AND subject_ref ~ '^identity:[0-9a-f-]{36}$' AND octet_length(client_id) BETWEEN 1 AND 128 AND client_id !~ '[[:cntrl:]]' AND kid !~ '[[:cntrl:]]');
ALTER TABLE oauth_grants DROP CONSTRAINT IF EXISTS oauth_grants_service_principal_fk;
DROP TABLE IF EXISTS oauth_service_principals;
-- +goose StatementEnd
