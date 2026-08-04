# Primer student client on the workstation.
#
# Threat model (Phase 5 reduced):
# - The Phase 3 TUI still runs as the student user and stores the device token
#   in the SQLite cache under /var/lib/primer-student. A local root or the
#   student account can read that token.
# - Full broker split (root-owned credential + IPC, TUI never sees the token)
#   is deferred. Bubblewrap is installed so terminal activities can sandbox.
# - Parent revoke invalidates the server-side token; local cache may retain a
#   stale token until the next 401/re-pair.
{ config, pkgs, lib, ... }:

let
  cfg = config.services.primer-student;
  stateDir = "/var/lib/primer-student";
  dbPath = "${stateDir}/state.db";
  prebuiltBin = "${stateDir}/bin/primer-student";

  nixBin =
    if cfg.package != null
    then lib.getExe cfg.package
    else "";

  primerStudent = pkgs.writeShellApplication {
    name = "primer-student";
    text = ''
      set -euo pipefail
      base_url=${lib.escapeShellArg cfg.baseUrl}
      db=${lib.escapeShellArg dbPath}
      nix_bin=${lib.escapeShellArg nixBin}
      prebuilt=${lib.escapeShellArg prebuiltBin}

      if [ -n "$nix_bin" ] && [ -x "$nix_bin" ]; then
        exec "$nix_bin" -base-url "$base_url" -db "$db" "$@"
      fi
      if [ -x "$prebuilt" ]; then
        exec "$prebuilt" -base-url "$base_url" -db "$db" "$@"
      fi
      echo "primer-student: no binary installed." >&2
      echo "Build with: make student-build && scp bin/primer-student root@host:$prebuilt" >&2
      echo "Or set services.primer-student.package after vendorHash is fixed." >&2
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
    runtimeInputs = [ pkgs.bubblewrap pkgs.coreutils ];
    text = ''
      set -euo pipefail
      errors=0
      state_dir=${lib.escapeShellArg stateDir}
      prebuilt=${lib.escapeShellArg prebuiltBin}

      if command -v primer-student >/dev/null 2>&1; then
        echo "ok: launcher $(command -v primer-student)"
      else
        echo "FAIL: primer-student launcher missing from PATH" >&2
        errors=$((errors + 1))
      fi

      if [ -x "$prebuilt" ]; then
        echo "ok: prebuilt binary $prebuilt"
      else
        echo "WARN: no prebuilt binary at $prebuilt (install via make student-build or nix package)" >&2
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
        if bwrap --unshare-net --ro-bind /nix /nix --tmpfs /tmp --chdir /tmp -- ${pkgs.coreutils}/bin/true; then
          echo "ok: bwrap can launch true"
        else
          echo "WARN: bwrap present but probe launch failed" >&2
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
      # null keeps deploy/eval working before vendorHash is pinned. Set to
      # pkgs.primer-student after packages/primer-student.nix has a real hash.
      default = null;
      defaultText = lib.literalExpression "null";
      description = "Optional buildGoModule primer-student package; null uses prebuilt path.";
    };
  };

  config = lib.mkIf cfg.enable {
    environment.systemPackages = [
      pkgs.bubblewrap
      primerStudent
      primerLauncher
      healthCheck
    ] ++ lib.optional (cfg.package != null) cfg.package;

    environment.persistence."/persist".directories = [
      stateDir
    ];

    systemd.tmpfiles.rules = [
      "d ${stateDir} 0750 student students -"
      "d ${stateDir}/bin 0750 student students -"
      "d ${stateDir}/workspaces 0750 student students -"
    ];
  };
}
