# Bootstrap ISO - minimal NixOS live USB with SSH access
# Boot this on the M93p, it gets DHCP, you SSH in, then deploy the real config.
{ config, pkgs, lib, ... }:

{
  # Enable SSH immediately on boot
  systemd.services.sshd.wantedBy = lib.mkForce [ "multi-user.target" ];
  services.openssh = {
    enable = true;
    settings = {
      PermitRootLogin = "prohibit-password";
      PasswordAuthentication = false;
    };
  };

  # Bake in your SSH key
  users.users.root.openssh.authorizedKeys.keys = [
    "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQDQFeDfovakWHp1alVhdI6qEI7Tw+/VLtbmFBRYGWwCCzGTvKl0TdG0KRdY4GJtDzXdOaYYwYobvCJ1w8Ww/MjKa1/FgA1XeUrwJrxdTXJE8gFK+sgtw/E4Qq+EJH3HBPAWLGuhMmQk7Sg1vx+mcQ1ejhaW2tM9KFM24tcRm4XCDZBoCZbmXCBBSjqM0KM+Zj5WH7qtb33JPHyIYdbvyKzVjklNeF9Sf9iLsAa1lpavqtRdQ+d/6TMK+u+fj9imsb4kIhOCXZTA9pyrp9HrIkSK4aCe1dACluTUmS8DYOAC9PM1STXah0WMFiG4IfoePCMus9VM/zA05FeH9ho6uG5n aleks.clark@gmail.com"
  ];

  # Network - DHCP on all interfaces
  networking = {
    useDHCP = true;
    hostName = "primer-installer";
  };

  # Show IP address on console after boot (so you can find it)
  systemd.services.show-ip = {
    description = "Display IP address on console";
    after = [ "network-online.target" ];
    wants = [ "network-online.target" ];
    wantedBy = [ "multi-user.target" ];
    serviceConfig = {
      Type = "oneshot";
      RemainAfterExit = true;
    };
    script = ''
      echo ""
      echo "=========================================="
      echo "  PRIMER WORKSTATION INSTALLER"
      echo "  SSH in as root to deploy the system"
      echo "=========================================="
      echo ""
      echo "  IP addresses:"
      ${pkgs.iproute2}/bin/ip -4 addr show | grep 'inet ' | grep -v '127.0.0.1' | awk '{print "    " $2}' || true
      echo ""
      echo "  ssh root@<ip>"
      echo "=========================================="
      echo ""
    '';
  };

  # Include tools needed for deployment
  environment.systemPackages = with pkgs; [
    git
    vim
    parted
    btrfs-progs
  ];

  # Enable flakes
  nix.settings.experimental-features = [ "nix-command" "flakes" ];
}
