# Named bubblewrap runtime profile for terminal activities that only need
# basic shell + coreutils (pwd, ls, cd, mv, cp, mkdir, cat, …).
#
# Bound read-only into the sandbox via PRIMER_RUNTIME_PROFILE_DIR (or
# PRIMER_RUNTIME_PROFILES_DIR/coreutils-basic). See
# server/internal/studentclient/sandbox/profiles.go.
{
  lib,
  symlinkJoin,
  bashInteractive,
  coreutils,
  findutils,
  gnugrep,
  gnused,
  gawk,
  diffutils,
}:

symlinkJoin {
  name = "primer-runtime-coreutils-basic";
  paths = [
    bashInteractive
    coreutils
    findutils
    gnugrep
    gnused
    gawk
    diffutils
  ];
  # Keep a stable layout: bin/ plus any share needed by the tools.
  postBuild = ''
    # Ensure /bin/sh exists for scripted checks that invoke sh -c.
    if [ ! -e "$out/bin/sh" ] && [ -e "$out/bin/bash" ]; then
      ln -s bash "$out/bin/sh"
    fi
    mkdir -p "$out/share/primer"
    cat > "$out/share/primer/runtime-profile" <<EOF
name=coreutils-basic
binaries=sh bash ls pwd cat mv cp mkdir rm rmdir echo true false head tail grep sed awk find diff touch chmod
EOF
  '';
  meta = {
    description = "Primer sandbox runtime profile: coreutils-basic";
  };
}
