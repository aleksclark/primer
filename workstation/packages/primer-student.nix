# primer-student package for the student workstation.
#
# Source is the monorepo server/ tree (sibling of workstation/). The flake root
# cannot see ../server via the default flake copy, so callers pass an explicit
# filtered path (see flake.nix).
#
# vendorHash must be recomputed after go.mod/go.sum changes:
#   ./scripts/update-primer-student-vendor-hash.sh
# or:
#   nix build .#primer-student --option sandbox false
# then replace the hash printed in the "got:" line.
{
  lib,
  buildGoModule,
  go_1_25 ? null,
  go,
  primerServerSrc,
  version ? "0.1.0",
  commit ? "unknown",
}:

let
  # go.mod requires go >= 1.25. Prefer an explicit go_1_25 from nixpkgs-unstable
  # when the workstation nixpkgs pin is older.
  goToolchain =
    if go_1_25 != null then go_1_25
    else if lib.versionAtLeast go.version "1.25" then go
    else
      throw "primer-student requires Go >= 1.25 (got ${go.version}); pass go_1_25 from nixpkgs-unstable";
in
(buildGoModule.override { go = goToolchain; }) {
  pname = "primer-student";
  inherit version;

  src = primerServerSrc;

  subPackages = [ "cmd/primer-student" ];

  # Fixed-output hash of the Go module download (buildGoModule go-modules).
  # Update with workstation/scripts/update-primer-student-vendor-hash.sh.
  vendorHash = "sha256-WKaDIFOOAgp0/RubxLoKwPjljVJZ4vk1R+M0UoeBmr0=";

  ldflags = [
    "-s"
    "-w"
    "-X main.version=${version}"
    "-X main.commit=${commit}"
  ];

  doCheck = false;

  meta = {
    description = "Primer student workstation TUI";
    mainProgram = "primer-student";
  };
}
