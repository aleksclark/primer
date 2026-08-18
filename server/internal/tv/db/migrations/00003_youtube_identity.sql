-- +goose Up
-- +goose StatementBegin
ALTER TABLE media_items
  ADD COLUMN youtube_video_id TEXT NULL,
  ADD COLUMN manifest_slug TEXT NOT NULL DEFAULT '',
  ADD COLUMN episode_key TEXT NOT NULL DEFAULT '',
  ADD COLUMN upload_date DATE NULL,
  ADD COLUMN title_locked BOOLEAN NOT NULL DEFAULT false,
  ADD COLUMN overview_locked BOOLEAN NOT NULL DEFAULT false,
  ADD COLUMN classification_locked BOOLEAN NOT NULL DEFAULT false;

ALTER TABLE media_items
  ADD CONSTRAINT media_items_youtube_video_id_unique
  UNIQUE (youtube_video_id);

ALTER TABLE media_items
  ADD CONSTRAINT media_items_youtube_video_id_format
  CHECK (youtube_video_id IS NULL OR youtube_video_id ~ '^[A-Za-z0-9_-]{11}$');
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE media_items
  DROP CONSTRAINT IF EXISTS media_items_youtube_video_id_format,
  DROP CONSTRAINT IF EXISTS media_items_youtube_video_id_unique,
  DROP COLUMN IF EXISTS classification_locked,
  DROP COLUMN IF EXISTS overview_locked,
  DROP COLUMN IF EXISTS title_locked,
  DROP COLUMN IF EXISTS upload_date,
  DROP COLUMN IF EXISTS episode_key,
  DROP COLUMN IF EXISTS manifest_slug,
  DROP COLUMN IF EXISTS youtube_video_id;
-- +goose StatementEnd
