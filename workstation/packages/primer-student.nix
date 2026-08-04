# primer-student package for the student workstation.
#
# Source is the monorepo server/ tree (sibling of workstation/). The flake root
# cannot see ../server via the default flake copy, so callers pass an explicit
# filtered path (see flake.nix).
#
# vendorHash must be recomputed after go.mod/go.sum changes:
#   nix build .#primer-student --option sandbox false
# then replace the hash printed in the "got:" line.
#
# Until a real vendorHash is set, this file can still be imported; prefer the
# stub in hosts/workstation/primer-student.nix for system evaluation so
# `nixos-rebuild` / deploy.sh do not require network or a fixed hash.
{
  lib,
  buildGoModule,
  primerServerSrc,
}:

buildGoModule {
  pname = "primer-student";
  version = "0.1.0";

  src = primerServerSrc;

  subPackages = [ "cmd/primer-student" ];

  # Replace lib.fakeHash after first failed nix build (see README).
  vendorHash = lib.fakeHash;

  ldflags = [
    "-s"
    "-w"
  ];

  doCheck = false;

  meta = {
    description = "Primer student workstation TUI";
    mainProgram = "primer-student";
  };
}
