#!/usr/bin/env bash
# Recompute vendorHash for packages/primer-student.nix after go.mod/go.sum changes.
#
# Host nix on this management machine may segfault; this script prefers the same
# Docker + shared volume approach as deploy.sh.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
pkg_nix="$root/packages/primer-student.nix"
image="${PRIMER_NIX_IMAGE:-nixos/nix:2.24.11}"
volume="${PRIMER_NIX_VOLUME:-primer-nix-store}"

if [[ ! -f "$pkg_nix" ]]; then
  echo "missing $pkg_nix" >&2
  exit 1
fi

docker volume create "$volume" >/dev/null

# Mount Primer parent so git worktrees resolve (same pattern as deploy/Makefile).
# root = .../worktrees/command-teacher/workstation
repo_root="$(cd "$root/.." && pwd)"             # .../worktrees/command-teacher
primer_root="$(cd "$repo_root/../.." && pwd)"   # .../primer

echo "Building primer-student with fakeHash to capture got: line..." >&2
log="$(mktemp)"
trap 'rm -f "$log"' EXIT

set +e
docker run --rm \
  -v "$volume:/nix" \
  -v "$primer_root:$primer_root" \
  -w "$root" \
  -e NIX_CONFIG='experimental-features = nix-command flakes' \
  "$image" \
  sh -c "
git config --global --add safe.directory '*'
nix build --impure --option sandbox false --no-link \
  --expr '
let
  flake = builtins.getFlake (toString $root);
  pkgs = import flake.inputs.nixpkgs { system = \"x86_64-linux\"; };
  pkgsGo = import flake.inputs.nixpkgs-go { system = \"x86_64-linux\"; };
  src = builtins.path {
    path = $repo_root/server;
    name = \"primer-server-src\";
    filter = path: type:
      let base = baseNameOf path;
      in !(builtins.elem base [ \".git\" \"bin\" \"coverage.out\" \"vendor\" ]);
  };
  buildGoModule = pkgs.buildGoModule.override { go = pkgsGo.go_1_25; };
in buildGoModule {
  pname = \"primer-student\";
  version = \"0.1.0\";
  inherit src;
  subPackages = [ \"cmd/primer-student\" ];
  vendorHash = pkgs.lib.fakeHash;
  doCheck = false;
}
'
" 2>"$log"
set -e

if ! grep -q 'got:' "$log"; then
  echo "failed to capture vendor hash; build log:" >&2
  cat "$log" >&2
  exit 1
fi

got="$(grep -oE 'got:[[:space:]]+sha256-[A-Za-z0-9+/=]+' "$log" | head -1 | awk '{print $2}')"
if [[ -z "$got" ]]; then
  echo "could not parse got: hash from log" >&2
  cat "$log" >&2
  exit 1
fi

echo "vendorHash = $got" >&2

# Replace vendorHash assignment in the package file.
tmp="$(mktemp)"
sed -E "s|vendorHash = \"sha256-[A-Za-z0-9+/=]+\";|vendorHash = \"$got\";|" \
  "$pkg_nix" >"$tmp"
# Also handle lib.fakeHash form.
if grep -q 'lib.fakeHash' "$tmp"; then
  sed -E "s|vendorHash = lib\.fakeHash;|vendorHash = \"$got\";|" "$tmp" >"${tmp}.2"
  mv "${tmp}.2" "$tmp"
fi
mv "$tmp" "$pkg_nix"

# Keep activity-validate.nix in sync when present.
av="$root/packages/activity-validate.nix"
if [[ -f "$av" ]]; then
  tmp="$(mktemp)"
  sed -E "s|vendorHash = \"sha256-[A-Za-z0-9+/=]+\";|vendorHash = \"$got\";|" "$av" >"$tmp"
  mv "$tmp" "$av"
  echo "updated $av" >&2
fi

echo "updated $pkg_nix" >&2
echo "$got"
