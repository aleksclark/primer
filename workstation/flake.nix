{
  description = "Primer student workstation - NixOS with impermanence";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-25.05";
    disko = {
      url = "github:nix-community/disko";
      inputs.nixpkgs.follows = "nixpkgs";
    };
    impermanence.url = "github:nix-community/impermanence";
  };

  outputs = { self, nixpkgs, disko, impermanence, ... }:
  let
    system = "x86_64-linux";
    # Explicit path so the flake root (workstation/) can see sibling server/.
    # Used when wiring packages/primer-student.nix after vendorHash is pinned.
    primerServerSrc = builtins.path {
      path = ../server;
      name = "primer-server-src";
      filter = path: type:
        let base = baseNameOf path;
        in !(builtins.elem base [ ".git" "bin" "coverage.out" ]);
    };
  in {
    # The bootstrap ISO - boots with SSH, your key, DHCP
    # Build: nix build .#installer-iso
    nixosConfigurations.installer = nixpkgs.lib.nixosSystem {
      inherit system;
      modules = [
        "${nixpkgs}/nixos/modules/installer/cd-dvd/installation-cd-minimal.nix"
        ./installer/iso.nix
      ];
    };

    # The actual student workstation config
    # Deploy: nixos-anywhere --flake .#workstation root@<ip>
    # Or after bootstrap: nixos-rebuild switch --flake .#workstation --target-host root@<ip>
    #
    # primer-student.nix defaults package=null so evaluation does not need a
    # pinned Go vendorHash. Install the binary via `make student-deploy`, or
    # after fixing packages/primer-student.nix vendorHash, enable:
    #
    #   nixpkgs.overlays = [(final: prev: {
    #     primer-student = final.callPackage ./packages/primer-student.nix {
    #       primerServerSrc = primerServerSrc;
    #     };
    #   })];
    #   services.primer-student.package = pkgs.primer-student;
    nixosConfigurations.workstation = nixpkgs.lib.nixosSystem {
      inherit system;
      specialArgs = { inherit primerServerSrc; };
      modules = [
        disko.nixosModules.disko
        impermanence.nixosModules.impermanence
        ./hosts/workstation/disko.nix
        ./hosts/workstation/configuration.nix
        ./hosts/workstation/impermanence.nix
        ./hosts/workstation/sway.nix
        ./hosts/workstation/monitoring.nix
        ./hosts/workstation/users.nix
        ./hosts/workstation/klipper.nix
        ./hosts/workstation/primer-student.nix
      ];
    };

    # Convenience: build the ISO image directly
    # nix build .#installer-iso
    #
    # Go package is intentionally not exposed here until vendorHash is real;
    # see packages/primer-student.nix. System deploy uses a prebuilt binary
    # path (/var/lib/primer-student/bin/primer-student) instead.
    packages.${system}.installer-iso =
      self.nixosConfigurations.installer.config.system.build.isoImage;
  };
}
