-- +goose Up
-- +goose StatementBegin
CREATE TABLE webhook_events (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 provider varchar(32) NOT NULL DEFAULT 'stytch_b2b',
 provider_project_id varchar(255) NOT NULL,
 provider_event_id varchar(255) NOT NULL,
 svix_message_id varchar(255) NOT NULL,
 event_type varchar(255) NOT NULL,
 source varchar(64) NOT NULL,
 object_type varchar(64) NOT NULL,
 action varchar(64) NOT NULL,
 entity_id varchar(255) NOT NULL,
 event_timestamp timestamptz NOT NULL,
 body_sha256 bytea NOT NULL,
 received_at timestamptz NOT NULL DEFAULT now(),
 signature_verified_at timestamptz NOT NULL,
 status varchar(24) NOT NULL DEFAULT 'received',
 lease_owner varchar(128), lease_token uuid, lease_expires_at timestamptz,
 next_attempt_at timestamptz NOT NULL DEFAULT now(), attempt_count integer NOT NULL DEFAULT 0,
 max_attempts integer NOT NULL DEFAULT 8, claimed_at timestamptz, applied_at timestamptz,
 terminal_at timestamptz, last_error_code varchar(64), version bigint NOT NULL DEFAULT 0,
 CONSTRAINT webhook_events_hash_ck CHECK (octet_length(body_sha256)=32),
 CONSTRAINT webhook_events_status_ck CHECK (status IN ('received','claimed','applied','ignored','failed_retryable','dead_letter')),
 CONSTRAINT webhook_events_attempts_ck CHECK (attempt_count BETWEEN 0 AND 8 AND max_attempts=8),
 CONSTRAINT webhook_events_lease_ck CHECK ((status='claimed') = (lease_owner IS NOT NULL AND lease_token IS NOT NULL AND lease_expires_at IS NOT NULL)),
 CONSTRAINT webhook_events_state_ck CHECK (
   (status='received' AND attempt_count=0 AND claimed_at IS NULL AND applied_at IS NULL AND terminal_at IS NULL AND last_error_code IS NULL)
   OR (status='claimed' AND claimed_at IS NOT NULL AND applied_at IS NULL AND terminal_at IS NULL)
   OR (status='failed_retryable' AND attempt_count BETWEEN 1 AND 7 AND claimed_at IS NULL AND applied_at IS NULL AND terminal_at IS NULL AND last_error_code IS NOT NULL)
   OR (status IN ('applied','ignored') AND claimed_at IS NULL AND applied_at IS NOT NULL AND terminal_at IS NOT NULL)
   OR (status='dead_letter' AND attempt_count=8 AND claimed_at IS NULL AND applied_at IS NULL AND terminal_at IS NOT NULL AND last_error_code IS NOT NULL)
 ),
 CONSTRAINT webhook_events_event_uq UNIQUE(provider,provider_project_id,provider_event_id),
 CONSTRAINT webhook_events_svix_uq UNIQUE(provider,svix_message_id)
);
CREATE INDEX webhook_events_claim_idx ON webhook_events(next_attempt_at,received_at) WHERE status IN ('received','failed_retryable');
CREATE INDEX webhook_events_reclaim_idx ON webhook_events(lease_expires_at) WHERE status='claimed';
CREATE TABLE webhook_security_events (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 event_id_match_webhook_event_id uuid REFERENCES webhook_events(id) ON DELETE RESTRICT,
 svix_id_match_webhook_event_id uuid REFERENCES webhook_events(id) ON DELETE RESTRICT,
 provider varchar(32) NOT NULL, provider_project_id varchar(255) NOT NULL,
 presented_event_id varchar(255) NOT NULL, presented_svix_message_id varchar(255) NOT NULL,
 original_body_sha256 bytea NOT NULL, presented_body_sha256 bytea NOT NULL,
 collision_fingerprint bytea NOT NULL, reason_code varchar(64) NOT NULL,
 occurred_at timestamptz NOT NULL DEFAULT now(),
 CONSTRAINT webhook_security_events_hash_ck CHECK (octet_length(original_body_sha256)=32 AND octet_length(presented_body_sha256)=32 AND octet_length(collision_fingerprint)=32),
 CONSTRAINT webhook_security_events_reason_ck CHECK (reason_code IN ('event_id_reused_new_svix_id','svix_id_reused_new_event_id','cross_id_collision','body_hash_mismatch')),
 CONSTRAINT webhook_security_events_match_ck CHECK (num_nonnulls(event_id_match_webhook_event_id,svix_id_match_webhook_event_id)>=1),
 CONSTRAINT webhook_security_events_fingerprint_uq UNIQUE(collision_fingerprint)
);
CREATE INDEX webhook_security_events_lookup_idx ON webhook_security_events(provider,provider_project_id,presented_event_id,occurred_at);
CREATE TABLE webhook_security_collision_observations (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), webhook_security_event_id uuid NOT NULL REFERENCES webhook_security_events(id) ON DELETE RESTRICT,
 observation_fingerprint bytea NOT NULL, reason_code varchar(64) NOT NULL,
 presented_svix_message_id varchar(255) NOT NULL,
 event_id_match_webhook_event_id uuid REFERENCES webhook_events(id) ON DELETE RESTRICT,
 svix_id_match_webhook_event_id uuid REFERENCES webhook_events(id) ON DELETE RESTRICT,
 occurred_at timestamptz NOT NULL DEFAULT now(),
 CONSTRAINT webhook_security_collision_observations_hash_ck CHECK (octet_length(observation_fingerprint)=32),
 CONSTRAINT webhook_security_collision_observations_reason_ck CHECK (reason_code IN ('event_id_reused_new_svix_id','svix_id_reused_new_event_id','cross_id_collision','body_hash_mismatch')),
 CONSTRAINT webhook_security_collision_observations_match_ck CHECK (num_nonnulls(event_id_match_webhook_event_id,svix_id_match_webhook_event_id)>=1),
 CONSTRAINT webhook_security_collision_observations_uq UNIQUE(observation_fingerprint)
);
CREATE TABLE webhook_security_alerts (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), webhook_security_event_id uuid NOT NULL REFERENCES webhook_security_events(id) ON DELETE RESTRICT,
 status varchar(24) NOT NULL DEFAULT 'pending', lease_owner varchar(128), lease_token uuid, lease_expires_at timestamptz,
 next_attempt_at timestamptz NOT NULL DEFAULT now(), attempt_count integer NOT NULL DEFAULT 0, max_attempts integer NOT NULL DEFAULT 8,
 claimed_at timestamptz, alerted_at timestamptz, terminal_at timestamptz, last_error_code varchar(64), version bigint NOT NULL DEFAULT 0,
 CONSTRAINT webhook_security_alerts_event_uq UNIQUE(webhook_security_event_id),
 CONSTRAINT webhook_security_alerts_status_ck CHECK (status IN ('pending','claimed','failed_retryable','alerted','dead_letter')),
 CONSTRAINT webhook_security_alerts_attempts_ck CHECK (attempt_count BETWEEN 0 AND 8 AND max_attempts=8),
 CONSTRAINT webhook_security_alerts_lease_ck CHECK ((status='claimed') = (lease_owner IS NOT NULL AND lease_token IS NOT NULL AND lease_expires_at IS NOT NULL)),
 CONSTRAINT webhook_security_alerts_state_ck CHECK (
   (status='pending' AND attempt_count=0 AND claimed_at IS NULL AND alerted_at IS NULL AND terminal_at IS NULL AND last_error_code IS NULL)
   OR (status='claimed' AND claimed_at IS NOT NULL AND alerted_at IS NULL AND terminal_at IS NULL)
   OR (status='failed_retryable' AND attempt_count BETWEEN 1 AND 7 AND claimed_at IS NULL AND alerted_at IS NULL AND terminal_at IS NULL AND last_error_code IS NOT NULL)
   OR (status='alerted' AND claimed_at IS NULL AND alerted_at IS NOT NULL AND terminal_at IS NOT NULL)
   OR (status='dead_letter' AND attempt_count=8 AND claimed_at IS NULL AND alerted_at IS NULL AND terminal_at IS NOT NULL AND last_error_code IS NOT NULL)
 )
);
CREATE INDEX webhook_security_alerts_claim_idx ON webhook_security_alerts(next_attempt_at,id) WHERE status IN ('pending','failed_retryable');
CREATE INDEX webhook_security_alerts_reclaim_idx ON webhook_security_alerts(lease_expires_at) WHERE status='claimed';
CREATE TABLE oauth_revocations (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), grant_id uuid REFERENCES oauth_grants(id) ON DELETE RESTRICT,
 refresh_family_id uuid REFERENCES oauth_refresh_families(id) ON DELETE RESTRICT,
 provider_session_association_id uuid REFERENCES provider_session_associations(id) ON DELETE RESTRICT,
 source varchar(32) NOT NULL, reason_code varchar(64) NOT NULL,
 webhook_event_id uuid REFERENCES webhook_events(id) ON DELETE RESTRICT, created_at timestamptz NOT NULL DEFAULT now(),
 CONSTRAINT oauth_revocations_target_ck CHECK (num_nonnulls(grant_id,refresh_family_id,provider_session_association_id)=1),
 CONSTRAINT oauth_revocations_source_ck CHECK (source IN ('user','client','provider_webhook','admin','reuse_detection','expiry')),
 CONSTRAINT oauth_revocations_webhook_ck CHECK ((source='provider_webhook') = (webhook_event_id IS NOT NULL))
);
CREATE UNIQUE INDEX oauth_revocations_webhook_grant_uq ON oauth_revocations(webhook_event_id,grant_id,reason_code) WHERE webhook_event_id IS NOT NULL AND grant_id IS NOT NULL;
CREATE UNIQUE INDEX oauth_revocations_webhook_family_uq ON oauth_revocations(webhook_event_id,refresh_family_id,reason_code) WHERE webhook_event_id IS NOT NULL AND refresh_family_id IS NOT NULL;
CREATE UNIQUE INDEX oauth_revocations_webhook_association_uq ON oauth_revocations(webhook_event_id,provider_session_association_id,reason_code) WHERE webhook_event_id IS NOT NULL AND provider_session_association_id IS NOT NULL;
CREATE TABLE identity_audit_events (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), occurred_at timestamptz NOT NULL DEFAULT now(),
 actor_class varchar(32) NOT NULL, actor_ref varchar(255), action varchar(128) NOT NULL,
 target_type varchar(64) NOT NULL, target_id uuid, outcome varchar(32) NOT NULL, reason_code varchar(64),
 oauth_client_id uuid REFERENCES oauth_clients(id) ON DELETE RESTRICT, client_id varchar(128), audience varchar(128),
 request_correlation_hash bytea, webhook_event_id uuid REFERENCES webhook_events(id) ON DELETE RESTRICT,
 CONSTRAINT identity_audit_events_actor_ck CHECK (actor_class IN ('human','service','provider_webhook','system','admin')),
 CONSTRAINT identity_audit_events_outcome_ck CHECK (outcome IN ('success','denied','failed','replayed','ignored','quarantined')),
 CONSTRAINT identity_audit_events_hash_ck CHECK (request_correlation_hash IS NULL OR octet_length(request_correlation_hash)=32)
);
CREATE OR REPLACE FUNCTION identity_ib4_append_only() RETURNS trigger AS $$
BEGIN
 RAISE EXCEPTION '% is append-only', TG_TABLE_NAME;
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER webhook_security_events_append_only BEFORE UPDATE OR DELETE ON webhook_security_events FOR EACH ROW EXECUTE FUNCTION identity_ib4_append_only();
CREATE TRIGGER webhook_security_collision_observations_append_only BEFORE UPDATE OR DELETE ON webhook_security_collision_observations FOR EACH ROW EXECUTE FUNCTION identity_ib4_append_only();
CREATE TRIGGER identity_audit_events_append_only BEFORE UPDATE OR DELETE ON identity_audit_events FOR EACH ROW EXECUTE FUNCTION identity_ib4_append_only();
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS identity_audit_events;
DROP TABLE IF EXISTS oauth_revocations;
DROP TABLE IF EXISTS webhook_security_alerts;
DROP TABLE IF EXISTS webhook_security_collision_observations;
DROP TABLE IF EXISTS webhook_security_events;
DROP TABLE IF EXISTS webhook_events;
DROP FUNCTION IF EXISTS identity_ib4_append_only();
-- +goose StatementEnd
