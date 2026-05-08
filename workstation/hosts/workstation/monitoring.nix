# Activity monitoring - lightweight, feeds into Primer tutor later
{ config, pkgs, lib, ... }:

{
  # Periodic screenshots (grim via systemd timer)
  # Captures screen every 60 seconds, stores in /persist
  systemd.user.services.screenshot-monitor = {
    description = "Periodic screenshot for activity monitoring";
    after = [ "graphical-session.target" ];
    serviceConfig = {
      Type = "oneshot";
      ExecStart = let
        script = pkgs.writeShellScript "take-screenshot" ''
          DIR="/persist/monitoring/screenshots/$(date +%Y-%m-%d)"
          mkdir -p "$DIR"
          ${pkgs.grim}/bin/grim "$DIR/$(date +%H%M%S).png"
          # Keep only last 7 days of screenshots
          find /persist/monitoring/screenshots -type d -mtime +7 -exec rm -rf {} + 2>/dev/null || true
        '';
      in "${script}";
    };
  };

  systemd.user.timers.screenshot-monitor = {
    description = "Screenshot timer";
    wantedBy = [ "timers.target" ];
    timerConfig = {
      OnCalendar = "*:*:00"; # every minute
      Persistent = false;
    };
  };

  # Process accounting - track all commands run by student
  services.acct.enable = true;

  # Audit daemon - log all execve calls by student user
  security.auditd.enable = true;
  security.audit = {
    enable = true;
    rules = [
      # Log all commands executed by student (uid will be set after first deploy)
      "-a always,exit -F arch=b64 -S execve -F uid>=1000 -F uid<=65000 -k student-commands"
    ];
  };

  # Persist monitoring data
  environment.persistence."/persist".directories = [
    "/var/log/audit"
    "/var/account"
  ];

  # Sway IPC window tracking daemon (tracks focused window over time)
  systemd.user.services.window-tracker = {
    description = "Track active window for time reporting";
    after = [ "graphical-session.target" ];
    wantedBy = [ "graphical-session.target" ];
    serviceConfig = {
      Restart = "on-failure";
      RestartSec = 5;
    };
    script = ''
      LOGDIR="/persist/monitoring/windows"
      mkdir -p "$LOGDIR"
      LOGFILE="$LOGDIR/$(date +%Y-%m-%d).jsonl"

      ${pkgs.sway}/bin/swaymsg -t subscribe '["window"]' | while read -r event; do
        TIMESTAMP=$(date -Iseconds)
        FOCUSED=$(${pkgs.sway}/bin/swaymsg -t get_tree | ${pkgs.jq}/bin/jq -r '.. | select(.focused?) | select(.focused==true) | {app_id, name}' 2>/dev/null)
        echo "{\"ts\":\"$TIMESTAMP\",\"window\":$FOCUSED}" >> "$LOGFILE"
      done
    '';
    path = with pkgs; [ sway jq coreutils ];
  };

  # Persist monitoring directory
  systemd.tmpfiles.rules = [
    "d /persist/monitoring 0700 root root -"
    "d /persist/monitoring/screenshots 0700 root root -"
    "d /persist/monitoring/windows 0700 root root -"
  ];
}
