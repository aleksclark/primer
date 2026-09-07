# Immutable image authority for the primer release set.
# Production selector is registry@sha256:<64 hex> only. Tags are trace metadata.
#
# Live baseline at contract introduction (source-only; not yet reconciled):
#   primer          trace_tag=f0ab751
#   primer-tv       trace_tag=6cd71b3
#   content-ingest  trace_tag=6cd71b3
# Digests are OCI index digests from GHCR (docker buildx imagetools inspect).

image_primer          = "ghcr.io/aleksclark/primer@sha256:53a0562581fb3d0992f49ad11383791f3ee5db824467cfa2d72bffe7a9a466f9"
# Recovery TV image from 3d642f16 and final ingest metadata-fix image, 2026-09-07.
# Fleet registry is authenticated/internal; Nomad nodes use their configured pull credentials.
image_primer_tv       = "registry.fleet.clark.team/primer-tv@sha256:ba7965af990019a0193a07f9aa4a296d00dd69d7d0576e254d5ca98d7b77dae0"
image_content_ingest  = "registry.fleet.clark.team/content-ingest@sha256:0a0201eb38b4b4b6e2c0516d7be91117ec795be7b49b267cd027ae117c3980e1"

# Optional human trace (never runtime selectors):
# image_primer_trace_tag         = "f0ab751"
# image_primer_tv_trace_tag      = "ingest-3d642f16"
# image_content_ingest_trace_tag = "finish-nfofix2-20260907005519"
