# Non-secret home-fleet overlay for the primer release set.
# Secrets MUST NOT appear here — they live in Nomad Variables only.

# ── primer ──────────────────────────────────────────────────────────────────
primer_cpu            = 200
primer_memory         = 128
primer_env            = "production"
primer_host           = "0.0.0.0"
primer_cors_origins   = "https://primer.fleet.clark.team,https://primer.clark.team"

# ── primer-tv ───────────────────────────────────────────────────────────────
primer_tv_cpu                       = 300
primer_tv_memory                    = 256
primer_tv_env                       = "production"
primer_tv_host                      = "0.0.0.0"
primer_tv_cors_origins              = "https://tv.fleet.clark.team,https://tv.clark.team"
primer_tv_jellyfin_base_url         = "https://jellyfin.fleet.clark.team"
primer_tv_channel_timezone          = "America/Chicago"
primer_tv_primer_base_url           = "https://primer.fleet.clark.team"
primer_tv_manifest_fail_max_attempts = "10"
primer_tv_manifest_fail_max_days     = "14"

# ── content-ingest ──────────────────────────────────────────────────────────
content_ingest_cpu                        = 1000
content_ingest_memory                     = 2048
content_ingest_manifest_path              = "/curriculum/content-manifest.yaml"
content_ingest_review_path                = "/curriculum/content-review.yaml"
content_ingest_report_dir                 = "/alloc/logs"
content_ingest_radarr_base_url            = "http://192.168.0.41:7878"
content_ingest_radarr_root_folder         = "/media/movies"
content_ingest_radarr_quality_profile_id  = "4"
content_ingest_sonarr_base_url            = "http://192.168.0.24:8989"
content_ingest_sonarr_root_folder         = "/media/tv"
content_ingest_sonarr_quality_profile_id  = "4"
content_ingest_jellyfin_base_url          = "https://jellyfin.fleet.clark.team"
content_ingest_tv_base_url                = "https://tv.fleet.clark.team/api/v1"
content_ingest_ytdlp_output_dir           = "/media"
content_ingest_ytdlp_archive_path         = "/media/ytdlp-archive.txt"
content_ingest_ytdlp_path                 = "yt-dlp"
content_ingest_cron                       = "0 */6 * * *"
content_ingest_timezone                   = "America/Chicago"
