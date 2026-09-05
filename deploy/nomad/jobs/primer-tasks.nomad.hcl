# Standalone Tasks P1/P2 runtime. No shared LMS/TV database or routing changes.
# Candidate until a real images.tasks.lock.hcl and approved fleet writer exist.
variable "image_primer_tasks" {
  type        = string
  description = "Immutable ghcr.io/aleksclark/primer-tasks@sha256:... reference"
  validation {
    condition     = can(regex("^ghcr\\.io/aleksclark/primer-tasks@sha256:[0-9a-f]{64}$", var.image_primer_tasks))
    error_message = "Tasks requires a published immutable GHCR digest, never a tag."
  }
}

variable "tasks_cpu" {
  type    = number
  default = 200
}

variable "tasks_memory" {
  type    = number
  default = 256
}

variable "tasks_public_origin" {
  type    = string
  default = "https://api.primerlms.com"
}

job "primer-tasks" {
  datacenters = ["home"]
  type        = "service"

  meta {
    managed_by       = "fleet-pull-reconciler"
    source_repo      = "https://github.com/aleksclark/primer"
    source_path      = "deploy/nomad/jobs/primer-tasks.nomad.hcl"
    deployment_owner = "aleks-clark"
    release_set      = "primer-tasks"
  }

  group "tasks" {
    count = 1

    update {
      max_parallel      = 1
      min_healthy_time  = "10s"
      healthy_deadline  = "2m"
      progress_deadline = "5m"
      auto_revert       = true
    }

    network {
      port "http" {
        to = 8080
      }
    }

    service {
      name     = "primer-tasks"
      port     = "http"
      provider = "nomad"

      # Do not strip /tasks or /tasks/api: the app owns that transformation.
      # The LMS host/catch-all and /api/v1 routers remain unchanged.
      tags = [
        "traefik.enable=true",
        "traefik.http.routers.primer-tasks.rule=Host(`api.primerlms.com`) && (Path(`/tasks`) || PathPrefix(`/tasks/`))",
        "traefik.http.routers.primer-tasks.priority=200",
        "traefik.http.routers.primer-tasks.entrypoints=websecure",
        # Cloudflared sends TLS SNI primer.fleet.clark.team while preserving
        # HTTP Host api.primerlms.com. Reuse the existing fleet certificate;
        # do not request a new api.primerlms.com origin certificate.
        "traefik.http.routers.primer-tasks.tls=true",
      ]

      check {
        type     = "http"
        path     = "/health"
        interval = "15s"
        timeout  = "5s"
      }
    }

    # Embedded, product-local migrations run before the server in each new
    # allocation. Failure blocks service startup. No public migration endpoint.
    task "migrate" {
      driver = "docker"
      user   = "65532:65532"

      lifecycle {
        hook    = "prestart"
        sidecar = false
      }

      config {
        image           = var.image_primer_tasks
        entrypoint      = ["/app/tasks-migrate"]
        readonly_rootfs = true
        cap_drop        = ["ALL"]
        security_opt    = ["no-new-privileges:true"]
      }

      restart {
        attempts = 0
        mode     = "fail"
      }

      template {
        destination = "secrets/tasks-migrate.env"
        env         = true
        change_mode = "noop"
        data        = <<-EOF
{{- with nomadVar "nomad/jobs/primer-tasks" -}}
TASKS_DATABASE_URL={{ .tasks_database_url | toJSON }}
{{- end -}}
        EOF
      }

      resources {
        cpu    = 100
        memory = 128
      }
    }

    task "tasks" {
      driver       = "docker"
      user         = "65532:65532"
      kill_signal  = "SIGTERM"
      kill_timeout = "10s"

      config {
        image           = var.image_primer_tasks
        ports           = ["http"]
        readonly_rootfs = true
        cap_drop        = ["ALL"]
        security_opt    = ["no-new-privileges:true"]
      }

      env {
        TASKS_ENV           = "production"
        TASKS_AUTH_MODE     = "clerk"
        TASKS_HOST          = "0.0.0.0"
        TASKS_PORT          = "8080"
        TASKS_BASE_PATH     = "/tasks"
        TASKS_WEB_DIR       = "/app/web"
        TASKS_PUBLIC_ORIGIN = var.tasks_public_origin
      }

      template {
        destination = "secrets/tasks.env"
        env         = true
        change_mode = "restart"
        data        = <<-EOF
{{- with nomadVar "nomad/jobs/primer-tasks" -}}
TASKS_DATABASE_URL={{ .tasks_database_url | toJSON }}
TASKS_CLERK_ISSUER={{ .tasks_clerk_issuer | toJSON }}
TASKS_CLERK_JWKS_URL={{ .tasks_clerk_jwks_url | toJSON }}
{{- end -}}
        EOF
      }

      resources {
        cpu    = var.tasks_cpu
        memory = var.tasks_memory
      }
    }
  }
}
