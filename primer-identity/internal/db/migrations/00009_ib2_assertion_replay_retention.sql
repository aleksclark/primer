-- +goose Up
-- +goose StatementBegin
-- Widen assertion replay retention to cover the private_key_jwt parser accept
-- window (JWT exp + 60s clock skew). expires_at stores retention expiry, not
-- bare JWT exp. consumed_at may equal retention at the skew boundary.
ALTER TABLE oauth_client_assertion_replays
  DROP CONSTRAINT oauth_client_assertion_replays_ck;

ALTER TABLE oauth_client_assertion_replays
  ADD CONSTRAINT oauth_client_assertion_replays_ck CHECK (
    endpoint_kind IN ('token', 'revocation')
    AND octet_length(jti_hash) = 32
    AND expires_at >= consumed_at
    AND expires_at > issued_at
    AND expires_at <= issued_at + interval '5 minutes' + interval '60 seconds'
    AND audience !~ '[[:cntrl:]]'
    AND octet_length(audience) BETWEEN 1 AND 2048
  );
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE oauth_client_assertion_replays
  DROP CONSTRAINT oauth_client_assertion_replays_ck;

ALTER TABLE oauth_client_assertion_replays
  ADD CONSTRAINT oauth_client_assertion_replays_ck CHECK (
    endpoint_kind IN ('token', 'revocation')
    AND octet_length(jti_hash) = 32
    AND expires_at > consumed_at
    AND expires_at > issued_at
    AND expires_at <= issued_at + interval '5 minutes'
    AND audience !~ '[[:cntrl:]]'
    AND octet_length(audience) BETWEEN 1 AND 2048
  );
-- +goose StatementEnd
