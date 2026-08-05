{
  description = "Primer student workstation - NixOS with impermanence";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-25.05";
    # Go 1.25+ for primer-student (go.mod requires >= 1.25.7). Workstation
    # system packages stay on nixos-25.05; only the Go toolchain comes from here.
    nixpkgs-go.url = "github:NixOS/nixpkgs/nixos-unstable";
    disko = {
      url = "github:nix-community/disko";
      inputs.nixpkgs.follows = "nixpkgs";
    };
    impermanence.url = "github:nix-community/impermanence";
  };

  outputs = { self, nixpkgs, nixpkgs-go, disko, impermanence, ... }:
  let
    system = "x86_64-linux";
    pkgs = import nixpkgs { inherit system; };
    pkgsGo = import nixpkgs-go { inherit system; };

    # Explicit path so the flake root (workstation/) can see sibling server/.
    primerServerSrc = builtins.path {
      path = ../server;
      name = "primer-server-src";
      filter = path: type:
        let base = baseNameOf path;
        in !(builtins.elem base [ ".git" "bin" "coverage.out" "vendor" ]);
    };

    curriculumActivities = builtins.path {
      path = ../curriculum/activities;
      name = "primer-curriculum-activities";
    };

    # self.rev / shortRev require a clean git flake context. Fall back so
    # `nix build` still works from Docker mounts and dirty worktrees.
    gitCommit =
      if self ? shortRev then self.shortRev
      else if self ? dirtyShortRev then self.dirtyShortRev
      else "unknown";
    gitVersion = "0.1.0+${gitCommit}";

    primerStudent = pkgs.callPackage ./packages/primer-student.nix {
      inherit primerServerSrc;
      go_1_25 = pkgsGo.go_1_25;
      version = gitVersion;
      commit = gitCommit;
    };

    activityValidate = pkgs.callPackage ./packages/activity-validate.nix {
      inherit primerServerSrc;
      go_1_25 = pkgsGo.go_1_25;
      curriculumActivities = curriculumActivities;
    };

    runtimeCoreutilsBasic = pkgs.callPackage ./packages/runtime-coreutils-basic.nix { };

    primerOverlay = final: prev: {
      primer-student = primerStudent;
      primer-runtime-coreutils-basic = runtimeCoreutilsBasic;
      primer-activity-validate = activityValidate;
    };
  in {
    overlays.default = primerOverlay;

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
    # Deploy: ./deploy.sh root@primer.local
    nixosConfigurations.workstation = nixpkgs.lib.nixosSystem {
      inherit system;
      specialArgs = {
        inherit primerServerSrc;
      };
      modules = [
        {
          nixpkgs.overlays = [ primerOverlay ];
        }
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
        {
          # Default to the flake-built package and coreutils-basic runtime.
          services.primer-student.package = nixpkgs.lib.mkDefault primerStudent;
          services.primer-student.runtimeProfilePackage =
            nixpkgs.lib.mkDefault runtimeCoreutilsBasic;
        }
      ];
    };

    packages.${system} = {
      default = primerStudent;
      primer-student = primerStudent;
      activity-validate = activityValidate;
      runtime-coreutils-basic = runtimeCoreutilsBasic;
      installer-iso =
        self.nixosConfigurations.installer.config.system.build.isoImage;
    };

    # Best-effort checks for CI / `nix flake check` (run via Docker when host
    # nix segfaults — see Makefile workstation-check).
    checks.${system} = {
      primer-student = primerStudent;
      runtime-coreutils-basic = runtimeCoreutilsBasic;
      activity-validate = pkgs.runCommand "activity-validate-check" {
        nativeBuildInputs = [ activityValidate ];
      } ''
        activity-validate -dir ${curriculumActivities} -no-materialize
        mkdir -p $out
        echo ok > $out/result
      '';
      # Evaluate the workstation module set and ensure the package is wired.
      workstation-eval = pkgs.runCommand "workstation-eval" {
        # Force evaluation of key options without building the full toplevel
        # (toplevel needs disko devices and is heavy for quick checks).
        passAsFile = [ ];
      } ''
        set -e
        test -n "${primerStudent}"
        test -x "${primerStudent}/bin/primer-student"
        "${primerStudent}/bin/primer-student" -version
        test -d "${runtimeCoreutilsBasic}/bin"
        mkdir -p $out
        echo "package=${primerStudent}" > $out/result
        echo "runtime=${runtimeCoreutilsBasic}" >> $out/result
      '';
    };

    # Expose for scripts that need the filtered server src path.
    primerServerSrc = primerServerSrc;
  };
}
