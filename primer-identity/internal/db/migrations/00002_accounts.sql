-- +goose Up
-- +goose StatementBegin

-- Identity accounts, external identities, and optional password credentials.
-- Students are intentionally out of scope for Identity v1 (no students table).
-- primary_email is NOT unique and is never used as a login merge key.

CREATE TABLE accounts (
    id                          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    status                      TEXT NOT NULL DEFAULT 'active'
                                CHECK (status IN ('active', 'locked')),
    display_name                TEXT NOT NULL DEFAULT '',
    primary_email               TEXT NULL,
    primary_email_verified_at   TIMESTAMPTZ NULL,
    created_at                  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at                  TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT accounts_display_name_len CHECK (char_length(display_name) <= 200),
    CONSTRAINT accounts_primary_email_len CHECK (
        primary_email IS NULL OR char_length(primary_email) <= 320
    )
);

-- Optional recovery/search aid only — NOT a unique login key.
CREATE INDEX accounts_primary_email_idx ON accounts (lower(primary_email))
    WHERE primary_email IS NOT NULL;

CREATE TABLE external_identities (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id          UUID NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
    provider            TEXT NOT NULL
                        CHECK (provider IN ('google', 'password', 'breakglass')),
    provider_subject    TEXT NOT NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT external_identities_subject_len CHECK (char_length(provider_subject) <= 255),
    CONSTRAINT external_identities_provider_subject_uidx UNIQUE (provider, provider_subject)
);

CREATE INDEX external_identities_account_id_idx ON external_identities (account_id);

CREATE TABLE credentials_password (
    account_id      UUID PRIMARY KEY REFERENCES accounts (id) ON DELETE CASCADE,
    password_hash   TEXT NOT NULL,
    algorithm       TEXT NOT NULL DEFAULT 'argon2id',
    rotated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    disabled_at     TIMESTAMPTZ NULL
);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS credentials_password;
DROP TABLE IF EXISTS external_identities;
DROP TABLE IF EXISTS accounts;
-- +goose StatementEnd
