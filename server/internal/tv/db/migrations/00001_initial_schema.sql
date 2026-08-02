-- +goose Up
-- +goose StatementBegin

-- Devices: paired clients (tablet, TV box). Only a hash of the pairing token
-- is stored, so a database leak cannot be replayed against the device API.
CREATE TABLE devices (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name               TEXT NOT NULL,
    kind               TEXT NOT NULL DEFAULT 'tablet',   -- tablet | tv_box
    pairing_code       TEXT NOT NULL DEFAULT '',
    pairing_expires_at TIMESTAMPTZ,
    token_hash         TEXT NOT NULL DEFAULT '',
    paired_at          TIMESTAMPTZ,
    revoked_at         TIMESTAMPTZ,
    last_seen_at       TIMESTAMPTZ,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Pairing codes and token hashes are looked up on every unpaired/paired
-- request, and must not collide. Partial uniqueness keeps the '' default
-- usable for rows in the other state.
CREATE UNIQUE INDEX idx_devices_pairing_code ON devices(pairing_code) WHERE pairing_code <> '';
CREATE UNIQUE INDEX idx_devices_token_hash ON devices(token_hash) WHERE token_hash <> '';
CREATE INDEX idx_devices_kind ON devices(kind);

-- Media items: the curated subset of the Jellyfin library exposed to devices.
CREATE TABLE media_items (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    jellyfin_item_id TEXT NOT NULL UNIQUE,
    title            TEXT NOT NULL,
    sort_title       TEXT NOT NULL DEFAULT '',
    overview         TEXT NOT NULL DEFAULT '',
    class            TEXT NOT NULL DEFAULT 'entertainment', -- educational | entertainment | mixed
    runtime_seconds  INTEGER NOT NULL DEFAULT 0,
    subject_tags     TEXT[] NOT NULL DEFAULT '{}',
    standard_codes   TEXT[] NOT NULL DEFAULT '{}',
    quality_notes    TEXT NOT NULL DEFAULT '',
    container        TEXT NOT NULL DEFAULT '',
    video_codec      TEXT NOT NULL DEFAULT '',
    audio_codec      TEXT NOT NULL DEFAULT '',
    direct_play_ok   BOOLEAN NOT NULL DEFAULT true,
    image_tag        TEXT NOT NULL DEFAULT '',
    orphaned_at      TIMESTAMPTZ,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_media_items_class ON media_items(class);
CREATE INDEX idx_media_items_direct_play ON media_items(direct_play_ok);

-- Availability windows: the on-demand rotation. A media item is offerable
-- while now() falls inside a window.
CREATE TABLE availability_windows (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    media_item_id UUID NOT NULL REFERENCES media_items(id) ON DELETE CASCADE,
    starts_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    ends_at       TIMESTAMPTZ NOT NULL,
    max_plays     INTEGER,
    note          TEXT NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (ends_at > starts_at)
);

CREATE INDEX idx_availability_windows_item ON availability_windows(media_item_id);
CREATE INDEX idx_availability_windows_span ON availability_windows(starts_at, ends_at);

-- Play ledger: watch-once enforcement. One row per (item, window) consumes
-- the play regardless of which device watched it.
CREATE TABLE play_ledger (
    id                     UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    media_item_id          UUID NOT NULL REFERENCES media_items(id) ON DELETE CASCADE,
    device_id              UUID REFERENCES devices(id) ON DELETE SET NULL,
    availability_window_id UUID NOT NULL REFERENCES availability_windows(id) ON DELETE CASCADE,
    consumed_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (media_item_id, availability_window_id)
);

CREATE INDEX idx_play_ledger_window ON play_ledger(availability_window_id);
CREATE INDEX idx_play_ledger_device ON play_ledger(device_id);

-- Schedule entries: the programmed channel grid. Resolution of "what's on
-- now" lands in a later phase; the table exists so admin CRUD can populate it.
CREATE TABLE schedule_entries (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    media_item_id    UUID NOT NULL REFERENCES media_items(id) ON DELETE CASCADE,
    airs_at          TIMESTAMPTZ NOT NULL,
    join_in_progress BOOLEAN NOT NULL DEFAULT true,
    block            TEXT NOT NULL DEFAULT 'morning',  -- morning | midday | afternoon | evening
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (media_item_id, airs_at)
);

CREATE INDEX idx_schedule_entries_airs_at ON schedule_entries(airs_at);
CREATE INDEX idx_schedule_entries_block ON schedule_entries(block);

-- Play grants: single-use, short-lived playback authorizations.
CREATE TABLE play_grants (
    id                     UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    media_item_id          UUID NOT NULL REFERENCES media_items(id) ON DELETE CASCADE,
    device_id              UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    availability_window_id UUID REFERENCES availability_windows(id) ON DELETE SET NULL,
    schedule_entry_id      UUID REFERENCES schedule_entries(id) ON DELETE SET NULL,
    mode                   TEXT NOT NULL DEFAULT 'on_demand', -- on_demand | programmed
    stream_url             TEXT NOT NULL DEFAULT '',
    start_offset_seconds   INTEGER NOT NULL DEFAULT 0,
    issued_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at             TIMESTAMPTZ NOT NULL,
    consumed_at            TIMESTAMPTZ,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_play_grants_device ON play_grants(device_id);
CREATE INDEX idx_play_grants_item ON play_grants(media_item_id);
CREATE INDEX idx_play_grants_expires ON play_grants(expires_at);

-- Playback sessions: the metrics and reporting source, one per consumed grant.
CREATE TABLE playback_sessions (
    id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    grant_id             UUID NOT NULL UNIQUE REFERENCES play_grants(id) ON DELETE CASCADE,
    media_item_id        UUID NOT NULL REFERENCES media_items(id) ON DELETE CASCADE,
    device_id            UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    started_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    ended_at             TIMESTAMPTZ,
    watched_seconds      INTEGER NOT NULL DEFAULT 0,
    max_position_seconds INTEGER NOT NULL DEFAULT 0,
    completed            BOOLEAN NOT NULL DEFAULT false,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_playback_sessions_item ON playback_sessions(media_item_id);
CREATE INDEX idx_playback_sessions_device ON playback_sessions(device_id);
CREATE INDEX idx_playback_sessions_started ON playback_sessions(started_at);

-- Primer reports: idempotency ledger for instructional-hours export. The
-- unique session key is what stops double-reporting after a retry.
CREATE TABLE primer_reports (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    playback_session_id UUID NOT NULL UNIQUE REFERENCES playback_sessions(id) ON DELETE CASCADE,
    reported_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    primer_ref          TEXT NOT NULL DEFAULT '',
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS primer_reports;
DROP TABLE IF EXISTS playback_sessions;
DROP TABLE IF EXISTS play_grants;
DROP TABLE IF EXISTS schedule_entries;
DROP TABLE IF EXISTS play_ledger;
DROP TABLE IF EXISTS availability_windows;
DROP TABLE IF EXISTS media_items;
DROP TABLE IF EXISTS devices;
-- +goose StatementEnd
