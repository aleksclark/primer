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
# Persist reports beyond allocation garbage collection (outside the scanned Shows subtree).
content_ingest_report_dir                 = "/media/primer/.ingest-reports"
content_ingest_radarr_base_url            = "https://radarr.fleet.clark.team"
content_ingest_radarr_root_folder         = "/media/movies"
content_ingest_radarr_quality_profile_id  = "4"
content_ingest_sonarr_base_url            = "https://sonarr.fleet.clark.team"
content_ingest_sonarr_root_folder         = "/media/tv"
content_ingest_sonarr_quality_profile_id  = "4"
content_ingest_jellyfin_base_url          = "https://jellyfin.fleet.clark.team"
content_ingest_jellyfin_collection_name   = "Primer"
content_ingest_tv_base_url                = "https://tv.fleet.clark.team/api/v1"
# Keep outside /media/tv: Jellyfin needs Shows/<slug> directly below a configured media path.
# Dedicated Primer Sources library scans /media/primer/Shows with remote metadata/subtitles disabled.
content_ingest_ytdlp_output_dir           = "/media/primer"
# Deprecated/unused — per-show archives at Shows/<slug>/.ytdlp-archive.txt.
content_ingest_ytdlp_archive_path         = ""
content_ingest_ytdlp_path                 = "yt-dlp"
# Cookies path: content_ingest_ytdlp_cookies_path is declared only on the
# content-ingest jobspec (default ""). Do not add it here until primer and
# primer-tv jobspecs also declare the key (Nomad rejects undeclared -var-file keys).
content_ingest_cron                       = "0 */6 * * *"
content_ingest_timezone                   = "America/Chicago"
