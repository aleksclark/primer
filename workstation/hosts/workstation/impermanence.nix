# Impermanence - wipe root on every boot, persist only what's declared
{ config, pkgs, lib, ... }:

{
  # Wipe @root subvolume on boot and replace with empty snapshot
  # This runs in initrd before anything mounts
  boot.initrd.systemd.enable = true;
  boot.initrd.systemd.services.rollback = {
    description = "Rollback root subvolume to empty state";
    wantedBy = [ "initrd.target" ];
    after = [ "systemd-cryptsetup@*.service" ];
    before = [ "sysroot.mount" ];
    unitConfig.DefaultDependencies = "no";
    serviceConfig.Type = "oneshot";
    script = ''
      mkdir -p /mnt
      mount -t btrfs -o subvol=/ /dev/disk/by-partlabel/root /mnt

      # Delete all subvolumes under @root (leftover from last boot)
      if [ -d /mnt/@root ]; then
        btrfs subvolume list -o /mnt/@root | cut -f9 -d' ' | while read subvol; do
          btrfs subvolume delete "/mnt/$subvol" 2>/dev/null || true
        done
        btrfs subvolume delete /mnt/@root || true
      fi

      # Recreate empty @root
      btrfs subvolume create /mnt/@root

      umount /mnt
    '';
  };

  # Declare what persists across reboots
  environment.persistence."/persist" = {
    hideMounts = true;
    directories = [
      "/etc/nixos"              # NixOS config (for rebuilds)
      "/etc/ssh"                # SSH host keys
      "/var/lib/systemd"        # systemd state (timers, etc.)
      "/var/lib/nixos"          # NixOS state (uid/gid maps)
    ];
    files = [
      "/etc/machine-id"
    ];
    users.student = {
      directories = [
        "projects"              # Student work - NEVER wiped
        ".local/share/ghostty"  # Ghostty state
      ];
      files = [
        ".bash_history"
      ];
    };
  };

  # Ensure /persist exists and has correct ownership
  systemd.tmpfiles.rules = [
    "d /persist 0755 root root -"
    "d /persist/home 0755 root root -"
    "d /persist/home/student 0700 student students -"
    "d /persist/home/student/projects 0755 student students -"
    "d /persist/var 0755 root root -"
    "d /persist/var/log 0755 root root -"
  ];

  # Btrfs maintenance
  services.btrfs.autoScrub = {
    enable = true;
    interval = "weekly";
    fileSystems = [ "/" ];
  };
}
