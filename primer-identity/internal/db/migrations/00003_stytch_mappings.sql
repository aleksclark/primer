-- +goose Up
-- +goose StatementBegin

-- A normalized immutable provider principal key. Email and profile fields do
-- not belong here: the exact three-column tuple owns account identity.
CREATE TABLE stytch_mappings (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id          UUID NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
    project_id          TEXT NOT NULL,
    organization_id     TEXT NOT NULL,
    member_id           TEXT NOT NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT stytch_mappings_project_id_ck
        CHECK (char_length(project_id) > 0 AND char_length(project_id) <= 255),
    CONSTRAINT stytch_mappings_organization_id_ck
        CHECK (char_length(organization_id) > 0 AND char_length(organization_id) <= 255),
    CONSTRAINT stytch_mappings_member_id_ck
        CHECK (char_length(member_id) > 0 AND char_length(member_id) <= 255),
    CONSTRAINT stytch_mappings_tuple_uidx
        UNIQUE (project_id, organization_id, member_id)
);

CREATE INDEX stytch_mappings_account_id_idx ON stytch_mappings (account_id);
CREATE INDEX stytch_mappings_project_org_idx
    ON stytch_mappings (project_id, organization_id, member_id);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS stytch_mappings;
-- +goose StatementEnd
