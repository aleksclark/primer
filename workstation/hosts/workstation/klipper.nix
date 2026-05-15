# Klipper 3D printer stack - Klipper + Moonraker + Fluidd + webcam
{ config, pkgs, lib, ... }:

{
  # Klipper firmware host process
  services.klipper = {
    enable = true;
    user = "root"; # needs USB device access
    group = "root";
    # Use mutable config so it can be edited via Fluidd without NixOS rebuild
    mutableConfig = true;
    mutableConfigFolder = "/var/lib/moonraker/config";
    settings = {
      mcu = {
        serial = "/dev/serial/by-id/usb-Klipper_stm32g0b1xx_220017000B504B5735313920-if00";
        restart_method = "command";
      };
      printer = {
        kinematics = "none"; # safe mode - no movement until properly configured
        max_velocity = 1;
        max_accel = 1;
      };
      virtual_sdcard = {
        path = "/var/lib/moonraker/gcodes";
      };
      display_status = {};
      pause_resume = {};
    };
  };

  # Moonraker API server
  services.moonraker = {
    enable = true;
    address = "0.0.0.0";
    port = 7125;
    user = "root";
    group = "root";
    settings = {
      authorization = {
        trusted_clients = [
          "127.0.0.0/8"
          "192.168.0.0/24"
        ];
        cors_domains = [
          "http://192.168.0.26"
          "http://primer"
          "http://localhost"
        ];
      };
      octoprint_compat = {};
      history = {};
      file_manager = {
        enable_object_processing = true;
      };
      "webcam camera" = {
        location = "printer";
        enabled = true;
        stream_url = "http://192.168.0.26:8080/stream";
        snapshot_url = "http://192.168.0.26:8080/snapshot";
        target_fps = 15;
      };
    };
  };

  # Fluidd web UI
  services.fluidd.enable = true;

  # Webcam streaming with ustreamer
  systemd.services.ustreamer = {
    description = "USB camera streamer for 3D printer";
    after = [ "network.target" ];
    wantedBy = [ "multi-user.target" ];
    serviceConfig = {
      ExecStart = "${pkgs.ustreamer}/bin/ustreamer --host 0.0.0.0 --port 8080 --device /dev/video0 --resolution 640x480 --format MJPEG --desired-fps 15";
      Restart = "on-failure";
      RestartSec = 5;
    };
  };

  # Open firewall for Fluidd (80), Moonraker (7125), and camera stream (8080)
  networking.firewall.allowedTCPPorts = [ 80 7125 8080 ];

  # Ensure USB device access
  services.udev.extraRules = ''
    # 3D printer USB serial
    SUBSYSTEM=="tty", ATTRS{idVendor}=="*", ATTRS{idProduct}=="*", MODE="0666"
    # USB camera
    SUBSYSTEM=="video4linux", MODE="0666"
  '';

  # Packages needed
  environment.systemPackages = with pkgs; [
    ustreamer
    klipper # for klipper_mcu tools
  ];
}
