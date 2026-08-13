# Authoritative portable jobspec for primer (plan-03 release set).
# Image: var.image_primer from images.lock.hcl (digest only).
# Secrets: Nomad Variable nomad/jobs/primer via template env=true.
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

job "primer" {
  datacenters = ["home"]
  type        = "service"

  meta {
    managed_by       = "fleet-pull-reconciler"
    source_repo      = "https://github.com/aleksclark/primer"
    source_path      = "deploy/nomad/jobs/primer.nomad.hcl"
    deployment_owner = "aleks-clark"
    release_set      = "primer"
  }

  group "primer" {
    count = 1

    network {
      port "http" {}
    }

    service {
      name     = "primer"
      port     = "http"
      provider = "nomad"

      tags = [
        "traefik.enable=true",
        "traefik.http.routers.primer.rule=Host(`primer.fleet.clark.team`) || Host(`primer.clark.team`)",
        "traefik.http.routers.primer.entrypoints=websecure",
        "traefik.http.routers.primer.tls.certresolver=letsencrypt",
      ]

      check {
        type     = "http"
        path     = "/api/v1/health"
        interval = "30s"
        timeout  = "5s"
      }
    }

    task "primer" {
      driver = "docker"

      config {
        image = var.image_primer
        ports = ["http"]
      }

      env {
        PORT         = "${NOMAD_PORT_http}"
        HOST         = var.primer_host
        ENV          = var.primer_env
        CORS_ORIGINS = var.primer_cors_origins
      }

      # Secret key names only — values from Nomad Variable nomad/jobs/primer.
      template {
        destination = "secrets/primer.env"
        env         = true
        change_mode = "restart"
        data        = <<-EOF
{{- with nomadVar "nomad/jobs/primer" -}}
DATABASE_URL={{ .database_url }}
SERVICE_TOKEN={{ .service_token }}
{{- end -}}
        EOF
      }

      resources {
        cpu    = var.primer_cpu
        memory = var.primer_memory
      }
    }
  }
}
