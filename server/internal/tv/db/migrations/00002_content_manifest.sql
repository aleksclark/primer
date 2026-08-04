-- +goose Up
-- +goose StatementBegin

-- Desired-state content catalog mirrored from curriculum/content-manifest.yaml,
-- plus acquisition status tracked by content-ingest (present/missing/failed).
CREATE TABLE content_manifest_entries (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    slug              TEXT NOT NULL UNIQUE,
    title             TEXT NOT NULL,
    year              INTEGER NOT NULL DEFAULT 0,
    kind              TEXT NOT NULL,
    tmdb_id           INTEGER NOT NULL DEFAULT 0,
    tvdb_id           INTEGER NOT NULL DEFAULT 0,
    url               TEXT NOT NULL DEFAULT '',
    class             TEXT NOT NULL DEFAULT 'entertainment',
    subject_tags      TEXT[] NOT NULL DEFAULT '{}',
    standard_codes    TEXT[] NOT NULL DEFAULT '{}',
    priority          INTEGER NOT NULL DEFAULT 0,
    exclude_episodes  TEXT[] NOT NULL DEFAULT '{}',
    max_episodes      INTEGER NOT NULL DEFAULT 0,
    notes             TEXT NOT NULL DEFAULT '',

    -- Acquisition tracking. content-ingest upserts desired-state fields and
    -- reports presence/attempts; the server flips missing → failed when
    -- attempt/day thresholds are crossed.
    status            TEXT NOT NULL DEFAULT 'missing',
    attempt_count     INTEGER NOT NULL DEFAULT 0,
    first_attempt_at  TIMESTAMPTZ,
    last_attempt_at   TIMESTAMPTZ,
    present_at        TIMESTAMPTZ,
    failed_at         TIMESTAMPTZ,
    last_error        TEXT NOT NULL DEFAULT '',

    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),

    CHECK (kind IN ('movie', 'series', 'youtube_channel', 'youtube_playlist', 'manual')),
    CHECK (class IN ('educational', 'entertainment', 'mixed')),
    CHECK (status IN ('missing', 'present', 'failed', 'manual')),
    CHECK (attempt_count >= 0),
    CHECK (year >= 0),
    CHECK (max_episodes >= 0),
    CHECK (priority >= 0)
);

CREATE INDEX idx_content_manifest_status ON content_manifest_entries(status);
CREATE INDEX idx_content_manifest_kind ON content_manifest_entries(kind);
CREATE INDEX idx_content_manifest_priority ON content_manifest_entries(priority);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS content_manifest_entries;
-- +goose StatementEnd
