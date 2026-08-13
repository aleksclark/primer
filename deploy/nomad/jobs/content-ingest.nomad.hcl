# Authoritative portable jobspec for content-ingest (plan-03 release set).
# Periodic batch writer into shared MooseFS media. Child runs are instances,
# not owned job IDs. Reconciliation registers the parent only and NEVER
# dispatches / force-runs this job.
#
# HANDOFF (S2): pause the periodic scheduler (or wait for a quiet window) and
# prove no child is running BEFORE flipping fleet registry authority. Do not
# pause from this source-only PR.
#
# Image: var.image_content_ingest from images.lock.hcl (digest only).
# Secrets: Nomad Variable nomad/jobs/content-ingest via template env=true.

# ── Release-set var-file surface (images.lock.hcl + env/home.nomadvars.hcl) ──
# Nomad rejects undeclared variables present in -var-file inputs, so every job
# in the release set declares the full surface. Only this job's image/env keys
# are referenced below.

variable "image_primer" {
  type        = string
  description = "Immutable ghcr.io/aleksclark/primer@sha256:... digest ref"
  default     = ""
}

variable "image_primer_tv" {
  type        = string
  description = "Immutable ghcr.io/aleksclark/primer-tv@sha256:... digest ref"
  default     = ""
}

variable "image_content_ingest" {
  type        = string
  description = "Immutable ghcr.io/aleksclark/content-ingest@sha256:... digest ref"
  default     = ""
}

variable "primer_cpu" {
  type    = number
  default = 200
}

variable "primer_memory" {
  type    = number
  default = 128
}

variable "primer_env" {
  type    = string
  default = "production"
}

variable "primer_host" {
  type    = string
  default = "0.0.0.0"
}

variable "primer_cors_origins" {
  type    = string
  default = "https://primer.fleet.clark.team,https://primer.clark.team"
}

variable "primer_tv_cpu" {
  type    = number
  default = 300
}

variable "primer_tv_memory" {
  type    = number
  default = 256
}

variable "primer_tv_env" {
  type    = string
  default = "production"
}

variable "primer_tv_host" {
  type    = string
  default = "0.0.0.0"
}

variable "primer_tv_cors_origins" {
  type    = string
  default = "https://tv.fleet.clark.team,https://tv.clark.team"
}

variable "primer_tv_jellyfin_base_url" {
  type    = string
  default = "https://jellyfin.fleet.clark.team"
}

variable "primer_tv_channel_timezone" {
  type    = string
  default = "America/Chicago"
}

variable "primer_tv_primer_base_url" {
  type    = string
  default = "https://primer.fleet.clark.team"
}

variable "primer_tv_manifest_fail_max_attempts" {
  type    = string
  default = "10"
}

variable "primer_tv_manifest_fail_max_days" {
  type    = string
  default = "14"
}

variable "content_ingest_cpu" {
  type    = number
  default = 1000
}

variable "content_ingest_memory" {
  type    = number
  default = 2048
}

variable "content_ingest_manifest_path" {
  type    = string
  default = "/curriculum/content-manifest.yaml"
}

variable "content_ingest_review_path" {
  type    = string
  default = "/curriculum/content-review.yaml"
}

variable "content_ingest_report_dir" {
  type    = string
  default = "/alloc/logs"
}

variable "content_ingest_radarr_base_url" {
  type    = string
  default = "http://192.168.0.41:7878"
}

variable "content_ingest_radarr_root_folder" {
  type    = string
  default = "/media/movies"
}

variable "content_ingest_radarr_quality_profile_id" {
  type    = string
  default = "4"
}

variable "content_ingest_sonarr_base_url" {
  type    = string
  default = "http://192.168.0.24:8989"
}

variable "content_ingest_sonarr_root_folder" {
  type    = string
  default = "/media/tv"
}

variable "content_ingest_sonarr_quality_profile_id" {
  type    = string
  default = "4"
}

variable "content_ingest_jellyfin_base_url" {
  type    = string
  default = "https://jellyfin.fleet.clark.team"
}

variable "content_ingest_tv_base_url" {
  type    = string
  default = "https://tv.fleet.clark.team/api/v1"
}

variable "content_ingest_ytdlp_output_dir" {
  type    = string
  default = "/media"
}

variable "content_ingest_ytdlp_archive_path" {
  type    = string
  default = "/media/ytdlp-archive.txt"
}

variable "content_ingest_ytdlp_path" {
  type    = string
  default = "yt-dlp"
}

variable "content_ingest_cron" {
  type    = string
  default = "0 */6 * * *"
}

variable "content_ingest_timezone" {
  type    = string
  default = "America/Chicago"
}

job "content-ingest" {
  datacenters = ["home"]
  type        = "batch"

  meta {
    managed_by       = "fleet-pull-reconciler"
    source_repo      = "https://github.com/aleksclark/primer"
    source_path      = "deploy/nomad/jobs/content-ingest.nomad.hcl"
    deployment_owner = "aleks-clark"
    release_set      = "primer"
    # Operator note: pause periodic before S2 writer handoff; never CI-dispatch.
    dispatch_policy = "never-from-reconciler-or-ci"
  }

  periodic {
    crons            = [var.content_ingest_cron]
    prohibit_overlap = true
    time_zone        = var.content_ingest_timezone
  }

  group "content-ingest" {
    count = 1

    # yt-dlp writes into the shared media library; same host volume Radarr/
    # Sonarr/Jellyfin mount from MooseFS.
    volume "moosefs-media" {
      type      = "host"
      source    = "moosefs-media"
      read_only = false
    }

    task "content-ingest" {
      driver = "docker"

      config {
        image   = var.image_content_ingest
        command = "apply"
      }

      volume_mount {
        volume      = "moosefs-media"
        destination = "/media"
        read_only   = false
      }

      env {
        INGEST_MANIFEST_PATH             = var.content_ingest_manifest_path
        INGEST_REVIEW_PATH               = var.content_ingest_review_path
        INGEST_REPORT_DIR                = var.content_ingest_report_dir
        INGEST_RADARR_BASE_URL           = var.content_ingest_radarr_base_url
        INGEST_RADARR_ROOT_FOLDER        = var.content_ingest_radarr_root_folder
        INGEST_RADARR_QUALITY_PROFILE_ID = var.content_ingest_radarr_quality_profile_id
        INGEST_SONARR_BASE_URL           = var.content_ingest_sonarr_base_url
        INGEST_SONARR_ROOT_FOLDER        = var.content_ingest_sonarr_root_folder
        INGEST_SONARR_QUALITY_PROFILE_ID = var.content_ingest_sonarr_quality_profile_id
        INGEST_JELLYFIN_BASE_URL         = var.content_ingest_jellyfin_base_url
        INGEST_TV_BASE_URL               = var.content_ingest_tv_base_url
        INGEST_YTDLP_OUTPUT_DIR          = var.content_ingest_ytdlp_output_dir
        INGEST_YTDLP_ARCHIVE_PATH        = var.content_ingest_ytdlp_archive_path
        INGEST_YTDLP_PATH                = var.content_ingest_ytdlp_path
      }

      # Secret key names only — values from Nomad Variable nomad/jobs/content-ingest.
      template {
        destination = "secrets/content-ingest.env"
        env         = true
        change_mode = "restart"
        data        = <<-EOF
{{- with nomadVar "nomad/jobs/content-ingest" -}}
INGEST_RADARR_API_KEY={{ .ingest_radarr_api_key }}
INGEST_SONARR_API_KEY={{ .ingest_sonarr_api_key }}
INGEST_JELLYFIN_API_KEY={{ .ingest_jellyfin_api_key }}
INGEST_JELLYFIN_USER_ID={{ .ingest_jellyfin_user_id }}
INGEST_TV_ADMIN_KEY={{ .ingest_tv_admin_key }}
{{- end -}}
        EOF
      }

      resources {
        cpu    = var.content_ingest_cpu
        memory = var.content_ingest_memory
      }
    }
  }
}
