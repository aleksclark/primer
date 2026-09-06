ALTER TABLE management_releases
    ADD COLUMN IF NOT EXISTS manifest_payload bytea NOT NULL DEFAULT ''::bytea,
    ADD COLUMN IF NOT EXISTS signing_key_id text NOT NULL DEFAULT '';

ALTER TABLE management_release_receipts
    DROP CONSTRAINT IF EXISTS management_release_receipts_target_id_fkey;
