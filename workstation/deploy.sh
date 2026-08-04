#!/usr/bin/env bash
set -euo pipefail

host="${1:-root@primer.local}"
rollback_unit="primer-deploy-rollback"
image="nixos/nix:2.24.11"
volume="primer-nix-store"
cache_dir="$(mktemp -d)"
remote_cache="/tmp/primer-deploy-cache"
trap 'rm -rf "$cache_dir"' EXIT

docker volume create "$volume" >/dev/null
system="$(docker run --rm \
  -v "$volume:/nix" \
  -v "$PWD:/work" \
  -w /work \
  "$image" \
  sh -c 'nix --extra-experimental-features "nix-command flakes" flake check --option sandbox false >&2 && nix --extra-experimental-features "nix-command flakes" build .#nixosConfigurations.workstation.config.system.build.toplevel --no-link --print-out-paths --option sandbox false')"

docker run --rm \
  -v "$volume:/nix" \
  -v "$cache_dir:/export" \
  "$image" \
  nix --extra-experimental-features 'nix-command flakes' copy --to file:///export "$system"
docker run --rm -v "$cache_dir:/export" "$image" chmod -R a+rwX /export

tar -C "$cache_dir" -czf - . | ssh "$host" "rm -rf '$remote_cache' && mkdir -p '$remote_cache' && tar -xzf - -C '$remote_cache'"
ssh "$host" "nix copy --extra-experimental-features 'nix-command flakes' --from 'file://$remote_cache' '$system'"

old_system="$(ssh "$host" 'readlink -f /run/current-system')"
ssh "$host" "'$system/bin/switch-to-configuration' test"
ssh "$host" "systemd-run --unit='$rollback_unit' --on-active=5m '$old_system/bin/switch-to-configuration' switch"
ssh "$host" "'$system/bin/switch-to-configuration' switch"
ssh "$host" 'systemctl is-active sshd >/dev/null && test "$(readlink -f /run/current-system)" = '"'"$system"'"''
ssh "$host" 'primer-student-health'
ssh "$host" "systemctl stop '$rollback_unit.timer'; systemctl reset-failed '$rollback_unit.service' 2>/dev/null || true; rm -rf '$remote_cache'"

printf 'Deployment confirmed on %s: %s\n' "$host" "$system"
