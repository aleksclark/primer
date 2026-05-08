# Student Workstation

NixOS configuration for the Primer student workstation (Lenovo ThinkCentre M93p Tiny).

## What This Does

- **Impermanent root**: Every reboot wipes the system back to a clean state
- **Persistent student work**: `~/projects` survives reboots (separate Btrfs subvolume)
- **Minimal desktop**: Sway (tiling WM) + Ghostty (terminal). Nothing else.
- **Activity monitoring**: Screenshots, process tracking, window focus logging
- **Remote access**: SSH + VNC for parent oversight

## Hardware Target

- Lenovo ThinkCentre M93p Tiny
- Intel i5-4570T (2C/4T, 2.9/3.6GHz)
- 8GB DDR3
- Intel HD 4600 (i915 driver, Wayland-native)
- Intel I217-LM Ethernet

## Bootstrap Process

### 1. Build the installer USB (on your workstation)

```bash
cd workstation
nix build .#installer-iso
sudo dd if=result/iso/*.iso of=/dev/sdX bs=4M status=progress
sync
```

### 2. Boot the M93p from USB

- Plug in USB, connect ethernet
- Power on, press F12 for boot menu, select USB
- Wait ~30 seconds for boot
- Console shows the IP address

### 3. Deploy the real system (from your workstation)

Option A: Using nixos-anywhere (formats disk + installs in one shot):
```bash
nix run github:nix-community/nixos-anywhere -- \
  --flake .#workstation \
  --disk main /dev/sda \
  root@<IP_FROM_CONSOLE>
```

Option B: Manual (SSH in, partition with disko, install):
```bash
ssh root@<IP>
# Then from the M93p:
nix run github:nix-community/disko -- --mode disko ./hosts/workstation/disko.nix
nixos-install --flake .#workstation --no-root-passwd
reboot
```

### 4. Ongoing management (after install)

```bash
# Rebuild remotely after config changes:
nixos-rebuild switch --flake .#workstation --target-host root@primer

# VNC into the student's session:
vncviewer primer:5900

# Check activity logs:
ssh root@primer cat /persist/monitoring/windows/$(date +%Y-%m-%d).jsonl
ssh root@primer ls /persist/monitoring/screenshots/$(date +%Y-%m-%d)/
```

## Student Environment

On boot, the student auto-logs in and gets:
- Sway tiling compositor (Super+Return = new terminal)
- Ghostty terminal emulator
- That's it. No browser, no file manager, no other GUI apps.

### Key bindings (Super = Mod key)
- `Super+Return` — Open Ghostty
- `Super+h/j/k/l` — Focus left/down/up/right
- `Super+Shift+h/j/k/l` — Move window
- `Super+1-4` — Switch workspace
- `Super+Shift+q` — Close window
- `Super+f` — Fullscreen
- `Super+r` — Resize mode

## Disk Layout

```
/dev/sda1  512MB  EFI System Partition (/boot)
/dev/sda2  4GB    Swap
/dev/sda3  rest   Btrfs
  @root     → /              (WIPED every boot)
  @nix      → /nix           (package store, persistent)
  @persist  → /persist       (declared persistent state)
  @projects → /persist/home/student/projects  (student work)
  @log      → /persist/var/log               (audit/monitoring logs)
```

## What Persists Across Reboots

- `/persist/home/student/projects` — Student's actual work
- `/persist/monitoring/` — Activity screenshots, window tracking, audit logs
- `/etc/ssh/` host keys, `/etc/machine-id`
- NixOS state (`/var/lib/nixos`, `/var/lib/systemd`)
- Student's `.bash_history`

Everything else is wiped. Browser cache, temp files, accidental config changes — all gone on reboot.

## TODO

- [ ] Set student password (currently placeholder hash in users.nix)
- [ ] Add network filtering (AdGuard Home in whitelist mode)
- [ ] Add typing metrics daemon
- [ ] Add break enforcement (screen lock after 45 min continuous use)
- [ ] Wire monitoring into Primer tutor API
- [ ] Add wayvnc TLS/password authentication
