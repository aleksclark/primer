# Main system configuration for the student workstation
# Lenovo ThinkCentre M93p Tiny - i5-4570T, 8GB RAM, Intel HD 4600
{ config, pkgs, lib, ... }:

{
  # System basics
  system.stateVersion = "25.05";
  networking.hostName = "primer";
  time.timeZone = "America/Chicago"; # Tennessee is Central

  # Boot
  boot = {
    loader = {
      systemd-boot.enable = true;
      efi.canTouchEfiVariables = true;
      # Limit generations to save /boot space (512MB)
      systemd-boot.configurationLimit = 10;
    };
    # Intel HD 4600 (Haswell) - i915 is in-kernel
    initrd.availableKernelModules = [
      "xhci_pci" "ehci_pci" "ahci" "usb_storage" "sd_mod"
    ];
    kernelModules = [ "kvm-intel" ];
  };

  # Hardware - Intel HD 4600
  hardware = {
    graphics = {
      enable = true;
      extraPackages = with pkgs; [
        intel-media-driver  # newer iHD driver
        intel-vaapi-driver  # older i965 driver (Haswell uses this)
        libvdpau-va-gl
      ];
    };
    cpu.intel.updateMicrocode = true;
  };

  # Networking - wired only (M93p Tiny has Intel I217-LM)
  networking = {
    useDHCP = false;
    interfaces.eno1.useDHCP = true; # Intel ethernet
    firewall = {
      enable = true;
      allowedTCPPorts = [ 22 5900 ]; # SSH + VNC for parent
    };
  };

  # Nix settings
  nix = {
    settings = {
      experimental-features = [ "nix-command" "flakes" ];
      auto-optimise-store = true;
    };
    # Garbage collect weekly to save disk
    gc = {
      automatic = true;
      dates = "weekly";
      options = "--delete-older-than 14d";
    };
  };

  # Locale
  i18n.defaultLocale = "en_US.UTF-8";
  console.keyMap = "us";

  # SSH access for parent/admin
  services.openssh = {
    enable = true;
    settings = {
      PermitRootLogin = "prohibit-password";
      PasswordAuthentication = false;
    };
  };

  # Minimal packages - no browser, no GUI apps except ghostty+sway
  environment.systemPackages = with pkgs; [
    # System tools
    vim
    git
    htop
    tree
    file
    unzip
    ripgrep
    fd

    # Terminal
    ghostty

    # Sway utilities (minimal)
    grim        # screenshots
    wl-clipboard
  ];

  # Fonts - readable mono for terminal work
  fonts = {
    packages = with pkgs; [
      nerd-fonts.jetbrains-mono
      noto-fonts
    ];
    fontconfig.defaultFonts = {
      monospace = [ "JetBrainsMono Nerd Font" ];
      sansSerif = [ "Noto Sans" ];
    };
  };

  # Security - no polkit escalation for student
  security = {
    polkit.enable = true;
    sudo = {
      enable = true;
      # Only root can sudo (student cannot)
      extraRules = [];
    };
    # Prevent student from reading other users' processes
    hideProcessInformation = true;
  };

  # Disable things the student doesn't need
  services.printing.enable = false;
  sound.enable = false; # no audio needed initially
  hardware.bluetooth.enable = false;
}
