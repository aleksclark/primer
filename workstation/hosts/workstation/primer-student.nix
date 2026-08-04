# Primer student client on the workstation.
#
# Threat model (Phase 5 reduced / Phase 7 packaging):
# - The Phase 3 TUI still runs as the student user and stores the device token
#   in the SQLite cache under /var/lib/primer-student. A local root or the
#   student account can read that token.
# - Full broker split (root-owned credential + IPC, TUI never sees the token)
#   is deferred (Phase 8). Bubblewrap + a pinned runtime profile sandbox
#   terminal activities.
# - Parent revoke invalidates the server-side token; local cache may retain a
#   stale token until the next 401/re-pair.
{ config, pkgs, lib, ... }:

let
  cfg = config.services.primer-student;
  stateDir = "/var/lib/primer-student";
  dbPath = "${stateDir}/state.db";
  # Deprecated escape hatch from Phase 5. Prefer cfg.package (flake default).
  prebuiltBin = "${stateDir}/bin/primer-student";

  nixBin =
    if cfg.package != null
    then lib.getExe cfg.package
    else "";

  runtimeProfileDir =
    if cfg.runtimeProfilePackage != null
    then "${cfg.runtimeProfilePackage}"
    else "";

  primerStudent = pkgs.writeShellApplication {
    name = "primer-student";
    text = ''
      set -euo pipefail
      base_url=${lib.escapeShellArg cfg.baseUrl}
      db=${lib.escapeShellArg dbPath}
      nix_bin=${lib.escapeShellArg nixBin}
      prebuilt=${lib.escapeShellArg prebuiltBin}
      runtime_dir=${lib.escapeShellArg runtimeProfileDir}

      if [ -n "$runtime_dir" ]; then
        export PRIMER_RUNTIME_PROFILE_DIR="$runtime_dir"
      fi

      if [ -n "$nix_bin" ] && [ -x "$nix_bin" ]; then
        exec "$nix_bin" -base-url "$base_url" -db "$db" "$@"
      fi
      if [ -x "$prebuilt" ]; then
        echo "primer-student: WARN: using deprecated prebuilt binary at $prebuilt" >&2
        echo "primer-student: set services.primer-student.package (flake default) instead." >&2
        exec "$prebuilt" -base-url "$base_url" -db "$db" "$@"
      fi
      echo "primer-student: no binary installed." >&2
      echo "Build with: nix build .#primer-student  (from workstation/)" >&2
      echo "Or (deprecated): make student-deploy HOST=root@primer.local" >&2
      exit 127
    '';
  };

  primerLauncher = pkgs.writeShellApplication {
    name = "primer";
    runtimeInputs = [ primerStudent ];
    text = ''
      exec primer-student "$@"
    '';
  };

  healthCheck = pkgs.writeShellApplication {
    name = "primer-student-health";
    runtimeInputs = [ pkgs.bubblewrap pkgs.coreutils ] ++ lib.optional (cfg.package != null) cfg.package;
    text = ''
      set -euo pipefail
      errors=0
      state_dir=${lib.escapeShellArg stateDir}
      prebuilt=${lib.escapeShellArg prebuiltBin}
      nix_bin=${lib.escapeShellArg nixBin}
      runtime_dir=${lib.escapeShellArg runtimeProfileDir}

      if command -v primer-student >/dev/null 2>&1; then
        echo "ok: launcher $(command -v primer-student)"
      else
        echo "FAIL: primer-student launcher missing from PATH" >&2
        errors=$((errors + 1))
      fi

      if [ -n "$nix_bin" ] && [ -x "$nix_bin" ]; then
        echo "ok: package binary $nix_bin"
        if out="$("$nix_bin" -version 2>&1)"; then
          echo "ok: version $out"
        else
          echo "FAIL: package binary -version failed" >&2
          errors=$((errors + 1))
        fi
        if [ -n "$runtime_dir" ]; then
          export PRIMER_RUNTIME_PROFILE_DIR="$runtime_dir"
        fi
        if "$nix_bin" -health -db "$state_dir/state.db" -base-url ${lib.escapeShellArg cfg.baseUrl}; then
          echo "ok: package health"
        else
          echo "FAIL: package -health reported errors" >&2
          errors=$((errors + 1))
        fi
      else
        echo "WARN: services.primer-student.package not set or not executable" >&2
        if [ -x "$prebuilt" ]; then
          echo "ok: deprecated prebuilt binary $prebuilt"
        else
          echo "FAIL: no package binary and no prebuilt at $prebuilt" >&2
          errors=$((errors + 1))
        fi
      fi

      if [ ! -d "$state_dir" ]; then
        echo "FAIL: state dir missing: $state_dir" >&2
        errors=$((errors + 1))
      elif ! touch "$state_dir/.health-write" 2>/dev/null; then
        echo "FAIL: state dir not writable: $state_dir" >&2
        errors=$((errors + 1))
      else
        rm -f "$state_dir/.health-write"
        echo "ok: state dir writable $state_dir"
      fi

      if ! command -v bwrap >/dev/null 2>&1; then
        echo "FAIL: bubblewrap (bwrap) not on PATH" >&2
        errors=$((errors + 1))
      else
        echo "ok: bwrap present $(command -v bwrap)"
        if [ -n "$runtime_dir" ] && [ -d "$runtime_dir" ]; then
          echo "ok: runtime profile dir $runtime_dir"
          if bwrap --unshare-net --ro-bind /nix/store /nix/store \
              --ro-bind "$runtime_dir" /runtime \
              --ro-bind "$runtime_dir/bin" /bin \
              --tmpfs /tmp --chdir /tmp -- /runtime/bin/true; then
            echo "ok: bwrap can launch profile true"
          else
            echo "WARN: bwrap profile probe launch failed" >&2
          fi
        else
          echo "WARN: no runtime profile package configured" >&2
          if bwrap --unshare-net --ro-bind /nix /nix --tmpfs /tmp --chdir /tmp -- ${pkgs.coreutils}/bin/true; then
            echo "ok: bwrap can launch true"
          else
            echo "WARN: bwrap present but probe launch failed" >&2
          fi
        fi
      fi

      if [ "$errors" -ne 0 ]; then
        echo "primer-student-health: $errors check(s) failed" >&2
        exit 1
      fi
      echo "primer-student-health: ok"
    '';
  };
in
{
  options.services.primer-student = {
    enable = lib.mkOption {
      type = lib.types.bool;
      default = true;
      description = "Install primer-student launcher, health check, and runtime deps.";
    };

    baseUrl = lib.mkOption {
      type = lib.types.str;
      default = "https://primer.fleet.clark.team/api/v1";
      description = "Primer LMS API base URL for the student client.";
    };

    package = lib.mkOption {
      type = lib.types.nullOr lib.types.package;
      default = null;
      defaultText = lib.literalExpression "pkgs.primer-student";
      description = ''
        buildGoModule primer-student package. The flake sets this to the
        packaged binary by default. null falls back to the deprecated
        prebuilt path at ${prebuiltBin}.
      '';
    };

    runtimeProfilePackage = lib.mkOption {
      type = lib.types.nullOr lib.types.package;
      default = null;
      defaultText = lib.literalExpression "pkgs.primer-runtime-coreutils-basic";
      description = ''
        Nix store path bound read-only into bubblewrap sandboxes via
        PRIMER_RUNTIME_PROFILE_DIR. Should provide bin/ with coreutils and a
        shell for activities using runtime_profile: coreutils-basic.
      '';
    };
  };

  config = lib.mkIf cfg.enable {
    environment.systemPackages = [
      pkgs.bubblewrap
      primerStudent
      primerLauncher
      healthCheck
    ]
    ++ lib.optional (cfg.package != null) cfg.package
    ++ lib.optional (cfg.runtimeProfilePackage != null) cfg.runtimeProfilePackage;

    environment.variables = lib.mkIf (cfg.runtimeProfilePackage != null) {
      PRIMER_RUNTIME_PROFILE_DIR = runtimeProfileDir;
    };

    environment.persistence."/persist".directories = [
      stateDir
    ];

    systemd.tmpfiles.rules = [
      "d ${stateDir} 0750 student students -"
      # Keep bin/ for one-release deprecated prebuilt path.
      "d ${stateDir}/bin 0750 student students -"
      "d ${stateDir}/workspaces 0750 student students -"
    ];
  };
}
