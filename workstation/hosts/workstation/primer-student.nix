# Primer student client on the workstation.
#
# Threat model (Phase 8 broker split):
# - primer-student-broker.service runs as system user primer-broker and exclusively
#   owns the device token (0600), SQLite cache, workspaces, sandbox launch, and API.
# - The unprivileged student TUI connects over a Unix socket
#   (/run/primer-student/broker.sock, group students, mode 0660) and never sees
#   the device token. Peers are authenticated with SO_PEERCRED.
# - Bubblewrap + a pinned runtime profile sandbox terminal activities.
# - Parent revoke invalidates the server-side token; local cache may retain a
#   stale token until the next 401/re-pair (broker rotates on re-pair).
#
# Migration: if an old student-owned state.db exists at the legacy path, the
# broker copies it to broker ownership on first start and renames the original
# to *.pre-broker.bak for one-release rollback.
{ config, pkgs, lib, ... }:

let
  cfg = config.services.primer-student;
  stateDir = "/var/lib/primer-student";
  runDir = "/run/primer-student";
  dbPath = "${stateDir}/state.db";
  tokenFile = "${stateDir}/device.token";
  socketPath = "${runDir}/broker.sock";
  legacyDB = "${stateDir}/legacy-student-state.db";
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

  primerStudentBin =
    if nixBin != ""
    then nixBin
    else if builtins.pathExists prebuiltBin
    then prebuiltBin
    else "primer-student";

  primerStudent = pkgs.writeShellApplication {
    name = "primer-student";
    text = ''
      set -euo pipefail
      socket=${lib.escapeShellArg socketPath}
      nix_bin=${lib.escapeShellArg nixBin}
      prebuilt=${lib.escapeShellArg prebuiltBin}
      runtime_dir=${lib.escapeShellArg runtimeProfileDir}

      if [ -n "$runtime_dir" ]; then
        export PRIMER_RUNTIME_PROFILE_DIR="$runtime_dir"
      fi
      export PRIMER_BROKER_SOCKET="$socket"

      bin=""
      if [ -n "$nix_bin" ] && [ -x "$nix_bin" ]; then
        bin="$nix_bin"
      elif [ -x "$prebuilt" ]; then
        echo "primer-student: WARN: using deprecated prebuilt binary at $prebuilt" >&2
        bin="$prebuilt"
      else
        echo "primer-student: no binary installed." >&2
        echo "Build with: nix build .#primer-student  (from workstation/)" >&2
        exit 127
      fi

      # Default invocation launches the TUI against the broker socket.
      if [ "$#" -eq 0 ]; then
        exec "$bin" tui -socket "$socket"
      fi
      exec "$bin" "$@"
    '';
  };

  primerLauncher = pkgs.writeShellApplication {
    name = "primer";
    runtimeInputs = [ primerStudent ];
    text = ''
      exec primer-student tui -socket ${lib.escapeShellArg socketPath} "$@"
    '';
  };

  healthCheck = pkgs.writeShellApplication {
    name = "primer-student-health";
    runtimeInputs = [ pkgs.bubblewrap pkgs.coreutils ] ++ lib.optional (cfg.package != null) cfg.package;
    text = ''
      set -euo pipefail
      errors=0
      state_dir=${lib.escapeShellArg stateDir}
      socket=${lib.escapeShellArg socketPath}
      nix_bin=${lib.escapeShellArg nixBin}
      runtime_dir=${lib.escapeShellArg runtimeProfileDir}
      base_url=${lib.escapeShellArg cfg.baseUrl}

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
        if "$nix_bin" health -socket "$socket" -base-url "$base_url" -db "$state_dir/state.db"; then
          echo "ok: package health"
        else
          echo "FAIL: package health reported errors" >&2
          errors=$((errors + 1))
        fi
      else
        echo "WARN: services.primer-student.package not set or not executable" >&2
      fi

      if [ -S "$socket" ]; then
        echo "ok: broker socket $socket"
      else
        echo "FAIL: broker socket missing: $socket" >&2
        errors=$((errors + 1))
      fi

      if systemctl is-active --quiet primer-student-broker.service; then
        echo "ok: primer-student-broker.service active"
      else
        echo "FAIL: primer-student-broker.service not active" >&2
        errors=$((errors + 1))
      fi

      if [ ! -d "$state_dir" ]; then
        echo "FAIL: state dir missing: $state_dir" >&2
        errors=$((errors + 1))
      else
        echo "ok: state dir $state_dir"
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
      description = "Install primer-student launcher, broker service, health check, and runtime deps.";
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
    # Broker system user — owns token + SQLite. Not a login account.
    users.users.primer-broker = {
      isSystemUser = true;
      group = "primer-broker";
      description = "Primer student broker (credential isolation)";
      home = stateDir;
    };
    users.groups.primer-broker = {};
    # Student account may connect to the broker socket (group students).
    users.users.student.extraGroups = lib.mkAfter [ "students" ];
    users.groups.students = lib.mkDefault {};

    environment.systemPackages = [
      pkgs.bubblewrap
      primerStudent
      primerLauncher
      healthCheck
    ]
    ++ lib.optional (cfg.package != null) cfg.package
    ++ lib.optional (cfg.runtimeProfilePackage != null) cfg.runtimeProfilePackage;

    environment.variables = {
      PRIMER_BROKER_SOCKET = socketPath;
    } // lib.optionalAttrs (cfg.runtimeProfilePackage != null) {
      PRIMER_RUNTIME_PROFILE_DIR = runtimeProfileDir;
    };

    environment.persistence."/persist".directories = [
      stateDir
    ];

    systemd.tmpfiles.rules = [
      # State is broker-only (token + SQLite). Runtime dir is group students so
      # the student account can traverse to broker.sock (0660 students).
      "d ${stateDir} 0700 primer-broker primer-broker -"
      "d ${stateDir}/workspaces 0700 primer-broker primer-broker -"
      "d ${stateDir}/bin 0750 root root -"
      # Runtime dir is world-traversable; broker.sock is 0666 and SO_PEERCRED
      # authorizes clients. This avoids failures for long-lived Sway sessions
      # that started before the student user gained the students group.
      "d ${runDir} 0755 primer-broker students -"
      "Z ${runDir} 0755 primer-broker students -"
    ];

    systemd.services.primer-student-broker = {
      description = "Primer student broker (credential isolation)";
      wantedBy = [ "multi-user.target" ];
      after = [ "network-online.target" ];
      wants = [ "network-online.target" ];

      serviceConfig = {
        Type = "simple";
        User = "primer-broker";
        Group = "students";
        SupplementaryGroups = [ "primer-broker" ];
        StateDirectory = "primer-student";
        # State must not be student-readable (token + SQLite).
        StateDirectoryMode = "0700";
        RuntimeDirectory = "primer-student";
        RuntimeDirectoryMode = "0755";
        # Socket is chmod'd 0666 in the broker; keep umask from tightening it.
        UMask = "0000";
        ExecStartPre = [
          "+${pkgs.coreutils}/bin/chgrp students ${runDir}"
          "+${pkgs.coreutils}/bin/chmod 0755 ${runDir}"
        ];
        ExecStart = lib.concatStringsSep " " [
          primerStudentBin
          "broker"
          "-socket ${socketPath}"
          "-db ${dbPath}"
          "-token-file ${tokenFile}"
          "-base-url ${lib.escapeShellArg cfg.baseUrl}"
          "-workspace ${stateDir}/workspaces"
          "-legacy-db ${legacyDB}"
          "-socket-group students"
        ];
        Restart = "on-failure";
        RestartSec = "2s";

        # Hardening
        NoNewPrivileges = true;
        ProtectSystem = "strict";
        ProtectHome = true;
        PrivateTmp = true;
        PrivateDevices = true;
        ProtectKernelTunables = true;
        ProtectKernelModules = true;
        ProtectControlGroups = true;
        RestrictSUIDSGID = true;
        RestrictRealtime = true;
        LockPersonality = true;
        MemoryDenyWriteExecute = true;
        SystemCallArchitectures = "native";
        ReadWritePaths = [ stateDir runDir ];
        # bubblewrap needs access to the runtime profile and nix store.
        ReadOnlyPaths = lib.mkIf (runtimeProfileDir != "") [ runtimeProfileDir ];
        # Capability bounding for sandbox launch (bwrap may need userns).
        RestrictAddressFamilies = [ "AF_UNIX" "AF_INET" "AF_INET6" ];
      };

      environment = {
        PRIMER_BASE_URL = cfg.baseUrl;
      } // lib.optionalAttrs (runtimeProfileDir != "") {
        PRIMER_RUNTIME_PROFILE_DIR = runtimeProfileDir;
      };
    };
  };
}
