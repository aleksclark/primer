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

  outputs = { self, nixpkgs, disko, impermanence, ... }: {
    # The bootstrap ISO - boots with SSH, your key, DHCP
    # Build: nix build .#installer-iso
    nixosConfigurations.installer = nixpkgs.lib.nixosSystem {
      system = "x86_64-linux";
      modules = [
        "${nixpkgs}/nixos/modules/installer/cd-dvd/installation-cd-minimal.nix"
        ./installer/iso.nix
      ];
    };

    # The actual student workstation config
    # Deploy: nixos-anywhere --flake .#workstation root@<ip>
    # Or after bootstrap: nixos-rebuild switch --flake .#workstation --target-host root@<ip>
    nixosConfigurations.workstation = nixpkgs.lib.nixosSystem {
      system = "x86_64-linux";
      modules = [
        disko.nixosModules.disko
        impermanence.nixosModules.impermanence
        ./hosts/workstation/disko.nix
        ./hosts/workstation/configuration.nix
        ./hosts/workstation/impermanence.nix
        ./hosts/workstation/sway.nix
        ./hosts/workstation/monitoring.nix
        ./hosts/workstation/users.nix
      ];
    };

    # Convenience: build the ISO image directly
    # nix build .#installer-iso
    packages.x86_64-linux.installer-iso =
      self.nixosConfigurations.installer.config.system.build.isoImage;
  };
}
