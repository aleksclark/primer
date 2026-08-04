# Student Workstation

NixOS configuration for the Primer student workstation (Lenovo ThinkCentre M93p Tiny).

## What This Does

- **Impermanent root**: Every reboot wipes the system back to a clean state
- **Persistent student work**: `~/projects` survives reboots (separate Btrfs subvolume)
- **Minimal desktop**: Sway (tiling WM) + Ghostty (terminal). Nothing else.
- **Activity monitoring**: Screenshots, process tracking, window focus logging
- **Remote access**: SSH administration + SSH-tunneled VNC for parent oversight

## Hardware Target

- Lenovo ThinkCentre M93p Tiny
- Intel i5-4570T (2C/4T, 2.9/3.6GHz)
- 8GB DDR3
- Intel HD 4600 (i915 driver, Wayland-native)
- Intel I217-LM Ethernet

## Bootstrap Process

### 1. Enable flakes on your build machine (one-time)

```bash
# Copy the nix config (or symlink it)
sudo cp workstation/nix.conf /etc/nix/nix.conf
sudo systemctl restart nix-daemon
```

### 2. Build the installer USB (on your workstation)

```bash
cd workstation
nix build .#installer-iso
sudo dd if=result/iso/*.iso of=/dev/sdX bs=4M status=progress
sync
```

### 3. Boot the M93p from USB

- Plug in USB, connect ethernet
- Power on, press F12 for boot menu, select USB
- Wait ~30 seconds for boot
- Console shows the IP address

### 4. Deploy the real system (from your workstation)

Option A: Using nixos-anywhere (formats disk + installs in one shot):
```bash
nix run github:nix-community/nixos-anywhere -- \
  --flake .#workstation \
  --disk main /dev/sda \
  root@<IP_FROM_CONSOLE>
```

Option B: Manual installation is intentionally not documented because the flake is on the build machine, not the installer. Use nixos-anywhere so partitioning and installation are driven and recorded from this checkout.

The installer and installed system have different SSH host keys. After the first reboot, remove only the installer's temporary key, reconnect, and verify the new fingerprint against the fingerprint shown by `ssh-keygen -lf /persist/etc/ssh/ssh_host_ed25519_key.pub` on the workstation console:

```bash
ssh-keygen -R <IP_FROM_CONSOLE>
ssh root@<IP_FROM_CONSOLE>
```

The installed host advertises itself as `primer.local` through mDNS. A DHCP reservation is still recommended so logs and router policy have a stable address.

### 5. Ongoing management (after install)

Use the guarded deployment wrapper. It evaluates and builds entirely on the management machine in an isolated Docker Nix store, bundles the completed closure, copies only build artifacts over SSH, activates with `test`, schedules a five-minute rollback on the host, switches, verifies SSH, and cancels the rollback only after the health check succeeds. Docker is required locally; the workstation does not build from source:

```bash
cd workstation
./deploy.sh root@primer.local
```

If your shell loses connectivity during deployment, do not retry immediately. Wait five minutes for the old generation to be restored. For manual recovery at the machine, select an older NixOS generation in the boot menu.

VNC is not exposed on the LAN. Forward it through SSH, leave the tunnel running, then connect the viewer to localhost:

```bash
ssh -N -L 5900:127.0.0.1:5900 root@primer.local
vncviewer 127.0.0.1:5900
```

Check activity logs:

```bash
ssh root@primer.local cat /persist/monitoring/windows/$(date +%Y-%m-%d).jsonl
ssh root@primer.local ls /persist/monitoring/screenshots/$(date +%Y-%m-%d)/
```

### 6. Pre-install validation

Run these before writing the USB and after changing workstation configuration:

```bash
nix flake check
nix build .#nixosConfigurations.workstation.config.system.build.toplevel
nix build .#installer-iso
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
- [ ] Restrict printer services to the actual management subnet and remove root/device-wide permissions
- [ ] Add CI evaluation and boot tests for the workstation flake
- [ ] Add encrypted storage if physical disk theft is in scope
