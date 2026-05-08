# Sway + Ghostty - minimal tiling desktop
# No browser, no file manager, no GUI apps. Just sway + ghostty.
{ config, pkgs, lib, ... }:

{
  # Sway compositor
  programs.sway = {
    enable = true;
    wrapperFeatures.gtk = false; # no GTK apps
    extraPackages = with pkgs; [
      swaylock   # screen locker
      swayidle   # idle management
      wmenu      # launcher (minimal dmenu replacement)
    ];
  };

  # Wayland session environment
  environment.sessionVariables = {
    # Force Wayland for GTK (ghostty uses GTK)
    GDK_BACKEND = "wayland";
    # Intel HD 4600 - use the right driver
    LIBVA_DRIVER_NAME = "i965";
    # XDG
    XDG_SESSION_TYPE = "wayland";
    XDG_CURRENT_DESKTOP = "sway";
  };

  # Auto-login student to sway on tty1
  services.getty.autologinUser = "student";

  # Start sway on login (added to student's shell profile)
  environment.loginShellInit = ''
    if [ "$(tty)" = "/dev/tty1" ] && [ -z "$WAYLAND_DISPLAY" ]; then
      exec sway
    fi
  '';

  # Sway configuration
  environment.etc."sway/config.d/primer.conf".text = ''
    # Primer student workstation sway config

    # Modifier = Super key
    set $mod Mod4

    # Terminal = Ghostty (the ONLY app)
    set $term ghostty
    bindsym $mod+Return exec $term

    # Kill focused window
    bindsym $mod+Shift+q kill

    # No launcher needed (only ghostty available)
    # But provide wmenu for future use
    bindsym $mod+d exec wmenu-run

    # Movement (vim keys)
    bindsym $mod+h focus left
    bindsym $mod+j focus down
    bindsym $mod+k focus up
    bindsym $mod+l focus right

    # Move windows
    bindsym $mod+Shift+h move left
    bindsym $mod+Shift+j move down
    bindsym $mod+Shift+k move up
    bindsym $mod+Shift+l move right

    # Splits
    bindsym $mod+b splith
    bindsym $mod+v splitv

    # Fullscreen
    bindsym $mod+f fullscreen

    # Layout
    bindsym $mod+s layout stacking
    bindsym $mod+w layout tabbed
    bindsym $mod+e layout toggle split

    # Workspaces
    bindsym $mod+1 workspace number 1
    bindsym $mod+2 workspace number 2
    bindsym $mod+3 workspace number 3
    bindsym $mod+4 workspace number 4
    bindsym $mod+Shift+1 move container to workspace number 1
    bindsym $mod+Shift+2 move container to workspace number 2
    bindsym $mod+Shift+3 move container to workspace number 3
    bindsym $mod+Shift+4 move container to workspace number 4

    # Resize mode
    mode "resize" {
      bindsym h resize shrink width 10px
      bindsym j resize grow height 10px
      bindsym k resize shrink height 10px
      bindsym l resize grow width 10px
      bindsym Return mode "default"
      bindsym Escape mode "default"
    }
    bindsym $mod+r mode "resize"

    # Lock screen
    bindsym $mod+Shift+l exec swaylock -c 000000

    # Status bar - minimal
    bar {
      position top
      status_command while date +'%Y-%m-%d %H:%M'; do sleep 60; done
      colors {
        background #1a1b26
        statusline #a9b1d6
      }
    }

    # Appearance
    default_border pixel 2
    gaps inner 4
    client.focused #7aa2f7 #7aa2f7 #1a1b26 #7dcfff
    client.unfocused #414868 #414868 #a9b1d6 #414868

    # Disable window titlebars
    font pango:JetBrainsMono Nerd Font 0

    # Output config (DisplayPort on M93p)
    # Will auto-detect, but set scale for standard 1080p monitor
    output * bg #1a1b26 solid_color

    # Idle: lock after 5 min, screen off after 10
    exec swayidle -w \
      timeout 300 'swaylock -f -c 000000' \
      timeout 600 'swaymsg "output * power off"' \
      resume 'swaymsg "output * power on"'

    # Start ghostty on login
    exec ghostty
  '';

  # Ghostty configuration
  environment.etc."xdg/ghostty/config".text = ''
    font-family = JetBrainsMono Nerd Font
    font-size = 14
    theme = tokyonight
    window-decoration = false
    confirm-close-surface = false
    copy-on-select = clipboard
  '';

  # VNC for parent remote viewing
  # wayvnc runs as a systemd user service
  environment.systemPackages = with pkgs; [
    wayvnc
  ];

  # wayvnc autostart (parent can connect on port 5900)
  systemd.user.services.wayvnc = {
    description = "VNC server for remote viewing";
    after = [ "graphical-session.target" ];
    wantedBy = [ "graphical-session.target" ];
    serviceConfig = {
      ExecStart = "${pkgs.wayvnc}/bin/wayvnc 0.0.0.0 5900";
      Restart = "on-failure";
      RestartSec = 5;
    };
  };
}
