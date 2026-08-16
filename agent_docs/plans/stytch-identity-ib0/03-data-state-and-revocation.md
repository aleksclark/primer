# 03 — Candidate durable data, state machines, and revocation contract

**Status: STOP — candidate contract under independent exact-tip review.** This is SQL-shaped contract prose, **not** a migration or implementation authorization. IB1 must independently review this exact tip with zero findings before any implementation begins.

## Global rules

All tables are additive in Identity PostgreSQL. All timestamps are `timestamptz`, stored in UTC; identifiers named `id` are `uuid`; binary HMAC/SHA-256 values are exactly `bytea` length 32. A versioned secret hash is `HMAC-SHA-256(pepper-version || context || secret)`. No table stores a raw Stytch token, SessionJWT, callback artifact, provider payload, email join key, product role, product membership, plaintext OAuth state, authorization code, refresh token, client secret, assertion, or private signing key.

Internal relationships use `oauth_client_id uuid`; the public client identifier is only `oauth_clients.client_id varchar(128)`. All delete actions are `RESTRICT`/soft-disable unless explicitly stated below: audit, revocation, security, and issuance evidence must never be cascade-deleted.

## SQL-shaped schema contract

```sql
CREATE TABLE oauth_clients (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  client_id varchar(128) NOT NULL UNIQUE,
  name varchar(200) NOT NULL,
  client_type varchar(16) NOT NULL,
  token_endpoint_auth_method varchar(32) NOT NULL,
  client_secret_hash bytea NULL, client_secret_pepper_version smallint NULL,
  allowed_grants text[] NOT NULL,
  enabled boolean NOT NULL DEFAULT true,
  created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
  disabled_at timestamptz NULL,
  CONSTRAINT oauth_clients_auth_method_value_ck CHECK (token_endpoint_auth_method IN ('none','client_secret_basic','private_key_jwt')),
  CONSTRAINT oauth_clients_type_ck CHECK (client_type IN ('public','confidential')),
  CONSTRAINT oauth_clients_grant_count_ck CHECK (cardinality(allowed_grants) BETWEEN 1 AND 3),
  CONSTRAINT oauth_clients_client_id_ck CHECK (client_id !~ '[[:cntrl:]]'),
  CONSTRAINT oauth_clients_grants_null_ck CHECK (array_position(allowed_grants, NULL) IS NULL),
  CONSTRAINT oauth_clients_allowed_grants_ck CHECK (allowed_grants <@ ARRAY['authorization_code','refresh_token','client_credentials']::text[]),
  CONSTRAINT oauth_clients_enabled_ck CHECK ((enabled AND disabled_at IS NULL) OR (NOT enabled AND disabled_at IS NOT NULL)),
  CONSTRAINT oauth_clients_auth_method_ck CHECK ((
    (client_type='public' AND token_endpoint_auth_method='none' AND client_secret_hash IS NULL AND client_secret_pepper_version IS NULL)
    OR (client_type='confidential' AND token_endpoint_auth_method='client_secret_basic' AND client_secret_hash IS NOT NULL AND octet_length(client_secret_hash)=32 AND client_secret_pepper_version IS NOT NULL AND client_secret_pepper_version>0)
    OR (client_type='confidential' AND token_endpoint_auth_method='private_key_jwt' AND client_secret_hash IS NULL AND client_secret_pepper_version IS NULL)
  ) IS TRUE)
);

CREATE TABLE oauth_client_redirects (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  oauth_client_id uuid NOT NULL REFERENCES oauth_clients(id) ON DELETE RESTRICT,
  redirect_uri varchar(2048) NOT NULL, resource_uri varchar(2048) NOT NULL,
  audience varchar(128) NOT NULL, allowed_scopes text[] NOT NULL,
  enabled boolean NOT NULL DEFAULT true,
  created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT oauth_client_redirects_tuple_uq UNIQUE (oauth_client_id,redirect_uri,resource_uri,audience),
  CONSTRAINT oauth_client_redirects_values_ck CHECK ((octet_length(redirect_uri) BETWEEN 1 AND 2048 AND octet_length(resource_uri) BETWEEN 1 AND 2048 AND octet_length(audience) BETWEEN 1 AND 128 AND redirect_uri !~ '[[:cntrl:]]' AND resource_uri !~ '[[:cntrl:]]' AND audience !~ '[[:cntrl:]]') IS TRUE),
  CONSTRAINT oauth_client_redirects_scopes_ck CHECK (cardinality(allowed_scopes) BETWEEN 1 AND 32 AND array_position(allowed_scopes,NULL) IS NULL)
);

CREATE TABLE oauth_client_keys (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  oauth_client_id uuid NOT NULL REFERENCES oauth_clients(id) ON DELETE RESTRICT,
  kid varchar(128) NOT NULL, jwk_json jsonb NOT NULL, alg varchar(16) NOT NULL DEFAULT 'ES256',
  use varchar(16) NOT NULL DEFAULT 'sig', enabled boolean NOT NULL DEFAULT true,
  not_before timestamptz NULL, not_after timestamptz NULL,
  created_at timestamptz NOT NULL DEFAULT now(), disabled_at timestamptz NULL,
  CONSTRAINT oauth_client_keys_client_kid_uq UNIQUE (oauth_client_id,kid),
  CONSTRAINT oauth_client_keys_profile_ck CHECK (alg='ES256' AND use='sig' AND kid !~ '[[:cntrl:]]'),
  CONSTRAINT oauth_client_keys_lifetime_ck CHECK (not_after IS NULL OR not_before IS NULL OR not_after > not_before)
);

CREATE TABLE broker_transactions (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  oauth_client_id uuid NOT NULL REFERENCES oauth_clients(id) ON DELETE RESTRICT,
  redirect_id uuid NOT NULL REFERENCES oauth_client_redirects(id) ON DELETE RESTRICT,
  state_hash bytea NOT NULL, state_pepper_version smallint NOT NULL,
  state_sealed bytea NULL, state_key_version smallint NOT NULL, state_length smallint NOT NULL,
  provider_code_sealed bytea NULL, provider_code_key_version smallint NULL,
  pkce_challenge char(43) NOT NULL, pkce_method varchar(8) NOT NULL DEFAULT 'S256', requested_scopes text[] NOT NULL,
  resource_uri varchar(2048) NOT NULL, audience varchar(128) NOT NULL,
  broker_cookie_hash bytea NOT NULL, broker_cookie_pepper_version smallint NOT NULL,
  status varchar(24) NOT NULL DEFAULT 'pending', failure_code varchar(64) NULL,
  account_id uuid NULL REFERENCES accounts(id) ON DELETE RESTRICT,
  provider_session_association_id uuid NULL,
  created_at timestamptz NOT NULL DEFAULT now(), expires_at timestamptz NOT NULL,
  provider_started_at timestamptz NULL, provider_validating_at timestamptz NULL,
  completed_at timestamptz NULL, version bigint NOT NULL DEFAULT 0,
  CONSTRAINT broker_transactions_state_hash_len CHECK (octet_length(state_hash)=32),
  CONSTRAINT broker_transactions_cookie_hash_len CHECK (octet_length(broker_cookie_hash)=32),
  CONSTRAINT broker_transactions_state_size CHECK (state_length BETWEEN 1 AND 1024 AND (state_sealed IS NULL OR octet_length(state_sealed) BETWEEN 30 AND 1053)),
  CONSTRAINT broker_transactions_state_versions CHECK (state_pepper_version > 0 AND state_key_version > 0),
  CONSTRAINT broker_transactions_provider_code_ck CHECK ((provider_code_sealed IS NULL) = (provider_code_key_version IS NULL)),
  CONSTRAINT broker_transactions_pkce_ck CHECK (pkce_method='S256' AND pkce_challenge ~ '^[A-Za-z0-9_-]{43}$'),
  CONSTRAINT broker_transactions_status CHECK (status IN ('pending','provider_started','provider_validating','authorized','denied','failed','expired')),
  CONSTRAINT broker_transactions_expiry CHECK (expires_at > created_at AND expires_at <= created_at + interval '10 minutes'),
  CONSTRAINT broker_transactions_terminal_erasure_ck CHECK (
    (status IN ('pending','provider_started','provider_validating') AND state_sealed IS NOT NULL)
    OR (status IN ('authorized','denied','failed','expired') AND state_sealed IS NULL AND provider_code_sealed IS NULL)
  ),
  CONSTRAINT broker_transactions_human_account_assoc_xor_ck CHECK ((
    (account_id IS NULL AND provider_session_association_id IS NULL)
    OR (account_id IS NOT NULL AND provider_session_association_id IS NOT NULL)
  ) IS TRUE)
);
CREATE UNIQUE INDEX broker_transactions_active_state_uq ON broker_transactions(state_hash)
  WHERE status IN ('pending','provider_started','provider_validating');
CREATE UNIQUE INDEX broker_transactions_active_cookie_uq ON broker_transactions(broker_cookie_hash)
  WHERE status IN ('pending','provider_started','provider_validating');
CREATE INDEX broker_transactions_expiry_idx ON broker_transactions(expires_at);

CREATE TABLE provider_session_associations (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(), account_id uuid NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
  stytch_mapping_id uuid NOT NULL REFERENCES stytch_mappings(id) ON DELETE RESTRICT,
  provider varchar(32) NOT NULL DEFAULT 'stytch_b2b', provider_project_id varchar(255) NOT NULL,
  provider_organization_id varchar(255) NOT NULL, provider_member_id varchar(255) NOT NULL,
  provider_member_session_id varchar(255) NOT NULL, provider_expires_at timestamptz NOT NULL,
  status varchar(16) NOT NULL DEFAULT 'active', last_validated_at timestamptz NOT NULL,
  revoked_at timestamptz NULL, revoke_reason_code varchar(64) NULL,
  created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT provider_session_associations_uq UNIQUE (provider,provider_project_id,provider_member_session_id),
  CONSTRAINT provider_session_associations_id_account_uq UNIQUE (id,account_id),
  CONSTRAINT provider_session_associations_status_ck CHECK (provider='stytch_b2b' AND status IN ('active','revoked','expired','suspended')),
  CONSTRAINT provider_session_associations_ids_ck CHECK (provider_project_id !~ '[[:cntrl:]]' AND provider_organization_id !~ '[[:cntrl:]]' AND provider_member_id !~ '[[:cntrl:]]' AND provider_member_session_id !~ '[[:cntrl:]]'),
  CONSTRAINT provider_session_associations_terminal_ck CHECK ((status='active' AND revoked_at IS NULL AND revoke_reason_code IS NULL) OR (status<>'active' AND revoked_at IS NOT NULL AND revoke_reason_code IS NOT NULL))
);
CREATE INDEX provider_session_associations_tuple_idx ON provider_session_associations(provider,provider_project_id,provider_organization_id,provider_member_id);
ALTER TABLE broker_transactions ADD CONSTRAINT broker_transactions_association_fk
  FOREIGN KEY (provider_session_association_id) REFERENCES provider_session_associations(id) ON DELETE RESTRICT;
ALTER TABLE broker_transactions ADD CONSTRAINT broker_transactions_association_account_fk
  FOREIGN KEY (provider_session_association_id, account_id)
  REFERENCES provider_session_associations(id, account_id)
  ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED;

CREATE TABLE oauth_grants (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(), account_id uuid NULL REFERENCES accounts(id) ON DELETE RESTRICT,
  service_principal_id uuid NULL,
  oauth_client_id uuid NOT NULL REFERENCES oauth_clients(id) ON DELETE RESTRICT,
  provider_session_association_id uuid NULL REFERENCES provider_session_associations(id) ON DELETE RESTRICT,
  resource_uri varchar(2048) NOT NULL, audience varchar(128) NOT NULL, scopes text[] NOT NULL,
  subject_class varchar(16) NOT NULL, status varchar(16) NOT NULL DEFAULT 'active',
  granted_at timestamptz NOT NULL DEFAULT now(), not_after timestamptz NOT NULL,
  revoked_at timestamptz NULL, revoke_reason_code varchar(64) NULL, version bigint NOT NULL DEFAULT 0,
  CONSTRAINT oauth_grants_class_status_ck CHECK (subject_class IN ('human','service') AND status IN ('active','revoked','expired')),
  CONSTRAINT oauth_grants_subject_xor_ck CHECK ((
    (subject_class='human' AND account_id IS NOT NULL AND service_principal_id IS NULL AND provider_session_association_id IS NOT NULL)
    OR (subject_class='service' AND account_id IS NULL AND service_principal_id IS NOT NULL AND provider_session_association_id IS NULL)
  ) IS TRUE),
  CONSTRAINT oauth_grants_lifetime_ck CHECK (not_after>granted_at),
  CONSTRAINT oauth_grants_terminal_ck CHECK ((status='active' AND revoked_at IS NULL AND revoke_reason_code IS NULL) OR (status<>'active' AND revoked_at IS NOT NULL AND revoke_reason_code IS NOT NULL))
);
CREATE UNIQUE INDEX oauth_grants_active_human_uq ON oauth_grants(account_id,oauth_client_id,provider_session_association_id,resource_uri,audience) WHERE subject_class='human' AND status='active';
CREATE UNIQUE INDEX oauth_grants_active_service_uq ON oauth_grants(service_principal_id,oauth_client_id,resource_uri,audience) WHERE subject_class='service' AND status='active';
ALTER TABLE oauth_grants ADD CONSTRAINT oauth_grants_association_account_fk
  FOREIGN KEY (provider_session_association_id, account_id)
  REFERENCES provider_session_associations(id, account_id)
  ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED;

CREATE TABLE oauth_authorization_codes (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(), code_hash bytea NOT NULL, pepper_version smallint NOT NULL,
  grant_id uuid NOT NULL REFERENCES oauth_grants(id) ON DELETE RESTRICT,
  broker_transaction_id uuid NOT NULL REFERENCES broker_transactions(id) ON DELETE RESTRICT,
  oauth_client_id uuid NOT NULL REFERENCES oauth_clients(id) ON DELETE RESTRICT,
  redirect_uri varchar(2048) NOT NULL, resource_uri varchar(2048) NOT NULL, audience varchar(128) NOT NULL,
  scopes text[] NOT NULL, pkce_challenge char(43) NOT NULL, pkce_method varchar(8) NOT NULL DEFAULT 'S256',
  issued_at timestamptz NOT NULL DEFAULT now(), expires_at timestamptz NOT NULL, consumed_at timestamptz NULL,
  CONSTRAINT oauth_authorization_codes_hash_lifetime_ck CHECK (octet_length(code_hash)=32 AND pepper_version>0 AND expires_at>issued_at AND expires_at<=issued_at+interval '60 seconds'),
  CONSTRAINT oauth_authorization_codes_pkce_ck CHECK (pkce_method='S256' AND pkce_challenge ~ '^[A-Za-z0-9_-]{43}$'),
  CONSTRAINT oauth_authorization_codes_hash_uq UNIQUE(code_hash),
  CONSTRAINT oauth_authorization_codes_broker_uq UNIQUE(broker_transaction_id)
);

CREATE TABLE oauth_refresh_families (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(), grant_id uuid NOT NULL REFERENCES oauth_grants(id) ON DELETE RESTRICT,
  oauth_client_id uuid NOT NULL REFERENCES oauth_clients(id) ON DELETE RESTRICT,
  resource_uri varchar(2048) NOT NULL, status varchar(24) NOT NULL DEFAULT 'active',
  absolute_expires_at timestamptz NOT NULL, idle_expires_at timestamptz NOT NULL, last_rotated_at timestamptz NOT NULL DEFAULT now(),
  revoked_at timestamptz NULL, expired_at timestamptz NULL, reuse_detected_at timestamptz NULL,
  revoke_reason_code varchar(64) NULL, version bigint NOT NULL DEFAULT 0,
  CONSTRAINT oauth_refresh_families_status_lifetime_ck CHECK ((status IN ('active','revoked','expired','reuse_detected') AND absolute_expires_at>last_rotated_at AND idle_expires_at>last_rotated_at AND idle_expires_at<=absolute_expires_at) IS TRUE),
  CONSTRAINT oauth_refresh_families_state_ck CHECK ((
    (status='active' AND revoked_at IS NULL AND expired_at IS NULL AND reuse_detected_at IS NULL AND revoke_reason_code IS NULL)
    OR (status='revoked' AND revoked_at IS NOT NULL AND expired_at IS NULL AND reuse_detected_at IS NULL AND revoke_reason_code IS NOT NULL)
    OR (status='expired' AND revoked_at IS NULL AND expired_at IS NOT NULL AND reuse_detected_at IS NULL AND revoke_reason_code IS NOT NULL)
    OR (status='reuse_detected' AND revoked_at IS NOT NULL AND expired_at IS NULL AND reuse_detected_at IS NOT NULL AND revoke_reason_code IS NOT NULL)
  ) IS TRUE)
);
CREATE TABLE oauth_refresh_tokens (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(), family_id uuid NOT NULL REFERENCES oauth_refresh_families(id) ON DELETE RESTRICT,
  token_hash bytea NOT NULL, pepper_version smallint NOT NULL, sequence bigint NOT NULL,
  issued_at timestamptz NOT NULL DEFAULT now(), expires_at timestamptz NOT NULL, consumed_at timestamptz NULL, revoked_at timestamptz NULL,
  replaced_by_id uuid NULL REFERENCES oauth_refresh_tokens(id) ON DELETE RESTRICT, reuse_detected_at timestamptz NULL,
  CONSTRAINT oauth_refresh_tokens_hash_lifetime_ck CHECK (octet_length(token_hash)=32 AND pepper_version>0 AND sequence>=0 AND expires_at>issued_at),
  CONSTRAINT oauth_refresh_tokens_replace_ck CHECK (replaced_by_id IS NULL OR replaced_by_id<>id),
  CONSTRAINT oauth_refresh_tokens_state_ck CHECK ((
    (consumed_at IS NULL AND revoked_at IS NULL AND replaced_by_id IS NULL AND reuse_detected_at IS NULL)
    OR (consumed_at IS NOT NULL AND revoked_at IS NULL AND replaced_by_id IS NOT NULL)
    OR (consumed_at IS NULL AND revoked_at IS NOT NULL AND replaced_by_id IS NULL AND reuse_detected_at IS NULL)
  ) IS TRUE),
  CONSTRAINT oauth_refresh_tokens_time_ck CHECK ((consumed_at IS NULL OR consumed_at>=issued_at) AND (revoked_at IS NULL OR revoked_at>=issued_at) AND (reuse_detected_at IS NULL OR (consumed_at IS NOT NULL AND replaced_by_id IS NOT NULL AND reuse_detected_at>=consumed_at))),
  CONSTRAINT oauth_refresh_tokens_hash_uq UNIQUE(token_hash), CONSTRAINT oauth_refresh_tokens_family_sequence_uq UNIQUE(family_id,sequence),
  CONSTRAINT oauth_refresh_tokens_replaced_by_uq UNIQUE(replaced_by_id)
);
CREATE UNIQUE INDEX oauth_refresh_tokens_one_live_uq ON oauth_refresh_tokens(family_id) WHERE consumed_at IS NULL AND revoked_at IS NULL;

CREATE TABLE oauth_client_assertion_replays (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(), oauth_client_id uuid NOT NULL REFERENCES oauth_clients(id) ON DELETE RESTRICT,
  endpoint_kind varchar(16) NOT NULL, jti_hash bytea NOT NULL, expires_at timestamptz NOT NULL, consumed_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT oauth_client_assertion_replays_ck CHECK (endpoint_kind IN ('token','revocation') AND octet_length(jti_hash)=32 AND expires_at>consumed_at),
  CONSTRAINT oauth_client_assertion_replays_uq UNIQUE(oauth_client_id,endpoint_kind,jti_hash)
);
CREATE INDEX oauth_client_assertion_replays_expiry_idx ON oauth_client_assertion_replays(expires_at);

CREATE TABLE signing_keys (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(), kid varchar(128) NOT NULL UNIQUE,
  alg varchar(16) NOT NULL DEFAULT 'ES256', public_jwk jsonb NOT NULL, sealed_private_key bytea NULL,
  key_version smallint NOT NULL, status varchar(16) NOT NULL, not_before timestamptz NOT NULL,
  not_after timestamptz NULL, created_at timestamptz NOT NULL DEFAULT now(),
  activated_at timestamptz NULL, retired_at timestamptz NULL, destroyed_at timestamptz NULL,
  CONSTRAINT signing_keys_profile_ck CHECK (alg='ES256' AND key_version>0 AND status IN ('next','active','retired','destroyed') AND kid !~ '[[:cntrl:]]'),
  CONSTRAINT signing_keys_lifetime_ck CHECK (not_after IS NULL OR not_after>not_before),
  CONSTRAINT signing_keys_status_material_ck CHECK ((
    (status='next' AND activated_at IS NULL AND retired_at IS NULL AND destroyed_at IS NULL AND sealed_private_key IS NOT NULL AND octet_length(sealed_private_key)>0)
    OR (status='active' AND activated_at IS NOT NULL AND retired_at IS NULL AND destroyed_at IS NULL AND sealed_private_key IS NOT NULL AND octet_length(sealed_private_key)>0 AND activated_at>=created_at)
    OR (status='retired' AND activated_at IS NOT NULL AND retired_at IS NOT NULL AND destroyed_at IS NULL AND sealed_private_key IS NOT NULL AND octet_length(sealed_private_key)>0 AND retired_at>=activated_at)
    OR (status='destroyed' AND activated_at IS NOT NULL AND retired_at IS NOT NULL AND destroyed_at IS NOT NULL AND sealed_private_key IS NULL AND destroyed_at>=retired_at)
  ) IS TRUE)
);
CREATE UNIQUE INDEX signing_keys_one_active_uq ON signing_keys(status) WHERE status='active';
CREATE UNIQUE INDEX signing_keys_one_next_uq ON signing_keys(status) WHERE status='next';

CREATE TABLE token_issuance_audit (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(), grant_id uuid NOT NULL REFERENCES oauth_grants(id) ON DELETE RESTRICT,
  authorization_code_id uuid NULL REFERENCES oauth_authorization_codes(id) ON DELETE SET NULL,
  authorization_code_hash bytea NULL,
  signing_key_id uuid NOT NULL REFERENCES signing_keys(id) ON DELETE RESTRICT,
  jti_hash bytea NOT NULL, audience varchar(128) NOT NULL, issued_at timestamptz NOT NULL DEFAULT now(), expires_at timestamptz NOT NULL,
  outcome varchar(24) NOT NULL DEFAULT 'committed', request_correlation_hash bytea NULL,
  CONSTRAINT token_issuance_audit_ck CHECK (octet_length(jti_hash)=32 AND outcome='committed' AND expires_at>issued_at AND (authorization_code_hash IS NULL OR octet_length(authorization_code_hash)=32) AND (authorization_code_id IS NULL OR authorization_code_hash IS NOT NULL) AND (request_correlation_hash IS NULL OR octet_length(request_correlation_hash)=32))
);

CREATE TABLE oauth_service_principals (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(), subject_ref varchar(160) NOT NULL UNIQUE,
  display_name varchar(200) NOT NULL, enabled boolean NOT NULL DEFAULT true,
  created_at timestamptz NOT NULL DEFAULT now(), disabled_at timestamptz NULL,
  CONSTRAINT oauth_service_principals_subject_ck CHECK (subject_ref = 'identity:svc:' || id::text),
  CONSTRAINT oauth_service_principals_enabled_ck CHECK ((enabled AND disabled_at IS NULL) OR (NOT enabled AND disabled_at IS NOT NULL))
);
ALTER TABLE oauth_grants ADD CONSTRAINT oauth_grants_service_principal_fk
  FOREIGN KEY (service_principal_id) REFERENCES oauth_service_principals(id) ON DELETE RESTRICT;
CREATE TABLE oauth_service_credentials (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(), service_principal_id uuid NOT NULL REFERENCES oauth_service_principals(id) ON DELETE RESTRICT,
  oauth_client_id uuid NOT NULL REFERENCES oauth_clients(id) ON DELETE RESTRICT,
  secret_hash bytea NULL, pepper_version smallint NULL, public_key_id uuid NULL REFERENCES oauth_client_keys(id) ON DELETE RESTRICT,
  allowed_resources text[] NOT NULL, allowed_audiences text[] NOT NULL, allowed_scopes text[] NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(), rotated_at timestamptz NULL, revoked_at timestamptz NULL,
  CONSTRAINT oauth_service_credentials_auth_xor_ck CHECK (((secret_hash IS NOT NULL AND octet_length(secret_hash)=32 AND pepper_version IS NOT NULL AND pepper_version>0 AND public_key_id IS NULL) OR (secret_hash IS NULL AND pepper_version IS NULL AND public_key_id IS NOT NULL)) IS TRUE)
);

CREATE TABLE webhook_events (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(), provider varchar(32) NOT NULL DEFAULT 'stytch_b2b',
  provider_project_id varchar(255) NOT NULL, provider_event_id varchar(255) NOT NULL, svix_message_id varchar(255) NOT NULL,
  event_type varchar(255) NOT NULL, source varchar(64) NOT NULL, object_type varchar(64) NOT NULL, action varchar(64) NOT NULL,
  entity_id varchar(255) NOT NULL, event_timestamp timestamptz NOT NULL, body_sha256 bytea NOT NULL,
  received_at timestamptz NOT NULL DEFAULT now(), signature_verified_at timestamptz NOT NULL,
  status varchar(24) NOT NULL DEFAULT 'received', lease_owner varchar(128) NULL, lease_token uuid NULL, lease_expires_at timestamptz NULL,
  next_attempt_at timestamptz NOT NULL DEFAULT now(), attempt_count integer NOT NULL DEFAULT 0, max_attempts integer NOT NULL DEFAULT 8,
  claimed_at timestamptz NULL, applied_at timestamptz NULL, terminal_at timestamptz NULL, last_error_code varchar(64) NULL, version bigint NOT NULL DEFAULT 0,
  CONSTRAINT webhook_events_hash_status_ck CHECK (octet_length(body_sha256)=32 AND status IN ('received','claimed','applied','ignored','failed_retryable','dead_letter')),
  CONSTRAINT webhook_events_attempts_ck CHECK (attempt_count BETWEEN 0 AND 8 AND max_attempts=8),
  CONSTRAINT webhook_events_lease_xor_ck CHECK ((status='claimed') = (lease_owner IS NOT NULL AND lease_token IS NOT NULL AND lease_expires_at IS NOT NULL)),
  CONSTRAINT webhook_events_state_ck CHECK ((
    (status='received' AND attempt_count=0 AND claimed_at IS NULL AND applied_at IS NULL AND terminal_at IS NULL AND last_error_code IS NULL)
    OR (status='claimed' AND claimed_at IS NOT NULL AND applied_at IS NULL AND terminal_at IS NULL)
    OR (status='failed_retryable' AND attempt_count BETWEEN 1 AND 7 AND claimed_at IS NULL AND applied_at IS NULL AND terminal_at IS NULL AND last_error_code IS NOT NULL)
    OR (status IN ('applied','ignored') AND claimed_at IS NULL AND applied_at IS NOT NULL AND terminal_at IS NOT NULL)
    OR (status='dead_letter' AND attempt_count=8 AND claimed_at IS NULL AND applied_at IS NULL AND terminal_at IS NOT NULL AND last_error_code IS NOT NULL)
  ) IS TRUE),
  CONSTRAINT webhook_events_event_uq UNIQUE(provider,provider_project_id,provider_event_id),
  CONSTRAINT webhook_events_svix_uq UNIQUE(provider,svix_message_id)
);
CREATE INDEX webhook_events_claim_idx ON webhook_events(next_attempt_at,received_at) WHERE status IN ('received','failed_retryable');
CREATE INDEX webhook_events_reclaim_idx ON webhook_events(lease_expires_at) WHERE status='claimed';
CREATE TABLE webhook_security_events (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  event_id_match_webhook_event_id uuid NULL REFERENCES webhook_events(id) ON DELETE RESTRICT,
  svix_id_match_webhook_event_id uuid NULL REFERENCES webhook_events(id) ON DELETE RESTRICT,
  provider varchar(32) NOT NULL, provider_project_id varchar(255) NOT NULL, presented_event_id varchar(255) NOT NULL,
  presented_svix_message_id varchar(255) NOT NULL, original_body_sha256 bytea NOT NULL, presented_body_sha256 bytea NOT NULL,
  collision_fingerprint bytea NOT NULL, reason_code varchar(64) NOT NULL, occurred_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT webhook_security_events_hash_ck CHECK ((octet_length(collision_fingerprint)=32 AND original_body_sha256 IS NOT NULL AND octet_length(original_body_sha256)=32 AND octet_length(presented_body_sha256)=32) IS TRUE),
  CONSTRAINT webhook_security_events_reason_ck CHECK (reason_code IN ('event_id_reused_new_svix_id','svix_id_reused_new_event_id','cross_id_collision','body_hash_mismatch')),
  CONSTRAINT webhook_security_events_match_ck CHECK (num_nonnulls(event_id_match_webhook_event_id,svix_id_match_webhook_event_id)>=1),
  CONSTRAINT webhook_security_events_fingerprint_uq UNIQUE(collision_fingerprint)
);
CREATE INDEX webhook_security_events_lookup_idx ON webhook_security_events(provider,provider_project_id,presented_event_id,occurred_at);
CREATE TABLE webhook_security_alerts (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(), webhook_security_event_id uuid NOT NULL REFERENCES webhook_security_events(id) ON DELETE RESTRICT,
  status varchar(24) NOT NULL DEFAULT 'pending', lease_owner varchar(128) NULL, lease_token uuid NULL, lease_expires_at timestamptz NULL,
  next_attempt_at timestamptz NOT NULL DEFAULT now(), attempt_count integer NOT NULL DEFAULT 0, max_attempts integer NOT NULL DEFAULT 8,
  claimed_at timestamptz NULL, alerted_at timestamptz NULL, terminal_at timestamptz NULL, last_error_code varchar(64) NULL, version bigint NOT NULL DEFAULT 0,
  CONSTRAINT webhook_security_alerts_event_uq UNIQUE(webhook_security_event_id),
  CONSTRAINT webhook_security_alerts_status_ck CHECK (status IN ('pending','claimed','failed_retryable','alerted','dead_letter')),
  CONSTRAINT webhook_security_alerts_attempts_ck CHECK (attempt_count BETWEEN 0 AND 8 AND max_attempts=8),
  CONSTRAINT webhook_security_alerts_lease_ck CHECK (((status='claimed')=(lease_owner IS NOT NULL AND lease_token IS NOT NULL AND lease_expires_at IS NOT NULL)) IS TRUE),
  CONSTRAINT webhook_security_alerts_state_ck CHECK ((
    (status='pending' AND attempt_count=0 AND claimed_at IS NULL AND alerted_at IS NULL AND terminal_at IS NULL AND last_error_code IS NULL)
    OR (status='claimed' AND claimed_at IS NOT NULL AND alerted_at IS NULL AND terminal_at IS NULL)
    OR (status='failed_retryable' AND attempt_count BETWEEN 1 AND 7 AND claimed_at IS NULL AND alerted_at IS NULL AND terminal_at IS NULL AND last_error_code IS NOT NULL)
    OR (status='alerted' AND claimed_at IS NULL AND alerted_at IS NOT NULL AND terminal_at IS NOT NULL)
    OR (status='dead_letter' AND attempt_count=8 AND claimed_at IS NULL AND alerted_at IS NULL AND terminal_at IS NOT NULL AND last_error_code IS NOT NULL)
  ) IS TRUE)
);
CREATE INDEX webhook_security_alerts_claim_idx ON webhook_security_alerts(next_attempt_at,id) WHERE status IN ('pending','failed_retryable');
CREATE INDEX webhook_security_alerts_reclaim_idx ON webhook_security_alerts(lease_expires_at) WHERE status='claimed';
CREATE TABLE webhook_security_collision_observations (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(), webhook_security_event_id uuid NOT NULL REFERENCES webhook_security_events(id) ON DELETE RESTRICT,
  observation_fingerprint bytea NOT NULL, reason_code varchar(64) NOT NULL, presented_svix_message_id varchar(255) NOT NULL,
  event_id_match_webhook_event_id uuid NULL REFERENCES webhook_events(id) ON DELETE RESTRICT,
  svix_id_match_webhook_event_id uuid NULL REFERENCES webhook_events(id) ON DELETE RESTRICT,
  occurred_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT webhook_security_collision_observations_hash_ck CHECK (octet_length(observation_fingerprint)=32),
  CONSTRAINT webhook_security_collision_observations_reason_ck CHECK (reason_code IN ('event_id_reused_new_svix_id','svix_id_reused_new_event_id','cross_id_collision','body_hash_mismatch')),
  CONSTRAINT webhook_security_collision_observations_match_ck CHECK (num_nonnulls(event_id_match_webhook_event_id,svix_id_match_webhook_event_id)>=1),
  CONSTRAINT webhook_security_collision_observations_uq UNIQUE(observation_fingerprint)
);

CREATE TABLE oauth_revocations (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(), grant_id uuid NULL REFERENCES oauth_grants(id) ON DELETE RESTRICT,
  refresh_family_id uuid NULL REFERENCES oauth_refresh_families(id) ON DELETE RESTRICT,
  provider_session_association_id uuid NULL REFERENCES provider_session_associations(id) ON DELETE RESTRICT,
  source varchar(32) NOT NULL, reason_code varchar(64) NOT NULL,
  webhook_event_id uuid NULL REFERENCES webhook_events(id) ON DELETE RESTRICT, created_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT oauth_revocations_source_ck CHECK (source IN ('user','client','provider_webhook','admin','reuse_detection','expiry')),
  CONSTRAINT oauth_revocations_target_xor_ck CHECK (num_nonnulls(grant_id,refresh_family_id,provider_session_association_id)=1),
  CONSTRAINT oauth_revocations_webhook_source_ck CHECK ((source='provider_webhook')=(webhook_event_id IS NOT NULL))
);
CREATE UNIQUE INDEX oauth_revocations_webhook_grant_uq ON oauth_revocations(webhook_event_id,grant_id,reason_code) WHERE webhook_event_id IS NOT NULL AND grant_id IS NOT NULL;
CREATE UNIQUE INDEX oauth_revocations_webhook_family_uq ON oauth_revocations(webhook_event_id,refresh_family_id,reason_code) WHERE webhook_event_id IS NOT NULL AND refresh_family_id IS NOT NULL;
CREATE UNIQUE INDEX oauth_revocations_webhook_association_uq ON oauth_revocations(webhook_event_id,provider_session_association_id,reason_code) WHERE webhook_event_id IS NOT NULL AND provider_session_association_id IS NOT NULL;
CREATE TABLE identity_audit_events (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(), occurred_at timestamptz NOT NULL DEFAULT now(),
  actor_class varchar(32) NOT NULL, actor_ref varchar(255) NULL, action varchar(128) NOT NULL,
  target_type varchar(64) NOT NULL, target_id uuid NULL, outcome varchar(32) NOT NULL, reason_code varchar(64) NULL,
  oauth_client_id uuid NULL REFERENCES oauth_clients(id) ON DELETE RESTRICT, client_id varchar(128) NULL, audience varchar(128) NULL,
  request_correlation_hash bytea NULL, webhook_event_id uuid NULL REFERENCES webhook_events(id) ON DELETE RESTRICT,
  CONSTRAINT identity_audit_events_actor_ck CHECK (actor_class IN ('human','service','provider_webhook','system','admin')),
  CONSTRAINT identity_audit_events_outcome_ck CHECK (outcome IN ('success','denied','failed','replayed','ignored','quarantined')),
  CONSTRAINT identity_audit_events_hash_ck CHECK (request_correlation_hash IS NULL OR octet_length(request_correlation_hash)=32)
);

-- The following table is owned by Studio PostgreSQL, not Identity PostgreSQL;
-- Identity UUIDs are immutable bound values, never cross-database foreign keys.
CREATE TABLE mcp_mrtr_confirmations (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(), request_state_hash bytea NOT NULL UNIQUE,
  human_subject_ref varchar(128) NOT NULL, client_id varchar(128) NOT NULL,
  workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE RESTRICT,
  draft_digest bytea NOT NULL, tool_name varchar(128) NOT NULL DEFAULT 'studio.publish.confirm',
  issued_jsonrpc_id_hash bytea NOT NULL, consumed_jsonrpc_id_hash bytea NULL,
  issued_at timestamptz NOT NULL DEFAULT now(), expires_at timestamptz NOT NULL, consumed_at timestamptz NULL, expired_at timestamptz NULL,
  CONSTRAINT mcp_mrtr_confirmations_ck CHECK (octet_length(request_state_hash)=32 AND octet_length(draft_digest)=32 AND octet_length(issued_jsonrpc_id_hash)=32 AND (consumed_jsonrpc_id_hash IS NULL OR octet_length(consumed_jsonrpc_id_hash)=32) AND human_subject_ref ~ '^identity:[0-9a-f-]{36}$' AND octet_length(client_id) BETWEEN 1 AND 128 AND client_id !~ '[[:cntrl:]]' AND tool_name='studio.publish.confirm' AND expires_at>issued_at AND expires_at<=issued_at+interval '5 minutes'),
  CONSTRAINT mcp_mrtr_confirmations_consume_ck CHECK (((consumed_at IS NULL AND expired_at IS NULL AND consumed_jsonrpc_id_hash IS NULL) OR (consumed_at IS NOT NULL AND expired_at IS NULL AND consumed_jsonrpc_id_hash IS NOT NULL AND consumed_jsonrpc_id_hash<>issued_jsonrpc_id_hash) OR (consumed_at IS NULL AND expired_at IS NOT NULL AND consumed_jsonrpc_id_hash IS NULL)) IS TRUE)
);
CREATE UNIQUE INDEX mcp_mrtr_confirmations_live_request_uq ON mcp_mrtr_confirmations(human_subject_ref,client_id,workspace_id,draft_digest,tool_name) WHERE consumed_at IS NULL AND expired_at IS NULL;
```

### Wave-owned migration gates

The schema above is a final-shape contract, not one IB1 migration. Each implementation wave tests only the tables and constraints it introduces:

- **IB1:** broker transactions and sealed-state integrity, provider-session associations and tuple/account consistency, human grants and their composite account/association FK, and authorization-code issuance. Its fresh/upgrade/down gate must not require signing-key, refresh, webhook, alert, revocation, or later audit tables.
- **IB2:** signing-key custody needed for initial ES256 issuance, client assertion replay, initial refresh family/current-token issuance, and token-issuance audit. IB2 proves at-most-one `active`/`next` only because it creates `signing_keys`; rotation through `retired`/`destroyed` remains IB7.
- **IB4:** webhook receipts, collision observations/evidence, security-alert delivery, revocations, Identity audit, and all worker claim/reclaim indexes and lease fences.
- **IB6:** complete refresh rotation/reuse/terminal lifecycle plus long-lived refresh/grant/revocation/audit retention.
- **IB7:** complete signing-key `next → active → retired → destroyed` lifecycle, rotation, timestamp ordering, and private-material destruction.

Every wave that creates a retained audit/evidence FK owns its delete-action and retention proof. In particular, IB2 owns `token_issuance_audit_code_fk`, the copied `authorization_code_hash`, nullable `ON DELETE SET NULL`, and the 400-day issuance-evidence proof; IB1 does not create or gate that future table.

### Exact foreign-key and cross-row constraint names

Every compact inline `REFERENCES` above is emitted with an exact named FK and no cascade. All are `ON DELETE RESTRICT` except `token_issuance_audit_code_fk`, which is nullable `ON DELETE SET NULL` because authorization codes are purged after 24 hours while issuance audit/grants are retained for 400 days; the immutable `authorization_code_hash` copy preserves non-secret evidence. Exact `RESTRICT` names are: `oauth_client_redirects_oauth_client_fk`, `oauth_client_keys_oauth_client_fk`, `broker_transactions_oauth_client_fk`, `broker_transactions_redirect_fk`, `broker_transactions_account_fk`, `broker_transactions_association_fk`, `broker_transactions_association_account_fk`, `provider_session_associations_account_fk`, `provider_session_associations_stytch_mapping_fk`, `oauth_grants_account_fk`, `oauth_grants_oauth_client_fk`, `oauth_grants_association_fk`, `oauth_grants_association_account_fk`, `oauth_grants_service_principal_fk`, `oauth_authorization_codes_grant_fk`, `oauth_authorization_codes_broker_fk`, `oauth_authorization_codes_oauth_client_fk`, `oauth_refresh_families_grant_fk`, `oauth_refresh_families_oauth_client_fk`, `oauth_refresh_tokens_family_fk`, `oauth_refresh_tokens_replaced_by_fk`, `oauth_client_assertion_replays_oauth_client_fk`, `token_issuance_audit_grant_fk`, `token_issuance_audit_signing_key_fk`, `oauth_service_credentials_principal_fk`, `oauth_service_credentials_oauth_client_fk`, `oauth_service_credentials_public_key_fk`, `webhook_security_events_event_match_fk`, `webhook_security_events_svix_match_fk`, `webhook_security_alerts_event_fk`, `webhook_security_collision_observations_security_event_fk`, `webhook_security_collision_observations_event_match_fk`, `webhook_security_collision_observations_svix_match_fk`, `oauth_revocations_grant_fk`, `oauth_revocations_family_fk`, `oauth_revocations_association_fk`, `oauth_revocations_webhook_fk`, `identity_audit_events_oauth_client_fk`, `identity_audit_events_webhook_fk`, and Studio-owned `mcp_mrtr_confirmations_workspace_fk`. Fresh/upgrade schema tests assert exact names, targets, nullability, delete actions, and copied evidence from `pg_constraint` and real failing inserts. `provider_session_associations_id_account_uq` is the documented unique `(id, account_id)` target. `oauth_grants_association_account_fk` and `broker_transactions_association_account_fk` are the required account-pair authority FKs: `ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED` on `(provider_session_association_id, account_id)`. They are not replaced by triggers. The existing single-column `broker_transactions_association_fk` / `oauth_grants_association_fk` remain as id-only RESTRICT references; they do not authorize a cross-account pair. Wrong-account grant/transaction inserts and migrations must fail the composite FKs. NULL-safe XOR checks are written `(...) IS TRUE`: human grants require account plus provider association and no service principal; service grants require a service principal and no account/provider association; broker rows carry account and provider association together or neither.

`provider_session_associations_stytch_mapping_tuple_ck` is a named deferrable constraint trigger. On insert/update it locks the referenced `stytch_mappings` row and requires identical account, project, organization, and member values; a mapping/session-account mismatch cannot commit. `oauth_clients_private_key_presence_ck` is a named deferrable constraint trigger requiring at least one enabled, currently valid `oauth_client_keys` row for each enabled `private_key_jwt` client. Scope/grant arrays are stored only in stable sorted unique canonical form; named constraint triggers `oauth_clients_grants_canonical_ck`, `oauth_client_redirects_scopes_canonical_ck`, `oauth_grants_scopes_canonical_ck`, `oauth_authorization_codes_scopes_canonical_ck`, and `oauth_service_credentials_authority_canonical_ck` reject blank/control-bearing/duplicate/unsorted values and enforce at most 32 elements of at most 128 bytes each. These are database constraints, not application-only validation.

`identity_audit_events`, `webhook_security_events`, and `webhook_security_collision_observations` are append-only by database ownership: the runtime role has `INSERT,SELECT` only; named append-only triggers reject `UPDATE`/`DELETE`. `webhook_security_alerts` is the separate mutable delivery ledger. Retention deletion runs under a separately audited maintenance role only after the stated retention window. `stytch_mappings.account_id` must be forward-fixed from cascade deletion to `ON DELETE RESTRICT` before association evidence is added.

`provider_session_associations` is the only durable provider-session correlation. `ProviderMemberSessionID` is an IB1 snapshot-adapter addition: valid UTF-8, no controls, ≤255 runes/1024 bytes before insertion into the narrower `varchar(255)` correlation policy. There is deliberately **no separate OAuth-state nonce column**: the nonce is embedded in `state_sealed`.

`oauth_refresh_family_lifecycle_ck` is a named deferrable constraint trigger. An active family has exactly one current token (`consumed_at IS NULL AND revoked_at IS NULL`); a terminal family has none. Rotation locks the family/current row, marks the parent consumed, inserts one same-family successor with sequence `n+1`, and sets `replaced_by_id` atomically. A consumed row has a successor; a current or revoked row has none. Reuse may be recorded only on a consumed row with a successor and atomically changes the family to `reuse_detected`, records both reuse timestamps, and revokes the grant and every live family token. Expiry/revocation clears current-token authority. The partial unique index is only the at-most-one guard; this trigger supplies existence and cross-row successor/status invariants.

Signing-key custody matches the selected donor lifecycle contract without merging donor status. Unique partial indexes `signing_keys_one_active_uq` and `signing_keys_one_next_uq` allow at most one `active` and at most one `next` row. Status/timestamp/private-material checks are exact: `next` has sealed private material and no activation/retirement/destruction timestamps; `active` has sealed private material and `activated_at`; `retired` keeps sealed private material and `retired_at>=activated_at`; `destroyed` has `destroyed_at>=retired_at` and `sealed_private_key IS NULL`. Activation, retirement, and destruction occur only in that order. Migration/constraint tests must reject a second active or next key, private ciphertext after destruction, and out-of-order timestamps.

## State, callback, and issuance transactions

### Recoverable OAuth state

`state-seal-v1` is exact. Input `state` is 1..1024 bytes. AES-256-GCM uses a 32-byte key selected only by positive `state_key_version`, a fresh cryptographically random 12-byte nonce, and its 16-byte tag. Stored bytes are exactly `0x01 || nonce[12] || ciphertext || tag[16]`; therefore valid stored length is 30..1053 bytes. AAD is exactly: ASCII bytes `primer.oauth.state` (18 bytes), byte `0x01`, broker transaction UUID as 16 RFC 4122/network-order raw bytes, OAuth-client UUID likewise, then registered redirect URI, resource URI, and audience in that order, each encoded as uint32 big-endian byte length followed by strict UTF-8 bytes. The text bounds remain 2048, 2048, and 128 bytes and total AAD is capped at 4287 bytes. This framing is normative; delimiter-free concatenation and alternate UUID/string encodings are forbidden.

Rotation makes one key version encrypt-active and prior 32-byte keys decrypt-only for at least transaction TTL plus skew. Decrypt selects the recorded key version and requires envelope byte `0x01`; it never guesses or falls back. `state_hash` supports lookup/replay protection; only `state_sealed` enables recovery. Unknown envelope/key version, removed key, malformed UTF-8/length/envelope, nonce/ciphertext/tag tamper, expiry, or any AAD mismatch fails closed. Required vectors include: two redirect/resource field splits that would collide under raw concatenation but produce different AAD; unknown envelope and key versions; one-bit nonce/ciphertext/tag/AAD tamper; old-key decrypt during overlap then failure after safe retirement; and every authorized/denied/failed/expired terminal path proving `state_sealed` NULL plus the in-memory plaintext buffer explicitly zeroed.

Registration checks use `octet_length`, not PostgreSQL character count, so the 2048/2048/128 byte bounds and 4287-byte AAD cap hold for every accepted strict UTF-8 value.

State progression is `pending → provider_started → provider_validating → authorized`; `denied|failed|expired` are terminal. The callback decrypts only in memory after provider validation and, in **one transaction**, creates the association/grant plus only the HMAC-hashed Primer authorization-code row, transitions any nonterminal broker transaction to its terminal outcome, and nulls both `state_sealed` and any transient `provider_code_sealed`. It commits before it redirects. Only after commit does the callback construct the exact original state for the redirect and, on success only, return the newly generated plaintext Primer code; neither plaintext value is persisted or recoverable for response replay.

### Provider revalidation

IB1 exposes a vendor-neutral `RevalidateMemberSession(ctx, exactProjectID, exactOrganizationID, exactMemberID, exactMemberSessionID)` adapter. Its Stytch implementation is exactly one unpaginated official B2B Get Sessions call (`GET /v1/b2b/sessions` with exact `organization_id` and `member_id`). Official Go SDK **v18.1.0** is `client.Sessions.Get(ctx, &sessions.GetParams{OrganizationID: exactOrganizationID, MemberID: exactMemberID})` from `github.com/stytchauth/stytch-go/v18/stytch/b2b/sessions`; `GetParams` has only those two fields and the response is `*sessions.GetResponse` with `MemberSessions []sessions.MemberSession`. There is no cursor, page, or limit. IB1 must qualify that exact method/params/response plus a bounded 1 MiB transport body; if it cannot or the pin drifts, **STOP**. Bounds are a 2-second total deadline, a 1 MiB response-body cap, and at most 256 returned `member_sessions`. Success requires exactly one byte-equal active, unexpired stored `member_session_id` with the exact tuple. Timeout/cancellation/429/5xx is unavailable. Missing exact stored `member_session_id` or inactive/expired is definitive denial. More than 256 sessions, duplicate IDs, body over cap, malformed payload, or tuple mismatch is unavailable/fail-closed. No stale or negative success exists. A positive proof cache is ≤15 seconds and contains no raw token.

### Code exchange / JWT

After every code, client, redirect, resource, PKCE, grant, and provider proof check, code exchange locks the code in a short transaction. It uses an already-ready local in-memory ES256 signer to sign **before commit**, appends `token_issuance_audit`, consumes the code, and commits atomically. Signing failure rolls back; commit failure discards the signed token; the HTTP response occurs only after commit. A lost response leaves the code consumed and requires authorization restart—there is no persisted token response or reusable code.

### Webhook worker

The HTTP handler only verifies, extracts bounded envelope fields, writes the receipt or immutable security event plus alert item, commits, and returns 204. It never follows up with Stytch or changes authority. The main worker CAS-claims due `received|failed_retryable` rows or expired leases by version plus fresh `lease_owner`, `lease_token`, and lease expiry. Success/failure/dead-letter finalization predicates on the same token and version; zero rows is stale-worker denial. Retry delays are exactly 1m, 5m, 15m, 1h, 4h, 8h, 12h; failed attempt eight becomes `dead_letter`. Reclaim uses a new token/fence.

An exact duplicate exists only when `provider_event_id` and `svix_message_id` lookups both resolve to the same receipt and that receipt's body hash matches. These cases are distinct quarantines even when one hash matches: `event_id_reused_new_svix_id` (provider event ID reused with a new Svix ID), `svix_id_reused_new_event_id` (Svix ID reused with a new provider event ID), `cross_id_collision` (the two IDs resolve to different rows), and `body_hash_mismatch` (same row pair, different body). Both lookup matches are recorded. For fingerprinting, `original_hash` is the provider-event-ID match hash when that match exists, otherwise the Svix-ID match hash; this rule also deterministically selects the provider-event-ID row for a cross-ID collision. The required semantic key is exactly `collision_fingerprint = SHA-256(ASCII "primer.webhook.collision" || 0x01 || LP(provider) || LP(project) || LP(event_id) || LP(original_hash) || LP(new_hash) || LP(reason_code))`, where every `LP` is uint32 big-endian byte length followed by strict UTF-8 bytes for strings or raw bytes for the two 32-byte hashes. A separate immutable observation key is `SHA-256(ASCII "primer.webhook.collision.observation" || 0x01 || LP(collision_fingerprint) || LP(reason_code) || LP(presented_svix_message_id) || LP(event_match_uuid_or_empty) || LP(svix_match_uuid_or_empty))`; it preserves distinct collision classes and transport/match evidence without changing the required semantic idempotency key. Repeated identical observation, including concurrent repeats, yields one semantic event, one observation row, and exactly one `webhook_security_alerts` row, with no authority effect. Alert delivery uses the same claim/reclaim, seven-delay schedule, attempt-eight dead-letter, token+version finalization, restart recovery, and stale-token denial as the main worker, so a crash cannot strand an alert. Collisions never overwrite original receipts or apply authority. Generic `ON CONFLICT DO NOTHING` receipt dedupe is forbidden because it cannot establish these cases.

MRTR confirmation accepts only JSON-RPC string IDs whose strict UTF-8 encoding is 1..128 bytes and contains no controls. Both ID hashes are exactly `SHA-256(ASCII "primer.mcp.jsonrpc-id" || 0x01 || uint32be(length) || UTF8(id))`; numeric, null, object/array, overlong, and malformed IDs fail before state change. Studio confirmation rows store the public OAuth `client_id` string (`varchar(128)`) and compare it to the validated JWT `client_id` claim; they never bind an internal OAuth-client UUID. Wrong or missing `client_id` fails closed with a sanitized audit field. An initial confirm transaction first locks and marks every matching unconsumed row with `expires_at<=now()` as `expired_at=now()` with sanitized audit, then inserts replacement state when required. Consume requires `expired_at IS NULL`, `expires_at>now()`, and a distinct canonical ID hash. Expiry terminalization and replacement issuance are atomic, so the live unique slot cannot strand; re-issue-after-expiry and concurrent expiry/reissue are required E2E cases.

## Retention and nonclaims

Authorization codes are deleted before their broker transactions, and both are hard-deleted after 24h. No 400-day grant/audit/security row has a non-nullable or `RESTRICT` FK back to either. When IB2 creates `token_issuance_audit`, it owns proof that `authorization_code_id` is nullable `ON DELETE SET NULL`, `authorization_code_hash` preserves non-secret evidence, and 400-day issuance evidence survives the short-lived code purge; IB1 has no dependency on that future table. Any later retained evidence that references a short-lived row must be proved by its creating wave with the same nullable-SET-NULL-plus-copied-evidence pattern. IB1 owns retention only for the human grant/association rows it creates; IB4 owns webhook receipt/security/alert/revocation/audit retention; IB6 owns long-lived refresh evidence and its grant/revocation/audit FK-safe purge behavior. Those retained rows live for 400 days subject to approved policy; there is no family-lifetime+30d purge that can conflict with retained grant/audit FKs. Signing public metadata is retained; encrypted key custody follows IB7. Existing offline access JWTs can remain valid until their ≤15-minute `exp`; this document does not claim immediate distributed JWT revocation.
