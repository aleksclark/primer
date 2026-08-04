# Offline curriculum activity validator (same server module tree as primer-student).
{
  lib,
  buildGoModule,
  go_1_25 ? null,
  go,
  primerServerSrc,
  curriculumActivities ? null,
}:

let
  goToolchain =
    if go_1_25 != null then go_1_25
    else if lib.versionAtLeast go.version "1.25" then go
    else
      throw "activity-validate requires Go >= 1.25 (got ${go.version})";
in
(buildGoModule.override { go = goToolchain; }) {
  pname = "activity-validate";
  version = "0.1.0";

  src = primerServerSrc;

  subPackages = [ "cmd/activity-validate" ];

  vendorHash = "sha256-hcdrQx9TbQvwpp2jS/WAZXRyoIhbt2UVXdRHjh1XZF0=";

  ldflags = [ "-s" "-w" ];

  doCheck = false;

  # When curriculum is provided, run validation as the package check.
  passthru = lib.optionalAttrs (curriculumActivities != null) {
    activities = curriculumActivities;
  };

  meta = {
    description = "Validate Primer curriculum activity documents";
    mainProgram = "activity-validate";
  };
}
