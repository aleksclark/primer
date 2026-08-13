# Authoritative portable jobspec for primer-tv (plan-03 release set).
# Image: var.image_primer_tv from images.lock.hcl (digest only).
# Secrets: Nomad Variable nomad/jobs/primer-tv via template env=true.
# source_revision is injected by the fleet pull reconciler at render time.

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

job "primer-tv" {
  datacenters = ["home"]
  type        = "service"

  meta {
    managed_by       = "fleet-pull-reconciler"
    source_repo      = "https://github.com/aleksclark/primer"
    source_path      = "deploy/nomad/jobs/primer-tv.nomad.hcl"
    deployment_owner = "aleks-clark"
    release_set      = "primer"
  }

  group "primer-tv" {
    count = 1

    network {
      port "http" {}
    }

    service {
      name     = "primer-tv"
      port     = "http"
      provider = "nomad"

      tags = [
        "traefik.enable=true",
        "traefik.http.routers.primer-tv.rule=Host(`tv.fleet.clark.team`) || Host(`tv.clark.team`)",
        "traefik.http.routers.primer-tv.entrypoints=websecure",
        "traefik.http.routers.primer-tv.tls.certresolver=letsencrypt",
      ]

      check {
        type     = "http"
        path     = "/api/v1/health"
        interval = "30s"
        timeout  = "5s"
      }
    }

    task "primer-tv" {
      driver = "docker"

      config {
        image = var.image_primer_tv
        ports = ["http"]
      }

      env {
        TV_PORT                       = "${NOMAD_PORT_http}"
        TV_HOST                       = var.primer_tv_host
        TV_ENV                        = var.primer_tv_env
        TV_CORS_ORIGINS               = var.primer_tv_cors_origins
        TV_JELLYFIN_BASE_URL          = var.primer_tv_jellyfin_base_url
        TV_CHANNEL_TIMEZONE           = var.primer_tv_channel_timezone
        TV_PRIMER_BASE_URL            = var.primer_tv_primer_base_url
        TV_MANIFEST_FAIL_MAX_ATTEMPTS = var.primer_tv_manifest_fail_max_attempts
        TV_MANIFEST_FAIL_MAX_DAYS     = var.primer_tv_manifest_fail_max_days
      }

      # Secret key names only — values from Nomad Variable nomad/jobs/primer-tv.
      template {
        destination = "secrets/primer-tv.env"
        env         = true
        change_mode = "restart"
        data        = <<-EOF
{{- with nomadVar "nomad/jobs/primer-tv" -}}
TV_DATABASE_URL={{ .tv_database_url }}
TV_JELLYFIN_API_KEY={{ .tv_jellyfin_api_key }}
TV_JELLYFIN_USER_ID={{ .tv_jellyfin_user_id }}
TV_ADMIN_API_KEY={{ .tv_admin_api_key }}
TV_PRIMER_SERVICE_TOKEN={{ .tv_primer_service_token }}
{{- end -}}
        EOF
      }

      resources {
        cpu    = var.primer_tv_cpu
        memory = var.primer_tv_memory
      }
    }
  }
}
