-- +goose Up
-- +goose StatementBegin

-- Persistent ES256 signing-key custody for IB2 issuance. Public JWK is stored
-- as JSONB; private material is sealed bytes only. Partial unique indexes allow
-- at most one active and one next key. Schema supports IB7 next→active→retired
-- →destroyed lifecycle constraints; rotate/retire/destroy operations themselves
-- are deferred. Down drops only this table.
--
-- E00 provenance: exact SQL from IB0 data contract
-- agent_docs/plans/stytch-identity-ib0/03-data-state-and-revocation.md
-- (donor 00004 at 53b693cc93c8cccb15109d90bae90811029af893 is concepts-only).
CREATE TABLE signing_keys (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  kid varchar(128) NOT NULL,
  alg varchar(16) NOT NULL DEFAULT 'ES256',
  public_jwk jsonb NOT NULL,
  sealed_private_key bytea NULL,
  key_version smallint NOT NULL,
  status varchar(16) NOT NULL,
  not_before timestamptz NOT NULL,
  not_after timestamptz NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  activated_at timestamptz NULL,
  retired_at timestamptz NULL,
  destroyed_at timestamptz NULL,
  CONSTRAINT signing_keys_kid_uq UNIQUE (kid),
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

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS signing_keys;
-- +goose StatementEnd
